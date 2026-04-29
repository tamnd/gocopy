package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitFuncExpr is the function-scope expression dispatcher for the
// v0.7.10 visit_FunctionDef extension. It mirrors
// CPython 3.14 Python/codegen.c::codegen_visit_expr but consults
// the compileUnit's symtable.Scope for Name resolution
// (via scope_ops.go) and uses function-body constant pool semantics
// (no module-scope phantom slot).
//
// Returns the (startCol, endCol) span of the emitted code. The
// span anchors a wrapping op's Loc — for example, BINARY_OP uses
// (left.startCol, right.endCol).
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_expr.
func visitFuncExpr(u *compileUnit, e ast.Expr, lines [][]byte) (uint16, uint16, error) {
	switch v := e.(type) {
	case *ast.Name:
		return visitFuncNameExpr(u, v)
	case *ast.Constant:
		return visitFuncConstantExpr(u, v, lines)
	case *ast.BinOp:
		return visitFuncBinOpExpr(u, v, lines)
	case *ast.UnaryOp:
		return visitFuncUnaryOpExpr(u, v, lines)
	case *ast.Compare:
		return visitFuncCompareExpr(u, v, lines)
	case *ast.Call:
		return visitFuncCallExpr(u, v, lines)
	case *ast.Attribute:
		return visitFuncAttributeExpr(u, v, lines)
	case *ast.Subscript:
		return visitFuncSubscriptExpr(u, v, lines)
	case *ast.Tuple:
		return visitFuncTupleExpr(u, v, lines)
	}
	return 0, 0, ErrNotImplemented
}

// visitFuncNameExpr emits a load for n in function (or any non-
// module) scope. Uses scope_ops to dispatch among
// LOAD_FAST_BORROW / LOAD_DEREF / LOAD_GLOBAL / LOAD_NAME. The
// LOAD_FAST_BORROW emit is the v0.7.10 byte-parity precomputation;
// see scope_ops.go::emitNameLoad for the lifecycle note.
func visitFuncNameExpr(u *compileUnit, n *ast.Name) (uint16, uint16, error) {
	if len(n.Id) < 1 || len(n.Id) > 15 || n.P.Col > 255 {
		return 0, 0, ErrNotImplemented
	}
	col := uint16(n.P.Col)
	end := col + uint16(len(n.Id))
	loc := bytecode.Loc{
		Line: uint32(n.P.Line), EndLine: uint32(n.P.Line),
		Col: col, EndCol: end,
	}
	u.emitNameLoad(n.Id, loc)
	fuseLflblflbTail(u)
	return col, end, nil
}

// visitFuncConstantExpr emits a constant load in function-body
// context. int 0..255 emits LOAD_SMALL_INT (no co_consts entry
// unless this is the first int seen with no other constant in the
// pool — the CPython 3.14 quirk that places the literal at
// co_consts[0] anyway, mirrored from
// compiler/func_body.go::loadConst).
func visitFuncConstantExpr(u *compileUnit, c *ast.Constant, lines [][]byte) (uint16, uint16, error) {
	if c.P.Col > 255 {
		return 0, 0, ErrNotImplemented
	}
	val, ok := constantValue(c)
	if !ok {
		return 0, 0, ErrNotImplemented
	}
	endCol := astExprEndCol(lines, c.P.Line, c)
	col := uint16(c.P.Col)
	end := uint16(endCol)
	loc := bytecode.Loc{
		Line: uint32(c.P.Line), EndLine: uint32(c.P.Line),
		Col: col, EndCol: end,
	}
	emitFuncBodyConstLoadFirstInt(u, val, loc)
	return col, end, nil
}

