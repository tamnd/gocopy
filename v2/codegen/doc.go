// Package codegen ports CPython's AST→pseudo-bytecode codegen pass.
//
// MIRRORS: cpython/Python/codegen.c (CPython 3.14).
//
// Walks the AST in arm-per-kind dispatchers (codegen_visit_stmt,
// codegen_visit_expr) and emits pseudo-instructions into an
// instruction_sequence. CPython exposes the entry as _PyCodegen_Module;
// per-shape helpers (codegen_if, codegen_for, codegen_jump_if, …)
// each get a 1:1 Go counterpart with the upstream name preserved.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package codegen
