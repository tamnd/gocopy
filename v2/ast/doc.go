// Package ast is the v2 alias surface for the parser AST.
//
// Mirrors v1/ast — re-exports the gopapy/parser AST node types under
// short Go-idiomatic names so codegen/symtable/etc. can speak ast.X
// without importing the parser package directly. The parser-import
// boundary is enforced by a guard test (parser_boundary_test.go at
// the v2 root once the orchestrator lands).
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package ast
