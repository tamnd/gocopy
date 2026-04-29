// Package optimize is the gocopy compiler's IR-level optimizer. It
// mirrors CPython 3.14 Python/flowgraph.c: a sequence of in-place
// passes over an *ir.InstrSeq that canonicalize the visitor's output
// into the byte form CPython's py_compile actually emits.
//
// At v0.7.0 the package is a no-op skeleton: Run returns its input
// unchanged. The seven passes (optimize_cfg, optimize_load_fast,
// insert_superinstructions, fold_constants,
// resolve_unconditional_jumps,
// add_checks_for_loads_of_uninitialized_variables,
// trim_unused_consts) land one per release across v0.7.13..v0.7.19,
// in the same order CPython runs them.
//
// SOURCE: CPython 3.14 Python/flowgraph.c.
package optimize

import "github.com/tamnd/gocopy/compiler/ir"

// Run applies the configured optimizer passes to seq in place and
// returns it. At v0.7.0 the function is the identity; subsequent
// releases populate it with real passes.
//
// Returning the same pointer (rather than a fresh tree) is
// deliberate and matches CPython's flowgraph.c: passes mutate the
// CFG owned by the compiler driver and the driver discards it after
// assembly.
func Run(seq *ir.InstrSeq) *ir.InstrSeq {
	return seq
}
