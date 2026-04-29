package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitSubscriptExpr emits IR for an *ast.Subscript in value (Load)
// context: recurse the object, recurse the slice, emit BINARY_OP
// NbGetItem (1 cache word per assembler). Returns the subscript's
// column span (object start → close-bracket column) for the caller's
// wrapping op Loc.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr_subscript
// (Load branch).
func visitSubscriptExpr(u *compileUnit, s *ast.Subscript, line uint32) (uint16, uint16, error) {
	objCol, _, err := visitExpr(u, s.Value, line)
	if err != nil {
		return 0, 0, err
	}
	_, sliceEnd, err := visitExpr(u, s.Slice, line)
	if err != nil {
		return 0, 0, err
	}
	closeEnd := sliceEnd + 1 // span past the closing `]`
	loc := bytecode.Loc{Line: line, EndLine: line, Col: objCol, EndCol: closeEnd}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BINARY_OP, Arg: uint32(bytecode.NbGetItem), Loc: loc,
	})
	return objCol, closeEnd, nil
}

// emitSubscriptStore emits IR for the subscript-target branch of
// `<obj>[<key>] = <val>`: recurse the RHS, recurse the object,
// recurse the slice, emit STORE_SUBSCR. Returns the store Loc the
// caller should anchor the trailing terminator on.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_stmt_assign
// (Subscript-target branch).
func emitSubscriptStore(u *compileUnit, s *ast.Subscript, value ast.Expr, line uint32) (bytecode.Loc, error) {
	if _, _, err := visitExpr(u, value, line); err != nil {
		return bytecode.Loc{}, err
	}
	objCol, _, err := visitExpr(u, s.Value, line)
	if err != nil {
		return bytecode.Loc{}, err
	}
	_, sliceEnd, err := visitExpr(u, s.Slice, line)
	if err != nil {
		return bytecode.Loc{}, err
	}
	closeEnd := sliceEnd + 1
	loc := bytecode.Loc{Line: line, EndLine: line, Col: objCol, EndCol: closeEnd}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.STORE_SUBSCR, Arg: 0, Loc: loc,
	})
	return loc, nil
}
