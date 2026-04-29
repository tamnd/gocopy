package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/optimize"
)

// pushChildUnit allocates and returns a fresh compileUnit linked to u
// as Parent. The child inherits u.Source and u.Filename so the
// per-instruction Loc resolution and assembled CodeObject metadata
// work without re-plumbing. Scope is left nil at v0.7.8 — the
// no-closure single-arg shape needs neither free nor cell flags;
// v0.7.9 wires the symtable child scope through here when closures
// arrive.
//
// SOURCE: CPython 3.14 Python/compile.c::compiler_enter_scope.
func (u *compileUnit) pushChildUnit(name, qualName string, firstLineNo int32) *compileUnit {
	child := newCompileUnit(nil, name, qualName, firstLineNo, u)
	child.Seq.FirstLineNo = firstLineNo
	child.Source = u.Source
	child.Filename = u.Filename
	return child
}

// popChildUnit finalizes child by running optimize.Run on its Seq and
// assemble.Assemble with the supplied options. Returns the resulting
// CodeObject. The child is discarded after this call; the caller is
// responsible for appending the CodeObject to its own const pool.
//
// opts.Consts and opts.Names are overwritten with the child's
// unit-level tables.
//
// SOURCE: CPython 3.14 Python/compile.c::compiler_exit_scope.
func (u *compileUnit) popChildUnit(child *compileUnit, opts assemble.Options) (*bytecode.CodeObject, error) {
	child.finalizeDeferred()
	seq := optimize.Run(child.Seq)
	opts.Consts = child.Consts
	if opts.Consts == nil {
		opts.Consts = []any{}
	}
	opts.Names = child.Names
	if opts.Names == nil {
		opts.Names = []string{}
	}
	return assemble.Assemble(seq, opts)
}
