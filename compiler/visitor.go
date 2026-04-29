package compiler

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/codegen"
	"github.com/tamnd/gocopy/compiler/optimize"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// runVisitorShadow drives the v0.7.x visitor pipeline:
// codegen.Generate → optimize.Run → assemble.Assemble. It returns
// the resulting CodeObject, or nil on any pipeline error including
// codegen.ErrNotImplemented.
//
// The helper exists so v0.7.1+ visitor arms light up automatically:
// each release that teaches codegen.Generate a new AST node category
// becomes visible end-to-end here without driver edits.
//
// The returned CodeObject is intentionally not authoritative.
// compiler.Compile returns the classifier path's CodeObject; the
// shadow's output is consumed only by TestVisitorParity, which
// re-invokes the same three calls with explicit error logging.
//
// At v0.7.0 every codegen.Generate call returns ErrNotImplemented
// and this helper always returns nil. That is the expected steady
// state for v0.7.0; v0.7.1 starts producing real CodeObjects here.
func runVisitorShadow(mod *ast.Module, scope *symtable.Scope, opts Options) *bytecode.CodeObject {
	if scope == nil {
		return nil
	}
	seq, err := codegen.Generate(mod, scope, codegen.GenerateOptions{
		Filename:    opts.Filename,
		Name:        "<module>",
		QualName:    "<module>",
		FirstLineNo: 1,
	})
	if err != nil {
		return nil
	}
	seq = optimize.Run(seq)
	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     "<module>",
		QualName: "<module>",
	})
	if err != nil {
		return nil
	}
	return co
}