// emitFuncBodyConstLoadFirstInt is emitFuncBodyConstLoad with the
// first-int quirk: if v is an int 0..255 and it's the first
// constant the function loads (and the function is not a
// noneCheckFunc), we plant the literal in co_consts[0] in addition
// to emitting LOAD_SMALL_INT. This mirrors
// compiler/func_body.go::loadConst's intConstSeen guard, which
// matches CPython 3.14 Python/compile.c's pool layout for
// LOAD_SMALL_INT-only functions.
//
// v0.7.14's trim_unused_consts pass will absorb this, at which
// point this helper reverts to the simpler emitFuncBodyConstLoad.
func emitFuncBodyConstLoadFirstInt(u *compileUnit, v any, loc bytecode.Loc) {
	block := u.currentBlock()
	if iv, ok := v.(int64); ok && iv >= 0 && iv <= 255 {
		if len(u.Consts) == 0 {
			u.addConst(iv)
		}
		block.Instrs = append(block.Instrs, ir.Instr{
			Op: bytecode.LOAD_SMALL_INT, Arg: uint32(iv), Loc: loc,
		})
		return
	}
	idx := u.addConst(v)
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: bytecode.LOAD_CONST, Arg: idx, Loc: loc,
	})
}

// visitFuncBinOpExpr emits a BinOp in recursive function-scope
// context. Both operands are recursed via visitFuncExpr; BINARY_OP's
// Loc spans (left.startCol, right.endCol).
//
// Paren extension (mirrors compiler/func_body.go's BinOp arm):
//
//   - When the left operand is itself a BinOp wrapped in '(...)',
//     extend lsc back over the '(' provided the matching ')' closes
//     at or before the right operand's start column.
//   - When the right operand is itself a BinOp wrapped in '(...)',
//     extend rec past whitespace and a trailing ')' via
//     scanWhitespaceClose. The check `scanBackOpen(rsc) < rsc`
//     confirms the right child's leftmost token is preceded by '('.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_expr_binop +
// the surrounding tokenizer's column tracking.
func visitFuncBinOpExpr(u *compileUnit, b *ast.BinOp, lines [][]byte) (uint16, uint16, error) {
	oparg, ok := binOpargFromOp(b.Op)
	if !ok {
		return 0, 0, ErrNotImplemented
	}
	lc, _, err := visitFuncExpr(u, b.Left, lines)
	if err != nil {
		return 0, 0, err
	}
	if _, isBinOp := b.Left.(*ast.BinOp); isBinOp {
		candidate := scanBackOpen(lines, b.P.Line, byte(lc))
		if candidate < byte(lc) {
			closeCol := scanMatchingClose(lines, b.P.Line, candidate)
			if closeCol <= astExprCol(b.Right) {
				lc = uint16(candidate)
			}
		}
	}
	rsc, re, err := visitFuncExpr(u, b.Right, lines)
	if err != nil {
		return 0, 0, err
	}
	if _, isBinOp := b.Right.(*ast.BinOp); isBinOp {
		if scanBackOpen(lines, b.P.Line, byte(rsc)) < byte(rsc) {
			re = uint16(scanWhitespaceClose(lines, b.P.Line, byte(re)))
		}
	}
	loc := bytecode.Loc{
		Line: uint32(b.P.Line), EndLine: uint32(b.P.Line),
		Col: lc, EndCol: re,
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BINARY_OP, Arg: uint32(oparg), Loc: loc,
	})
	return lc, re, nil
}

