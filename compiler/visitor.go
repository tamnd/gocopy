package compiler

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/codegen"
	"github.com/tamnd/gocopy/compiler/flowgraph"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// runVisitorShadow drives the v0.7.x visitor pipeline:
// codegen.Generate → optimize.Run → assemble.Assemble. It returns
// the resulting CodeObject, or nil on any pipeline error including
// codegen.ErrNotImplemented.
//
// At v0.7.1 the visitor knows modEmpty / modNoOps / modDocstring
// shapes; for other AST inputs it returns ErrNotImplemented and the
// helper returns nil so compiler.Compile can fall back to the
// classifier path.
func runVisitorShadow(mod *ast.Module, scope *symtable.Scope, source []byte, opts Options) *bytecode.CodeObject {
	if scope == nil {
		return nil
	}
	seq, consts, names, err := codegen.Generate(mod, scope, codegen.GenerateOptions{
		Source:      source,
		Filename:    opts.Filename,
		Name:        "<module>",
		QualName:    "<module>",
		FirstLineNo: 1,
	})
	if err != nil {
		return nil
	}
	// Single-driver post-codegen pipeline (spec 1573 Phase A): mirrors
	// CPython 3.14 Python/flowgraph.c:3659 _PyCfg_OptimizeCodeUnit.
	seq, consts = flowgraph.OptimizeCodeUnit(seq, consts)
	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     "<module>",
		QualName: "<module>",
		Consts:   consts,
		Names:    names,
	})
	if err != nil {
		return nil
	}
	return co
}
