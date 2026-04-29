package compiler

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/codegen"
	"github.com/tamnd/gocopy/compiler/flowgraph"
	"github.com/tamnd/gocopy/compiler/optimize"
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
	// Run the same const-pool passes the child-unit pipeline does, so
	// module-level def-statements with default arguments fold their
	// BUILD_TUPLE chains identically to CPython.
	flowgraph.OptimizeLoadConst(seq, consts)
	consts = flowgraph.FoldTupleOfConstants(seq, consts)
	consts = flowgraph.RemoveUnusedConsts(seq, consts)
	seq = optimize.Run(seq)
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
