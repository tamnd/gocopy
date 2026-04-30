package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
)

// visitAssignStmt lowers `<target> = <value>` and chained
// `t0 = t1 = ... = tN-1 = <value>` to IR. The supported RHS shapes
// (mutually exclusive) are:
//
//   - *ast.Constant (simple-constant value).
//   - *ast.BinOp with both sides *ast.Constant of foldable numeric
//     kinds (compile-time fold via foldBinOp). Single-target only.
//   - *ast.BinOp with both sides *ast.Name (1..15 ASCII chars) and
//     a supported BinOp operator. Single-target only.
//   - *ast.UnaryOp with USub on a numeric *ast.Constant (negative
//     literal fold). Single-target only.
//   - *ast.UnaryOp with USub/Invert/Not on a Name (1..15 ASCII
//     chars). Single-target only.
//
// All target Names must be 1..15 ASCII chars and start at column
// 0..255. Returns the Loc of the last STORE_NAME (used by
// visitModule as the lastLoc anchor for the trailing terminator
// when this is the body's final statement).
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_stmt_assign.
func visitAssignStmt(u *compileUnit, a *ast.Assign, source []byte, _ bool) (bytecode.Loc, error) {
	if len(a.Targets) == 0 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	line := uint32(a.P.Line)
	if line < 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	if len(a.Targets) == 1 {
		switch t := a.Targets[0].(type) {
		case *ast.Subscript:
			return emitSubscriptStore(u, t, a.Value, line)
		case *ast.Attribute:
			return emitAttributeStore(u, t, a.Value, line)
		}
	}

	type targetInfo struct {
		Name string
		Col  uint16
		Len  uint16
	}
	targets := make([]targetInfo, len(a.Targets))
	for i, t := range a.Targets {
		n, ok := t.(*ast.Name)
		if !ok {
			return bytecode.Loc{}, ErrNotImplemented
		}
		nl := len(n.Id)
		if nl < 1 || nl > 15 {
			return bytecode.Loc{}, ErrNotImplemented
		}
		if n.P.Col > 255 {
			return bytecode.Loc{}, ErrNotImplemented
		}
		targets[i] = targetInfo{Name: n.Id, Col: uint16(n.P.Col), Len: uint16(nl)}
	}

	lines := splitLines(source)
	lec, ok := lineEndCol(lines, int(line))
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	lineEnd := uint16(lec)

	if err := emitAssignValue(u, a.Value, line, lineEnd, len(targets) > 1); err != nil {
		return bytecode.Loc{}, err
	}

	// Emit STORE_NAMEs. Single target → STORE_NAME at name Loc.
	// Chained → COPY 1 + STORE_NAME for each non-last target, then
	// STORE_NAME for the last target (no COPY).
	n := len(targets)
	var lastLoc bytecode.Loc
	if n == 1 {
		nameLoc := bytecode.Loc{Line: line, EndLine: line, Col: 0, EndCol: targets[0].Len}
		u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
			Op: bytecode.STORE_NAME, Arg: u.addName(targets[0].Name), Loc: nameLoc,
		})
		lastLoc = nameLoc
	} else {
		copyLoc := bytecode.Loc{Line: line, EndLine: line, Col: 0, EndCol: lineEnd}
		block := u.currentBlock()
		for i, t := range targets {
			tgtLoc := bytecode.Loc{
				Line: line, EndLine: line,
				Col: t.Col, EndCol: t.Col + t.Len,
			}
			if i < n-1 {
				block.Instrs = append(block.Instrs,
					ir.Instr{Op: bytecode.COPY, Arg: 1, Loc: copyLoc},
					ir.Instr{Op: bytecode.STORE_NAME, Arg: u.addName(t.Name), Loc: tgtLoc},
				)
			} else {
				block.Instrs = append(block.Instrs,
					ir.Instr{Op: bytecode.STORE_NAME, Arg: u.addName(t.Name), Loc: tgtLoc},
				)
				lastLoc = tgtLoc
			}
		}
	}
	return lastLoc, nil
}

// emitAssignValue emits the IR for the RHS expression of an Assign.
// chained is true when there are >=2 targets; in that case only the
// simple-Constant RHS shape is supported (CPython parity).
func emitAssignValue(u *compileUnit, e ast.Expr, line uint32, lineEnd uint16, chained bool) error {
	switch v := e.(type) {
	case *ast.Constant:
		val, ok := constantToValue(v)
		if !ok {
			return ErrNotImplemented
		}
		if sv, isStr := val.(string); isStr {
			for i := 0; i < len(sv); i++ {
				if sv[i] == '\n' {
					return ErrNotImplemented
				}
			}
			if !isPlainAsciiNoEscape(sv) {
				return ErrNotImplemented
			}
		}
		if v.P.Col > 255 {
			return ErrNotImplemented
		}
		valLoc := bytecode.Loc{
			Line: line, EndLine: line,
			Col: uint16(v.P.Col), EndCol: lineEnd,
		}
		emitConstLoad(u, val, valLoc)
		return nil

	case *ast.BinOp:
		if chained {
			return ErrNotImplemented
		}
		return emitBinOpValue(u, v, line, lineEnd)

	case *ast.UnaryOp:
		if chained {
			return ErrNotImplemented
		}
		return emitUnaryOpValue(u, v, line, lineEnd)

	case *ast.Compare:
		if chained {
			return ErrNotImplemented
		}
		return emitCompareValue(u, v, line, lineEnd)

	case *ast.BoolOp:
		if chained {
			return ErrNotImplemented
		}
		return emitBoolOpValue(u, v, line, lineEnd)

	case *ast.IfExp:
		if chained {
			return ErrNotImplemented
		}
		return emitIfExpValue(u, v, line, lineEnd)

	case *ast.List, *ast.Tuple, *ast.Set, *ast.Dict,
		*ast.Subscript, *ast.Attribute, *ast.Call, *ast.Name:
		if chained {
			return ErrNotImplemented
		}
		_, _, err := visitExpr(u, e, line)
		return err
	}
	return ErrNotImplemented
}

