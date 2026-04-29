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
// Loc spans (left.startCol, right.endCol). Mirrors
// CPython 3.14 Python/codegen.c::codegen_visit_expr_binop.
func visitFuncBinOpExpr(u *compileUnit, b *ast.BinOp, lines [][]byte) (uint16, uint16, error) {
	oparg, ok := binOpargFromOp(b.Op)
	if !ok {
		return 0, 0, ErrNotImplemented
	}
	lc, _, err := visitFuncExpr(u, b.Left, lines)
	if err != nil {
		return 0, 0, err
	}
	_, re, err := visitFuncExpr(u, b.Right, lines)
	if err != nil {
		return 0, 0, err
	}
	fuseLflblflbTail(u)
	loc := bytecode.Loc{
		Line: uint32(b.P.Line), EndLine: uint32(b.P.Line),
		Col: lc, EndCol: re,
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BINARY_OP, Arg: uint32(oparg), Loc: loc,
	})
	return lc, re, nil
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
