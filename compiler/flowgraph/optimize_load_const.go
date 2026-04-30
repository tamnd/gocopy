package flowgraph

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// OptimizeLoadConst rewrites every LOAD_CONST whose pool entry is an
// int in [0, 255] into LOAD_SMALL_INT with the value embedded in the
// oparg. The const-pool entry is left in place; remove_unused_consts
// is the pass that condenses the pool afterwards.
//
// Out of scope here (split into later v0.7.10.x sub-releases):
//
//   - The LOAD_CONST + COPY-of-LOAD_CONST + POP_JUMP_IF_* / IS_OP /
//     CONTAINS_OP fold blocks in basicblock_optimize_load_const.
//     Those reach into peephole territory the visitor does not
//     produce yet.
//   - The fold_const_unaryop / fold_const_binop /
//     fold_tuple_of_constants passes, which run alongside this one
//     in CPython's optimize_basic_block.
//
// SOURCE: CPython 3.14 Python/flowgraph.c:2169
// basicblock_optimize_load_const (LOAD_SMALL_INT-rewriting half)
// + Python/flowgraph.c:1408 maybe_instr_make_load_smallint.
func OptimizeLoadConst(seq *ir.InstrSeq, consts []any) {
	if seq == nil {
		return
	}
	for _, b := range seq.Blocks {
		for i := range b.Instrs {
			inst := &b.Instrs[i]
			if inst.Op != bytecode.LOAD_CONST {
				continue
			}
			if int(inst.Arg) >= len(consts) {
				continue
			}
			maybeInstrMakeLoadSmallInt(inst, consts[inst.Arg])
		}
	}
}

// maybeInstrMakeLoadSmallInt rewrites inst from LOAD_CONST idx to
// LOAD_SMALL_INT v when v is an int in [0, 255]. Returns true when
// the rewrite happens.
//
// SOURCE: CPython 3.14 Python/flowgraph.c:1408
// maybe_instr_make_load_smallint.
func maybeInstrMakeLoadSmallInt(inst *ir.Instr, v any) bool {
	iv, ok := v.(int64)
	if !ok {
		return false
	}
	if iv < 0 || iv > 255 {
		return false
	}
	inst.Op = bytecode.LOAD_SMALL_INT
	inst.Arg = uint32(iv)
	return true
}
