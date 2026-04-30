package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
)

// visitName emits a name reference at module scope. Mirrors
// CPython 3.14 Python/codegen.c::codegen_visit_expr_name for the Load
// context.
//
// At module scope (the only scope handled at v0.7.1) every name is a
// Global per the symtable, so Load emits LOAD_NAME. Function-scope
// dispatch (LOAD_FAST / LOAD_FAST_BORROW / LOAD_DEREF) lands with the
// function-body visitor in v0.7.8/v0.7.9.
//
// Store / Del contexts are not yet handled: they wire up via
// visit_Assign in v0.7.2.
func visitName(u *compileUnit, n *ast.Name, loc bytecode.Loc) error {
	idx := u.addName(n.Id)
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: bytecode.LOAD_NAME, Arg: idx, Loc: loc,
	})
	return nil
}