// visitFuncUnaryOpExpr emits IR for `-x` / `~x` / `not x` in
// function-scope context. Mirrors compiler/func_body.go's UnaryOp
// arm:
//
//   - USub / Invert: walk operand, emit UNARY_NEGATIVE or
//     UNARY_INVERT at (opCol, operandEnd).
//   - Not (operand is single-op single-comparator Compare): emit the
//     Compare with the conditional-context bit (oparg+16) set, then
//     UNARY_NOT — no TO_BOOL, no caches between. The COMPARE_OP and
//     UNARY_NOT both span (opCol, scanWhitespaceClose(rhsEnd)).
//   - Not (general): walk operand, emit TO_BOOL + UNARY_NOT at
//     (opCol, scanWhitespaceClose(operandEnd)).
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_expr_unary +
// the codegen_compare conditional-flag specialisation.
func visitFuncUnaryOpExpr(u *compileUnit, un *ast.UnaryOp, lines [][]byte) (uint16, uint16, error) {
	if un.P.Col > 255 {
		return 0, 0, ErrNotImplemented
	}
	opCol := uint16(un.P.Col)
	block := u.currentBlock()
	switch un.Op {
	case "USub", "Invert":
		_, oe, err := visitFuncExpr(u, un.Operand, lines)
		if err != nil {
			return 0, 0, err
		}
		loc := bytecode.Loc{
			Line: uint32(un.P.Line), EndLine: uint32(un.P.Line),
			Col: opCol, EndCol: oe,
		}
		op := bytecode.UNARY_NEGATIVE
		if un.Op == "Invert" {
			op = bytecode.UNARY_INVERT
		}
		block.Instrs = append(block.Instrs, ir.Instr{Op: op, Arg: 0, Loc: loc})
		return opCol, oe, nil
	case "Not":
		if cmp, isCmp := un.Operand.(*ast.Compare); isCmp &&
			len(cmp.Ops) == 1 && len(cmp.Comparators) == 1 {
			op, base, ok := cmpOpFromAstOp(cmp.Ops[0])
			if !ok {
				return 0, 0, ErrNotImplemented
			}
			if _, _, err := visitFuncExpr(u, cmp.Left, lines); err != nil {
				return 0, 0, err
			}
			_, re, err := visitFuncExpr(u, cmp.Comparators[0], lines)
			if err != nil {
				return 0, 0, err
			}
			closedRec := uint16(scanWhitespaceClose(lines, un.P.Line, byte(re)))
			loc := bytecode.Loc{
				Line: uint32(un.P.Line), EndLine: uint32(un.P.Line),
				Col: opCol, EndCol: closedRec,
			}
			block.Instrs = append(block.Instrs,
				ir.Instr{Op: op, Arg: uint32(base + 16), Loc: loc},
				ir.Instr{Op: bytecode.UNARY_NOT, Arg: 0, Loc: loc},
			)
			return opCol, closedRec, nil
		}
		_, oe, err := visitFuncExpr(u, un.Operand, lines)
		if err != nil {
			return 0, 0, err
		}
		closedEnd := uint16(scanWhitespaceClose(lines, un.P.Line, byte(oe)))
		loc := bytecode.Loc{
			Line: uint32(un.P.Line), EndLine: uint32(un.P.Line),
			Col: opCol, EndCol: closedEnd,
		}
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: loc},
			ir.Instr{Op: bytecode.UNARY_NOT, Arg: 0, Loc: loc},
		)
		return opCol, closedEnd, nil
	}
	return 0, 0, ErrNotImplemented
}

// visitFuncCompareExpr emits a value-context Compare in function
// scope. v0.7.10 supports a single op with one Comparator (no
// chained `a < b < c` — that emits a different shape via
// JUMP_IF_FALSE_OR_POP).
//
// Both operands are recursed via visitFuncExpr; the trailing
// COMPARE_OP / IS_OP / CONTAINS_OP uses the value-context oparg
// (no +16 conditional bit) and a Loc spanning (left.start,
// right.end).
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_compare.
func visitFuncCompareExpr(u *compileUnit, c *ast.Compare, lines [][]byte) (uint16, uint16, error) {
	if len(c.Ops) != 1 || len(c.Comparators) != 1 {
		return 0, 0, ErrNotImplemented
	}
	op, base, ok := cmpOpFromAstOp(c.Ops[0])
	if !ok {
		return 0, 0, ErrNotImplemented
	}
	lc, _, err := visitFuncExpr(u, c.Left, lines)
	if err != nil {
		return 0, 0, err
	}
	_, re, err := visitFuncExpr(u, c.Comparators[0], lines)
	if err != nil {
		return 0, 0, err
	}
	loc := bytecode.Loc{
		Line: uint32(c.P.Line), EndLine: uint32(c.P.Line),
		Col: lc, EndCol: re,
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: op, Arg: uint32(base), Loc: loc,
	})
	return lc, re, nil
}

