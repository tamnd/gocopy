package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitAttributeExpr emits IR for an *ast.Attribute in value (Load)
// context: recurse the object, emit LOAD_ATTR with oparg = idx<<1
// (the assembler appends 9 cache words; the line-table encoder splits
// the 10-code-unit run as 8+2 with the same Loc). Returns the
// attribute's column span (object start → attribute end) for the
// caller's wrapping op Loc.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr_attribute
// (Load branch).
func visitAttributeExpr(u *compileUnit, a *ast.Attribute, line uint32) (uint16, uint16, error) {
	if l := len(a.Attr); l < 1 || l > 15 {
		return 0, 0, ErrNotImplemented
	}
	objCol, objEnd, err := visitExpr(u, a.Value, line)
	if err != nil {
		return 0, 0, err
	}
	attrEnd := objEnd + 1 + uint16(len(a.Attr)) // +1 for the '.'
	loc := bytecode.Loc{Line: line, EndLine: line, Col: objCol, EndCol: attrEnd}
	idx := u.addName(a.Attr)
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.LOAD_ATTR, Arg: idx << 1, Loc: loc,
	})
	return objCol, attrEnd, nil
}

// emitAttributeStore emits IR for the attribute-target branch of
// `<obj>.<attr> = <val>`: recurse the RHS, recurse the object, emit
// STORE_ATTR with oparg = attrIdx (the assembler appends 4 cache
// words). Returns the store Loc the caller should anchor the trailing
// terminator on.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_stmt_assign
// (Attribute-target branch).
func emitAttributeStore(u *compileUnit, a *ast.Attribute, value ast.Expr, line uint32) (bytecode.Loc, error) {
	if l := len(a.Attr); l < 1 || l > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if _, _, err := visitExpr(u, value, line); err != nil {
		return bytecode.Loc{}, err
	}
	objCol, objEnd, err := visitExpr(u, a.Value, line)
	if err != nil {
		return bytecode.Loc{}, err
	}
	attrEnd := objEnd + 1 + uint16(len(a.Attr))
	loc := bytecode.Loc{Line: line, EndLine: line, Col: objCol, EndCol: attrEnd}
	idx := u.addName(a.Attr)
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.STORE_ATTR, Arg: idx, Loc: loc,
	})
	return loc, nil
}
