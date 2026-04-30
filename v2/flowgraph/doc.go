// Package flowgraph ports CPython's CFG construction + optimisation passes.
//
// MIRRORS: cpython/Python/flowgraph.c (CPython 3.14).
//
// Builds a basicblock-graph from an instruction_sequence and runs the
// 12-pass optimiser (optimize_basic_block, propagate_line_numbers,
// remove_unreachable_basic_blocks, inline_small_or_no_lineno_blocks,
// optimize_load_const, optimize_load_fast, …). CPython exposes the
// entry as _PyCfg_OptimizeCodeUnit.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package flowgraph
