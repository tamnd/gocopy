// Package optimize is the gocopy compiler's IR-level optimizer. It
// mirrors CPython 3.14 Python/flowgraph.c: a sequence of in-place
// passes over an *ir.InstrSeq that canonicalize the visitor's output
// into the byte form CPython's py_compile actually emits.
//
// At v0.7.6 Run runs three passes:
//
//   - eliminateEmptyBlocks drops empty labelled blocks by
//     retargeting jumps to the block's first non-empty successor
//     (or by transferring the empty's label when the successor has
//     no label). Narrow specialisation of CPython's
//     eliminate_empty_basic_blocks. The v0.7.6 visit_If shapes
//     emit no empty bridging blocks (per-branch implicit-return
//     emit terminates every branch directly), so the pass is a
//     no-op here; it lands now so future shapes that DO produce
//     empty bridges have it ready.
//   - inlineSmallExitBlocks duplicates a small RETURN_VALUE-tail
//     block into every predecessor reaching it via fall-through or
//     unconditional JUMP_FORWARD, then removes the merge block.
//     Verbatim clone of the v0.7.4 / v0.7.5 implementation —
//     visit_If's per-branch tail emit means the inliner sees no
//     candidate merge blocks for v0.7.6 fixtures, so the pass is
//     a no-op for those; v0.7.4 BoolOp / IfExp shapes still
//     exercise it. Mirrors CPython's
//     inline_small_or_no_lineno_blocks.
//   - resolveJumps rewrites every jump instruction's Arg from a
//     LabelID into the byte-distance form ir.Encode consumes, then
//     linearises the multi-block IR into a single flat block.
//     Combines CPython's resolve_unconditional_jumps with the
//     relocation half of _PyAssemble_ResolveJumps.
//
// Both passes are no-ops on single-block / decoder-shape input, so
// the existing IR / flowgraph / assemble round-trip tests stay
// green.
//
// The remaining passes (optimize_cfg, optimize_load_fast,
// insert_superinstructions, fold_constants,
// add_checks_for_loads_of_uninitialized_variables,
// trim_unused_consts) land one per release across v0.7.13..v0.7.19,
// in the same order CPython runs them.
//
// SOURCE: CPython 3.14 Python/flowgraph.c.
package optimize

import (
	"github.com/tamnd/gocopy/compiler/flowgraph"
	"github.com/tamnd/gocopy/compiler/ir"
)

// Run applies the configured optimizer passes to seq in place and
// returns it.
//
// Returning the same pointer (rather than a fresh tree) is
// deliberate and matches CPython's flowgraph.c: passes mutate the
// CFG owned by the compiler driver and the driver discards it after
// assembly.
//
// Pipeline order (gocopy approximation of CPython 3.14
// _PyCfg_OptimizeCodeUnit, Python/flowgraph.c:3659):
//
//  1. eliminateEmptyBlocks — drops empty labelled blocks.
//  2. inlineSmallExitBlocks — duplicates RETURN_VALUE-tail blocks.
//  3. flowgraph.InsertSuperinstructions — fuses LOAD_FAST/STORE_FAST
//     pairs into super-instructions. v0.7.10.3 ports
//     Python/flowgraph.c:2588 insert_superinstructions.
//  4. resolveJumps — relocates labels to byte distances and flattens
//     to a single block.
func Run(seq *ir.InstrSeq) *ir.InstrSeq {
	if seq == nil {
		return nil
	}
	eliminateEmptyBlocks(seq)
	inlineSmallExitBlocks(seq)
	flowgraph.InsertSuperinstructions(seq)
	resolveJumps(seq)
	return seq
}
