package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitConstant emits LOAD_SMALL_INT (for int 0..255) or LOAD_CONST
// for an *ast.Constant in expression context. Phantom-slot logic is
// applied so the unit's first const-emit claims co_consts[0] even
// when LOAD_SMALL_INT replaces the LOAD_CONST. Mirrors CPython 3.14
// Python/codegen.c::codegen_visit_expr_constant + the
// LOAD_SMALL_INT specialization in compile.c.
func visitConstant(u *compileUnit, c *ast.Constant, loc bytecode.Loc) error {
	v, ok := constantValue(c)
	if !ok {
		return ErrNotImplemented
	}
	emitConstLoad(u, v, loc)
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
	case "int", "float", "complex", "str", "bytes":
		return c.Value, true
	case "Ellipsis":
		return bytecode.Ellipsis, true
	default:
		return nil, false
	}
}

// emitConstLoad emits LOAD_SMALL_INT for an int64 in 0..255 (claiming
// the phantom co_consts[0] slot when this is the unit's first const-
// emit), or LOAD_CONST for any other value. After return,
// u.phantomDone is true.
func emitConstLoad(u *compileUnit, v any, loc bytecode.Loc) {
	block := u.currentBlock()
	if iv, ok := v.(int64); ok && iv >= 0 && iv <= 255 {
		if !u.phantomDone {
			u.addConst(iv)
			u.phantomDone = true
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
	u.phantomDone = true
}

// emitDeferredConstLoad emits a LOAD_CONST whose arg is rewritten in
// finalizeDeferred to the index of value once None has consumed its
// post-phantom slot. Used for compile-time-folded results
// (BinOp(Const,Const), UnaryOp(USub,Const)) that don't fit
// LOAD_SMALL_INT and must land in co_consts AFTER None to mirror
// CPython 3.14's Python/compile.c constant-folding pass.
func emitDeferredConstLoad(u *compileUnit, value any, loc bytecode.Loc) {
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: bytecode.LOAD_CONST, Arg: 0, Loc: loc,
	})
	u.deferredPatches = append(u.deferredPatches, deferredPatch{
		block:    block,
		instrIdx: len(block.Instrs) - 1,
		value:    value,
	})
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
