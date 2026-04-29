// Package optimize is the gocopy compiler's IR-level optimizer. It
// mirrors CPython 3.14 Python/flowgraph.c: a sequence of in-place
// passes over an *ir.InstrSeq that canonicalize the visitor's output
// into the byte form CPython's py_compile actually emits.
//
// At v0.7.4 Run runs two passes:
//
//   - inlineSmallExitBlocks duplicates a small RETURN_VALUE-tail
//     block into every predecessor reaching it via fall-through or
//     unconditional JUMP_FORWARD, then drops the merge block. A
//     narrow specialisation of CPython's
//     inline_small_or_no_lineno_blocks; the visitor's IfExp shape
//     needs it.
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

import "github.com/tamnd/gocopy/compiler/ir"

// Run applies the configured optimizer passes to seq in place and
// returns it.
//
// Returning the same pointer (rather than a fresh tree) is
// deliberate and matches CPython's flowgraph.c: passes mutate the
// CFG owned by the compiler driver and the driver discards it after
// assembly.
func Run(seq *ir.InstrSeq) *ir.InstrSeq {
	if seq == nil {
		return nil
	}
	inlineSmallExitBlocks(seq)
	resolveJumps(seq)
	return seq
}
