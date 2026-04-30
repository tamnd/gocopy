package flowgraph

import (
	"github.com/tamnd/gocopy/v1/ir"
)

// OptimizeCodeUnit is the single entry point for the post-codegen
// optimizer pipeline. It mirrors CPython 3.14
// Python/flowgraph.c:3659 _PyCfg_OptimizeCodeUnit, which is the
// function compile.c calls per-scope after codegen finishes. Both the
// module-level visitor (compiler/visitor.go) and the child-unit
// driver (compiler/codegen/scope.go::popChildUnit) route through here.
//
// Pass order is verbatim from CPython 3.14:
//
//	int
//	_PyCfg_OptimizeCodeUnit(cfg_builder *g, ...)             // line 3659
//	{
//	    // Preprocessing — gocopy uses Block pointers directly,
//	    // so translate_jump_labels_to_targets / mark_except_handlers /
//	    // label_exception_targets have no analogue yet (Phase E).
//	    optimize_cfg(g, consts, ...);                          // line 3691
//	    remove_unused_consts(g->g_entryblock, consts);         // line 3697
//	    // add_checks_for_loads_of_uninitialized_variables     // line 3699 — not ported
//	    insert_superinstructions(g);                           // line 3701
//	    // push_cold_blocks_to_end(g);                         // line 3703 — Phase E
//	    resolve_line_numbers(g, firstlineno);                  // line 3704
//	}
//
//	int
//	optimize_cfg(cfg_builder *g, ...)                         // line 2552
//	{
//	    inline_small_or_no_lineno_blocks(g->g_entryblock);    // line 2557
//	    remove_unreachable(g->g_entryblock);                   // line 2558
//	    resolve_line_numbers(g, firstlineno);                  // line 2559
//	    optimize_load_const(...);                              // line 2560
//	    for (basicblock *b = ...; b; b = b->b_next) {
//	        optimize_basic_block(...);                         // line 2562 — Phase D
//	    }
//	    remove_redundant_nops_and_pairs(g->g_entryblock);     // line 2564
//	    remove_unreachable(g->g_entryblock);                   // line 2565
//	    remove_redundant_nops_and_jumps(g);                    // line 2566 — not ported
//	}
//
// optimize_load_fast and the label/offset resolution (gocopy's
// resolveJumps) are part of CPython's
// _PyCfg_OptimizedCfgToInstructionSequence (flowgraph.c:4025), which
// runs after _PyCfg_OptimizeCodeUnit. gocopy invokes them inline here
// because the caller (codegen/scope.go::popChildUnit) immediately
// hands off to ir.Encode / assemble.Assemble — there is no separate
// instruction-sequence stage yet.
//
// FoldTupleOfConstants is part of CPython's optimize_basic_block
// peephole (Phase D). Until that pass lands it is hoisted up next to
// optimize_load_const so the const-pool dedupe in remove_unused_consts
// sees the folded entries.
//
// Returns the (mutated) seq and the (possibly replaced) consts pool.
// The const passes may shrink the pool when entries become unused.
//
// SOURCE: CPython 3.14 Python/flowgraph.c:3659 _PyCfg_OptimizeCodeUnit
// + Python/flowgraph.c:2552 optimize_cfg + Python/flowgraph.c:4025
// _PyCfg_OptimizedCfgToInstructionSequence.
func OptimizeCodeUnit(seq *ir.InstrSeq, consts []any) (*ir.InstrSeq, []any) {
	if seq == nil {
		return nil, consts
	}
	// optimize_cfg (flowgraph.c:2552)
	inlineSmallOrNoLinenoBlocks(seq)           // line 2557
	removeUnreachable(seq)                     // line 2558
	propagateLineNumbers(seq)                  // line 2559
	OptimizeLoadConst(seq, consts)             // line 2560
	consts = FoldTupleOfConstants(seq, consts) // line 2562 (part of optimize_basic_block)
	removeRedundantNops(seq)                   // line 2564
	removeUnreachable(seq)                     // line 2565
	// _PyCfg_OptimizeCodeUnit tail (flowgraph.c:3659)
	consts = RemoveUnusedConsts(seq, consts) // line 3697
	InsertSuperinstructions(seq)             // line 3701
	// _PyCfg_OptimizedCfgToInstructionSequence (flowgraph.c:4025)
	OptimizeLoadFast(seq) // line 4062
	resolveJumps(seq)     // _PyCfg_ToInstructionSequence (line 4065)
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
	inlineSmallOrNoLinenoBlocks(seq)
	removeUnreachable(seq)
	propagateLineNumbers(seq)
	removeRedundantNops(seq)
	removeUnreachable(seq)
	InsertSuperinstructions(seq)
	OptimizeLoadFast(seq)
	resolveJumps(seq)
	return seq
}
