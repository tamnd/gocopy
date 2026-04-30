// Package preprocess ports CPython's _PyAST_Preprocess pass.
//
// MIRRORS: cpython/Python/ast_preprocess.c (CPython 3.14).
//
// The preprocess pass runs after parsing and before symtable build.
// It desugars f-strings, walrus operators, and PEP 695 type-parameter
// syntax into shapes the symtable + codegen passes can handle without
// special cases. CPython invokes it via _PyAST_Preprocess(mod, arena,
// filename, optimize, ff_features, syntax_check_only).
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package preprocess
