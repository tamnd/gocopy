// Package lower is the v2 parser→ast translation boundary.
//
// Mirrors v1/lower — converts the gopapy participle parse tree into
// the ast.* node types that the rest of the compiler consumes. Like
// v1/lower this is one of the only two production files allowed to
// import gopapy/parser.
//
// Spec: ~/notes/Spec/1500/ (per-port spec assigned when this package
// lands; placeholder until then).
package lower
