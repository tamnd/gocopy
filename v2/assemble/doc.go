// Package assemble ports CPython's bytecode assembly pass.
//
// MIRRORS: cpython/Python/assemble.c (CPython 3.14).
//
// Lowers a fully optimised CFG into a packed PyCodeObject: emits the
// co_code byte array, the line table, exception table, free/cell
// variable tuples, and the inline-cache slots that CPython's
// specialiser reads. CPython exposes the entry as
// _PyAssemble_MakeCodeObject.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package assemble
