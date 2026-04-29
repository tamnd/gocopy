package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitExpr emits IR for e in value (load) context. Returns the
// (startCol, endCol) span the caller can use to anchor a wrapping
// op's Loc. line is the statement's source line; v0.7.5 keeps the
// classifier-era single-line surface restrictions for byte parity.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr — the
// recursive expression dispatcher every value-context emit goes
// through. v0.7.5 is the first release where the visitor actually
// recurses through nested expressions rather than handling closed-
// form one- or two-operand cases.
func visitExpr(u *compileUnit, e ast.Expr, line uint32) (uint16, uint16, error) {
	switch v := e.(type) {
	case *ast.Constant:
		return visitConstantExpr(u, v, line)
	case *ast.Name:
		return visitNameExpr(u, v, line)
	case *ast.BinOp:
		return visitBinOpExpr(u, v, line)
	case *ast.UnaryOp:
		return visitUnaryOpExpr(u, v, line)
	case *ast.Compare:
		if err := emitCompareValue(u, v, line, 0); err != nil {
			return 0, 0, err
		}
		return 0, 0, nil
	case *ast.BoolOp:
		if err := emitBoolOpValue(u, v, line, 0); err != nil {
			return 0, 0, err
		}
		return 0, 0, nil
	case *ast.IfExp:
		if err := emitIfExpValue(u, v, line, 0); err != nil {
			return 0, 0, err
		}
		return 0, 0, nil
	case *ast.List:
		return visitListExpr(u, v, line)
	case *ast.Tuple:
		return visitTupleExpr(u, v, line)
	case *ast.Set:
		return visitSetExpr(u, v, line)
	case *ast.Dict:
		return visitDictExpr(u, v, line)
	case *ast.Subscript:
		return visitSubscriptExpr(u, v, line)
	case *ast.Attribute:
		return visitAttributeExpr(u, v, line)
	case *ast.Call:
		return visitCallExpr(u, v, line)
	}
	return 0, 0, ErrNotImplemented
}

// visitNameExpr emits LOAD_NAME for n at module scope. Restricted to
// 1..15 ASCII chars on the statement's source line and a column ≤ 255
// (the v0.6 classifier's SHORT0 envelope, kept here so byte parity
// holds at promotion).
func visitNameExpr(u *compileUnit, n *ast.Name, line uint32) (uint16, uint16, error) {
	nl := len(n.Id)
	if nl < 1 || nl > 15 {
		return 0, 0, ErrNotImplemented
	}
	if n.P.Col > 255 || uint32(n.P.Line) != line {
		return 0, 0, ErrNotImplemented
	}
	col := uint16(n.P.Col)
	end := col + uint16(nl)
	loc := bytecode.Loc{Line: line, EndLine: line, Col: col, EndCol: end}
	idx := u.addName(n.Id)
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.LOAD_NAME, Arg: idx, Loc: loc,
	})
	return col, end, nil
}

// visitConstantExpr emits LOAD_SMALL_INT (for int 0..255) or
// LOAD_CONST for c in a recursive value context. The constant's end
// column is computed from source via constEndCol — distinct from the
// top-level Constant arm in emitAssignValue, which uses lineEnd because
// the simple-Constant `x = K` shape has K occupying the rest of the
// line. v0.7.5 keeps the modGenExpr surface restriction (int 0..255
// only) for byte parity with the v0.6 classifier walker.
func visitConstantExpr(u *compileUnit, c *ast.Constant, line uint32) (uint16, uint16, error) {
	if uint32(c.P.Line) != line || c.P.Col > 255 {
		return 0, 0, ErrNotImplemented
	}
	if c.Kind != "int" {
		return 0, 0, ErrNotImplemented
	}
	iv, ok := c.Value.(int64)
	if !ok || iv < 0 || iv > 255 {
		return 0, 0, ErrNotImplemented
	}
	col := uint16(c.P.Col)
	end := constEndCol(u.Source, c)
	loc := bytecode.Loc{Line: line, EndLine: line, Col: col, EndCol: end}
	emitConstLoad(u, iv, loc)
	return col, end, nil
}

// visitBinOpExpr emits IR for a BinOp in recursive (non-fold)
// context. Both operands are recursed via visitExpr; the BINARY_OP's
// Loc spans from the left operand's start column to the right
// operand's end column. Mirrors the gen-expr classifier walker's
// BinOp arm and CPython 3.14's
// Python/codegen.c::codegen_visit_expr_binop.
func visitBinOpExpr(u *compileUnit, b *ast.BinOp, line uint32) (uint16, uint16, error) {
	oparg, ok := binOpargFromOp(b.Op)
	if !ok {
		return 0, 0, ErrNotImplemented
	}
	lc, _, err := visitExpr(u, b.Left, line)
	if err != nil {
		return 0, 0, err
	}
	_, re, err := visitExpr(u, b.Right, line)
	if err != nil {
		return 0, 0, err
	}
	loc := bytecode.Loc{Line: line, EndLine: line, Col: lc, EndCol: re}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BINARY_OP, Arg: uint32(oparg), Loc: loc,
	})
	return lc, re, nil
}

// visitUnaryOpExpr emits IR for a UnaryOp in recursive (non-fold)
// context. Op is one of USub / Invert / Not. The operand is recursed
// via visitExpr; the UNARY_* (or TO_BOOL+UNARY_NOT for Not) Loc spans
// from the op column to the operand's end column.
func visitUnaryOpExpr(u *compileUnit, un *ast.UnaryOp, line uint32) (uint16, uint16, error) {
	if un.P.Col > 255 || uint32(un.P.Line) != line {
		return 0, 0, ErrNotImplemented
	}
	opCol := uint16(un.P.Col)
	_, oe, err := visitExpr(u, un.Operand, line)
	if err != nil {
		return 0, 0, err
	}
	loc := bytecode.Loc{Line: line, EndLine: line, Col: opCol, EndCol: oe}
	block := u.currentBlock()
	switch un.Op {
	case "USub":
		block.Instrs = append(block.Instrs, ir.Instr{
			Op: bytecode.UNARY_NEGATIVE, Arg: 0, Loc: loc,
		})
	case "Invert":
		block.Instrs = append(block.Instrs, ir.Instr{
			Op: bytecode.UNARY_INVERT, Arg: 0, Loc: loc,
		})
	case "Not":
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: loc},
			ir.Instr{Op: bytecode.UNARY_NOT, Arg: 0, Loc: loc},
		)
	default:
		return 0, 0, ErrNotImplemented
	}
	return opCol, oe, nil
}

// constEndCol returns the column after the last character of the
// constant token starting at c.P.Col on the source line. Mirrors the
// v0.6 gen-expr walker's constEndCol byte-for-byte: scans
// [0-9a-zA-Z_.] forward from the start column. For modGenExpr's
// int 0..255 surface this picks up the digit run.
func constEndCol(source []byte, c *ast.Constant) uint16 {
	lines := splitLines(source)
	line := c.P.Line
	if line < 1 || line > len(lines) {
		return uint16(c.P.Col) + 1
	}
	src := lines[line-1]
	col := c.P.Col
	if col >= len(src) {
		return uint16(col) + 1
	}
	i := col
	for i < len(src) {
		ch := src[i]
		if ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch == '.' {
			i++
		} else {
			break
		}
	}
	return uint16(i)
}
