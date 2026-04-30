// Package instrseq ports CPython's pseudo-instruction sequence type.
//
// MIRRORS: cpython/Python/instruction_sequence.c (CPython 3.14).
//
// instruction_sequence is the linear pre-CFG instruction list that
// codegen emits into and that compile.c later folds into a flowgraph.
// CPython exposes _PyInstructionSequence_New / _Append /
// _ApplyLabelMap and friends.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package instrseq
