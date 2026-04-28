// Package codegen lowers a gocopy AST plus its symbol-table scope to
// a fully-formed *bytecode.CodeObject by emitting an InstrSeq IR and
// running it through the v0.6.5 assembler.
//
// At v0.6.6 codegen recognizes only the empty-module shape
// (`len(mod.Body) == 0`). Everything else returns ErrUnsupported and
// the caller falls back to the classifier. v0.6.7+ grow the
// supported shape set; v0.6.13 retires the classifier.
//
// SOURCE: CPython 3.14 Python/codegen.c.
package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// ErrUnsupported indicates the codegen package does not yet handle
// this source shape. Callers should fall back to whatever path
// drove compilation before codegen took over the shape.
var ErrUnsupported = errors.New("codegen: source shape not yet supported")

// Options carries the metadata fields the AST does not encode.
// Filename and Name end up in the CodeObject directly; FirstLineNo
// is the source line on which the code object's first instruction
// would appear in CPython terms (1 for module top level).
type Options struct {
	Filename    string
	Name        string
	QualName    string
	FirstLineNo int32
}

// Build emits a fully-formed CodeObject for the supported shape, or
// ErrUnsupported when the source falls outside the v0.6.6 codegen
// surface.
func Build(mod *ast.Module, scope *symtable.Scope, opts Options) (*bytecode.CodeObject, error) {
	if mod == nil {
		return nil, errors.New("codegen.Build: nil module")
	}
	_ = scope // reserved for v0.6.7 (function bodies need slot lookup)

	if len(mod.Body) == 0 {
		return buildEmptyModule(opts)
	}
	return nil, ErrUnsupported
}

// buildEmptyModule emits the synthetic prologue that CPython generates
// for a module whose body is empty (the file has no statements after
// the parser strips comments and blank lines):
//
//	RESUME 0
//	LOAD_CONST 0   ; None
//	RETURN_VALUE
//
// All three instructions share the synthetic-prologue Loc that the
// PEP 626 line-table encoder turns into a single LONG entry covering
// three code units. The decoded form is Loc{Line:0, EndLine:1}; the
// v0.6.5 round-trip across every fixture proved this re-encodes to
// the canonical `LineTableEmpty` byte sequence.
func buildEmptyModule(opts Options) (*bytecode.CodeObject, error) {
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: syntheticLoc},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: syntheticLoc},
	}

	return assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{},
	})
}
