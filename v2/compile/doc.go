// Package compile ports CPython's top-level compiler driver.
//
// MIRRORS: cpython/Python/compile.c (CPython 3.14).
//
// Orchestrates the pipeline: _PyFuture_FromAST → _PyAST_Preprocess →
// _PySymtable_Build → _PyCodegen_Module → _PyCfg_OptimizeCodeUnit →
// _PyAssemble_MakeCodeObject. CPython exposes the entry as
// _PyAST_Compile and that's the public surface of this package.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package compile
