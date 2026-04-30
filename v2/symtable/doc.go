// Package symtable ports CPython's symtable construction pass.
//
// MIRRORS: cpython/Python/symtable.c (CPython 3.14).
//
// The symtable pass walks the AST twice: first to collect names and
// scopes (symtable_visit_*), then to resolve free/cell/global bindings
// (symtable_analyze). CPython exposes this as _PySymtable_Build and
// the resulting symtable is consumed by both codegen and the
// compile.c orchestrator.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package symtable
