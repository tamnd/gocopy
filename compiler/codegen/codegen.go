// Package codegen lowers a gocopy AST plus its symbol-table scope to
// a fully-formed *bytecode.CodeObject by emitting an InstrSeq IR and
// running it through the v0.6.5 assembler.
//
// At v0.6.6 codegen owned the empty-module shape. v0.6.7 extends to
// modules whose body is N >= 1 no-op statements (`pass`, non-string
// `Constant` expression statements). Anything else returns
// ErrUnsupported and the caller falls back to the classifier.
//
// SOURCE: CPython 3.14 Python/codegen.c.
package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// ErrUnsupported indicates the codegen package does not yet handle
// this source shape. Callers should fall back to whatever path
// drove compilation before codegen took over the shape.
var ErrUnsupported = errors.New("codegen: source shape not yet supported")

// Options carries the metadata fields the AST does not encode plus
// the raw source bytes the no-op shape needs for end-of-line column
// calculation (the parser AST does not yet record end positions).
type Options struct {
	Source      []byte
	Filename    string
	Name        string
	QualName    string
	FirstLineNo int32
}

// Build emits a fully-formed CodeObject for the supported shape, or
// ErrUnsupported when the source falls outside the v0.6.7 codegen
// surface.
func Build(mod *ast.Module, scope *symtable.Scope, opts Options) (*bytecode.CodeObject, error) {
	if mod == nil {
		return nil, errors.New("codegen.Build: nil module")
	}
	_ = scope // reserved for the function-codegen release

	if len(mod.Body) == 0 {
		return buildEmptyModule(opts)
	}
	if stmts, ok := classifyNoOpModule(mod, opts.Source); ok {
		return buildNoOpsModule(stmts, opts)
	}
	return nil, ErrUnsupported
}
