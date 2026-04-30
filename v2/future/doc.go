// Package future ports CPython's __future__ feature scanner.
//
// MIRRORS: cpython/Python/future.c (CPython 3.14).
//
// Walks the AST head and collects the bitmask of `from __future__
// import …` directives. CPython exposes this as _PyFuture_FromAST and
// stores the result in PyFutureFeatures.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package future