// visitFuncAttributeExpr emits IR for `value.attr` in value
// context (LOAD_ATTR with the method bit clear). For method
// callees (`obj.method()`) the Call dispatcher in
// visitFuncCallExpr emits its own LOAD_ATTR with the method bit
// set — this helper is the value-context (load-only) path.
func visitFuncAttributeExpr(u *compileUnit, a *ast.Attribute, lines [][]byte) (uint16, uint16, error) {
	if len(a.Attr) < 1 || len(a.Attr) > 15 || a.P.Col > 255 {
		return 0, 0, ErrNotImplemented
	}
	startCol, _, err := visitFuncExpr(u, a.Value, lines)
	if err != nil {
		return 0, 0, err
	}
	endCol := uint16(astExprEndCol(lines, a.P.Line, a))
	loc := bytecode.Loc{
		Line: uint32(a.P.Line), EndLine: uint32(a.P.Line),
		Col: startCol, EndCol: endCol,
	}
	nameIdx := u.addName(a.Attr)
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.LOAD_ATTR, Arg: uint32(bytecode.LoadAttrArg(byte(nameIdx), false)), Loc: loc,
	})
	return startCol, endCol, nil
}

// visitFuncSubscriptExpr emits IR for `value[slice]` — pushes
// value and slice, then BINARY_OP NbGetItem (the CPython 3.14
// subscript opcode is BINARY_OP with the NB_GETITEM oparg).
func visitFuncSubscriptExpr(u *compileUnit, s *ast.Subscript, lines [][]byte) (uint16, uint16, error) {
	if s.P.Col > 255 {
		return 0, 0, ErrNotImplemented
	}
	startCol, _, err := visitFuncExpr(u, s.Value, lines)
	if err != nil {
		return 0, 0, err
	}
	_, _, err = visitFuncExpr(u, s.Slice, lines)
	if err != nil {
		return 0, 0, err
	}
	endCol := uint16(astExprEndCol(lines, s.P.Line, s))
	loc := bytecode.Loc{
		Line: uint32(s.P.Line), EndLine: uint32(s.P.Line),
		Col: startCol, EndCol: endCol,
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BINARY_OP, Arg: bytecode.NbGetItem, Loc: loc,
	})
	return startCol, endCol, nil
}

// visitFuncTupleExpr emits IR for an unparenthesized or
// parenthesized tuple literal in value context — recurse each
// element, then BUILD_TUPLE n.
//
// Paren extension (mirrors compiler/func_body.go's Tuple arm):
//
//   - For parenthesised tuples gopapy's parser sets t.P.Col to the
//     '(' column, which is one column before the first element. If
//     so, extend startCol back to t.P.Col.
//   - Always run scanWhitespaceClose past the last element to
//     consume a trailing ')'. For unparenthesised tuples this is a
//     no-op (the byte after the last element is not ')').
func visitFuncTupleExpr(u *compileUnit, t *ast.Tuple, lines [][]byte) (uint16, uint16, error) {
	if t.P.Col > 255 || len(t.Elts) == 0 {
		return 0, 0, ErrNotImplemented
	}
	var startCol, lastEnd uint16
	for i, e := range t.Elts {
		sc, ec, err := visitFuncExpr(u, e, lines)
		if err != nil {
			return 0, 0, err
		}
		if i == 0 {
			startCol = sc
		}
		lastEnd = ec
	}
	if uint16(t.P.Col) < startCol {
		startCol = uint16(t.P.Col)
	}
	endCol := uint16(scanWhitespaceClose(lines, t.P.Line, byte(lastEnd)))
	loc := bytecode.Loc{
		Line: uint32(t.P.Line), EndLine: uint32(t.P.Line),
		Col: startCol, EndCol: endCol,
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BUILD_TUPLE, Arg: uint32(len(t.Elts)), Loc: loc,
	})
	return startCol, endCol, nil
}

