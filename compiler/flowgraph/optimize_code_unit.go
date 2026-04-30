package flowgraph

import (
	"github.com/tamnd/gocopy/compiler/ir"
)

// OptimizeCodeUnit is the single entry point for the post-codegen
// optimizer pipeline. It mirrors CPython 3.14
// Python/flowgraph.c:3659 _PyCfg_OptimizeCodeUnit, which is the
// function compile.c calls per-scope after codegen finishes. Both the
// module-level visitor (compiler/visitor.go) and the child-unit
// driver (compiler/codegen/scope.go::popChildUnit) route through here.
//
// Pass order (verbatim from CPython, with notes on gocopy coverage):
//
//   - translate_jump_labels_to_targets — gocopy uses Block pointers
//     directly; this CPython pass has no analogue here. Once except
//     handlers land it will become a real port.
//   - mark_except_handlers / label_exception_targets — exception
//     machinery; spec 1573 Phase E.
//   - optimize_cfg → const-pool passes
//     (basicblock_optimize_load_const, fold_tuple_of_constants,
//     remove_unused_consts), eliminate_empty_basic_blocks,
//     inline_small_or_no_lineno_blocks. Spec 1573 Phase D ports the
//     full optimize_basic_block peephole.
//   - insert_superinstructions
//   - optimize_load_fast
//   - resolve unconditional jumps + linearise (gocopy's resolveJumps,
//     consolidated into flowgraph at v0.7.11 spec 1573 Phase A).
//
// Returns the (mutated) seq and the (possibly replaced) consts pool.
// The const passes may shrink the pool when entries become unused.
//
// SOURCE: CPython 3.14 Python/flowgraph.c:3659 _PyCfg_OptimizeCodeUnit.
func OptimizeCodeUnit(seq *ir.InstrSeq, consts []any) (*ir.InstrSeq, []any) {
	if seq == nil {
		return nil, consts
	}
	OptimizeLoadConst(seq, consts)
	consts = FoldTupleOfConstants(seq, consts)
	consts = RemoveUnusedConsts(seq, consts)
	eliminateEmptyBlocks(seq)
	removeUnreachable(seq)
	removeRedundantNops(seq)
	propagateLineNumbers(seq)
	inlineSmallExitBlocks(seq)
	InsertSuperinstructions(seq)
	OptimizeLoadFast(seq)
	resolveJumps(seq)
	return seq, consts
}

// Run is a shim that runs the post-codegen passes that operate on the
// CFG only (no const-pool transformations). It exists for tests that
// pre-date the OptimizeCodeUnit consolidation; new callers should use
// OptimizeCodeUnit, which takes the consts pool too.
func Run(seq *ir.InstrSeq) *ir.InstrSeq {
	if seq == nil {
		return nil
	}
	eliminateEmptyBlocks(seq)
	removeUnreachable(seq)
	removeRedundantNops(seq)
	propagateLineNumbers(seq)
	inlineSmallExitBlocks(seq)
	InsertSuperinstructions(seq)
	OptimizeLoadFast(seq)
	resolveJumps(seq)
	return seq
}
