// Package dump renders intermediate compiler stages as text for
// side-by-side diffing against CPython.
//
// Stages: ast, preprocess, symtable, codegen-instrseq, flowgraph
// (pre-opt and post-opt), assemble (final code object). The matching
// CPython-side dump scripts live under tests/scripts/ and emit the
// same text format so a stage-mismatch can be located by `diff -u`.
//
// Public entry points: Dump<Stage>(w io.Writer, x StageInput) plus
// CLI hooks invoked by `gocopy compile --dump-stage=<stage>`.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package dump