// visitFuncCallExpr emits a positional-only Call in function-body
// scope. The callee is loaded via emitNameLoadCall — for a global
// or builtin Name that means LOAD_GLOBAL with bit 0 of the oparg
// set (the "push NULL before call" hint that replaces the explicit
// PUSH_NULL the module-scope visitor emits). Args are recursed in
// order; fuseLflblflbTail collapses the trailing pair if both are
// LOAD_FAST_BORROW with slot ≤ 15.
//
// Keyword args, *args, and **kwargs land in later steps. Func
// shapes other than *ast.Name (e.g. Attribute method calls) also
// land later.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_call (positional
// fast path).
func visitFuncCallExpr(u *compileUnit, c *ast.Call, lines [][]byte) (uint16, uint16, error) {
	if len(c.Keywords) != 0 {
		return 0, 0, ErrNotImplemented
	}
	var funcCol, funcEnd uint16
	switch fn := c.Func.(type) {
	case *ast.Name:
		if len(fn.Id) < 1 || len(fn.Id) > 15 || fn.P.Col > 255 {
			return 0, 0, ErrNotImplemented
		}
		funcCol = uint16(fn.P.Col)
		funcEnd = funcCol + uint16(len(fn.Id))
		funcLoc := bytecode.Loc{
			Line: uint32(fn.P.Line), EndLine: uint32(fn.P.Line),
			Col: funcCol, EndCol: funcEnd,
		}
		u.emitNameLoadCall(fn.Id, funcLoc)
	case *ast.Attribute:
		if len(fn.Attr) < 1 || len(fn.Attr) > 15 || fn.P.Col > 255 {
			return 0, 0, ErrNotImplemented
		}
		var err error
		funcCol, _, err = visitFuncExpr(u, fn.Value, lines)
		if err != nil {
			return 0, 0, err
		}
		funcEnd = uint16(astExprEndCol(lines, fn.P.Line, fn))
		attrLoc := bytecode.Loc{
			Line: uint32(fn.P.Line), EndLine: uint32(fn.P.Line),
			Col: funcCol, EndCol: funcEnd,
		}
		nameIdx := u.addName(fn.Attr)
		u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
			Op:  bytecode.LOAD_ATTR,
			Arg: uint32(bytecode.LoadAttrArg(byte(nameIdx), true)),
			Loc: attrLoc,
		})
	default:
		return 0, 0, ErrNotImplemented
	}

	scanFrom := funcEnd
	for _, e := range c.Args {
		_, argEnd, err := visitFuncExpr(u, e, lines)
		if err != nil {
			return 0, 0, err
		}
		scanFrom = argEnd
	}

	closeEnd := scanCallEnd(lines, c.P.Line, byte(scanFrom))
	if closeEnd == byte(scanFrom) {
		return 0, 0, ErrNotImplemented
	}
	loc := bytecode.Loc{
		Line: uint32(c.P.Line), EndLine: uint32(c.P.Line),
		Col: funcCol, EndCol: uint16(closeEnd),
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.CALL, Arg: uint32(len(c.Args)), Loc: loc,
	})
	return funcCol, uint16(closeEnd), nil
}

// fuseLflblflbTail scans the current block's last few instructions
// and fuses two consecutive LOAD_FAST_BORROW with slots ≤ 15 into a
// single LOAD_FAST_BORROW_LOAD_FAST_BORROW. The fused instruction's
// Loc and Arg are taken from the first LOAD_FAST_BORROW's Loc and
// the LflblflbArg(slotL, slotR) encoding respectively.
//
// This is v0.7.10 byte-parity precomputation for the
// insert_superinstructions pass that lands at v0.7.16. CPython's
// real pass runs over the entire CFG at the end of compilation; the
// visitor's pre-pass runs after each expression / statement so it
// catches the same back-to-back-load shapes the v0.6 classifier
// emitted by hand.
func fuseLflblflbTail(u *compileUnit) {
	block := u.currentBlock()
	n := len(block.Instrs)
	if n < 2 {
		return
	}
	a := block.Instrs[n-2]
	b := block.Instrs[n-1]
	if a.Op != bytecode.LOAD_FAST_BORROW || b.Op != bytecode.LOAD_FAST_BORROW {
		return
	}
	if a.Arg >= 16 || b.Arg >= 16 {
		return
	}
	fused := ir.Instr{
		Op:  bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW,
		Arg: uint32(bytecode.LflblflbArg(byte(a.Arg), byte(b.Arg))),
		Loc: a.Loc,
	}
	block.Instrs = append(block.Instrs[:n-2], fused)
}
