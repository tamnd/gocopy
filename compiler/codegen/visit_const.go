package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitConstant emits LOAD_CONST for an *ast.Constant in expression
// context. The constant is registered in the unit's const pool via
// addConst (dedup-by-equality); the index is encoded as the LOAD_CONST
// arg. Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr_constant.
//
// At v0.7.1 visitConstant is not called from any expression context
// directly (constant ExprStmts are handled as NOPs in visitStmt and
// the docstring is handled inline in visitModule). The helper is wired
// here so v0.7.2's visit_Assign can call it without re-shaping
// dispatch.
func visitConstant(u *compileUnit, c *ast.Constant, loc bytecode.Loc) error {
	v, ok := constantValue(c)
	if !ok {
		return ErrNotImplemented
	}
	idx := u.addConst(v)
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: bytecode.LOAD_CONST, Arg: idx, Loc: loc,
	})
	return nil
}

// constantValue maps an *ast.Constant to the Go-typed value the
// const pool stores. Returns false for kinds the visitor does not yet
// support; the caller is expected to bubble up ErrNotImplemented.
func constantValue(c *ast.Constant) (any, bool) {
	switch c.Kind {
	case "None":
		return nil, true
	case "True":
		return true, true
	case "False":
		return false, true
	case "int", "float", "complex", "str", "bytes", "Ellipsis":
		return c.Value, true
	default:
		return nil, false
	}
}

// currentBlock returns the most recently added Block, allocating one
// if Seq has none. Helper for visitor arms that emit instructions
// without owning block lifecycle.
func (u *compileUnit) currentBlock() *ir.Block {
	if len(u.Seq.Blocks) == 0 {
		return u.Seq.AddBlock()
	}
	return u.Seq.Blocks[len(u.Seq.Blocks)-1]
}