// emitBinOpValue handles BinOp RHS in two flavors:
//
//   - BinOp(Constant, op, Constant) with foldable numeric operands:
//     phantom-add the leftVal, then either LOAD_SMALL_INT (when the
//     fold result fits 0..255) or a deferred LOAD_CONST that finalize
//     rewrites to land AFTER None.
//   - Anything else: delegate to visitBinOpExpr, which recurses both
//     operands through visitExpr and emits BINARY_OP at the composite
//     span. v0.7.5 widened this from the v0.7.2 Name+Name closed form
//     so modGenExpr-shape inputs (`x = a + 5`, `x = a + b * c`) lower
//     through the visitor.
func emitBinOpValue(u *compileUnit, b *ast.BinOp, line uint32, lineEnd uint16) error {
	if lc, ok := b.Left.(*ast.Constant); ok {
		if rc, ok := b.Right.(*ast.Constant); ok {
			lv, lok := numConstVal(lc)
			rv, rok := numConstVal(rc)
			if !lok || !rok {
				return ErrNotImplemented
			}
			result, fok := foldBinOp(lv, b.Op, rv)
			if !fok {
				return ErrNotImplemented
			}
			if lc.P.Col > 255 {
				return ErrNotImplemented
			}
			valLoc := bytecode.Loc{
				Line: line, EndLine: line,
				Col: uint16(lc.P.Col), EndCol: lineEnd,
			}
			if !u.phantomDone {
				u.addConst(lv)
				u.phantomDone = true
			}
			if iv, isInt := result.(int64); isInt && iv >= 0 && iv <= 255 {
				u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
					Op: bytecode.LOAD_SMALL_INT, Arg: uint32(iv), Loc: valLoc,
				})
				return nil
			}
			emitDeferredConstLoad(u, result, valLoc)
			return nil
		}
	}

	_, _, err := visitBinOpExpr(u, b, line)
	return err
}

// emitUnaryOpValue handles UnaryOp RHS in two flavors:
//
//   - UnaryOp(USub, Constant) with int64/float64 operand: phantom-add
//     the positive operand, then emit a deferred LOAD_CONST that
//     finalize rewrites to land AFTER None for the negative result.
//   - Anything else: delegate to visitUnaryOpExpr, which recurses the
//     operand through visitExpr and emits UNARY_NEGATIVE / UNARY_INVERT
//     (or TO_BOOL + UNARY_NOT for Not) at the composite span. Widened
//     in v0.7.5 from the v0.7.2 Name-only closed form.
func emitUnaryOpValue(u *compileUnit, un *ast.UnaryOp, line uint32, lineEnd uint16) error {
	if un.Op == "USub" {
		if c, ok := un.Operand.(*ast.Constant); ok {
			cv, cok := numConstVal(c)
			if !cok {
				return ErrNotImplemented
			}
			var neg any
			switch tv := cv.(type) {
			case int64:
				if tv == 0 {
					return ErrNotImplemented
				}
				neg = -tv
			case float64:
				neg = -tv
			default:
				return ErrNotImplemented
			}
			if un.P.Col > 255 {
				return ErrNotImplemented
			}
			valLoc := bytecode.Loc{
				Line: line, EndLine: line,
				Col: uint16(un.P.Col), EndCol: lineEnd,
			}
			if !u.phantomDone {
				u.addConst(cv)
				u.phantomDone = true
			}
			emitDeferredConstLoad(u, neg, valLoc)
			return nil
		}
	}

	_, _, err := visitUnaryOpExpr(u, un, line)
	return err
}

// visitAugAssignStmt lowers `<name> <op>= <value>` to IR. Only the
// shape `<name> += int_const` (and friends; 12 in-place BinOp
// operators) is supported, with name 1..15 ASCII chars and value a
// non-negative int Constant.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_stmt_augassign
// for the int-RHS case the v0.7.2 visitor handles.
func visitAugAssignStmt(u *compileUnit, aug *ast.AugAssign, source []byte, _ bool) (bytecode.Loc, error) {
	n, ok := aug.Target.(*ast.Name)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	nl := len(n.Id)
	if nl < 1 || nl > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if n.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	c, ok := aug.Value.(*ast.Constant)
	if !ok || c.Kind != "int" {
		return bytecode.Loc{}, ErrNotImplemented
	}
	val, ok := c.Value.(int64)
	if !ok || val < 0 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if c.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	oparg, ok := augOpargFromOp(aug.Op)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	line := uint32(aug.P.Line)
	if line < 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	lines := splitLines(source)
	lec, ok := lineEndCol(lines, int(line))
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	lineEnd := uint16(lec)

	nameLoc := bytecode.Loc{Line: line, EndLine: line, Col: 0, EndCol: uint16(nl)}
	valLoc := bytecode.Loc{Line: line, EndLine: line, Col: uint16(c.P.Col), EndCol: lineEnd}
	binOpLoc := bytecode.Loc{Line: line, EndLine: line, Col: 0, EndCol: lineEnd}

	nameIdx := u.addName(n.Id)
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.LOAD_NAME, Arg: nameIdx, Loc: nameLoc,
	})
	emitConstLoad(u, val, valLoc)
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.BINARY_OP, Arg: uint32(oparg), Loc: binOpLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: nameIdx, Loc: nameLoc},
	)
	return nameLoc, nil
}
