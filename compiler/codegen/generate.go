package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// ErrNotImplemented signals that the visitor pipeline does not yet
// emit IR for this AST node. Distinct from ErrUnsupported (which
// belongs to the legacy Build path), so callers can tell the two
// transition paths apart while v0.7.x ratchets visitor coverage up.
//
// At v0.7.0 every non-empty module returns ErrNotImplemented. v0.7.1
// starts lighting up arms.
var ErrNotImplemented = errors.New("codegen: visitor not yet implemented for this AST node")

// GenerateOptions carries the metadata fields the AST does not encode
// for the visitor pipeline. Kept separate from the legacy Options
// struct so the legacy path can be deleted at v0.7.26 without a
// signature break here.
type GenerateOptions struct {
	Filename    string
	Name        string
	QualName    string
	FirstLineNo int32
}

// compileUnit is the per-frame state CPython 3.14 carries on its
// compiler scope stack (Python/compile.c::compiler_unit). Every code
// object — module, function, class, lambda, comprehension — lowers to
// one compileUnit. Nested scopes push a child unit; on exit, the
// child's Seq becomes a const-pool entry in the parent.
//
// At v0.7.0 only the Module unit is allocated and only the field
// initialization is exercised; visitor arms in v0.7.1+ populate the
// Seq, Consts, and Names tables on the way down.
type compileUnit struct {
	Scope *symtable.Scope
	Seq   *ir.InstrSeq

	Consts []any
	Names  []string

	Name        string
	QualName    string
	FirstLineNo int32

	Parent *compileUnit
}

// newCompileUnit allocates a fresh per-frame compileUnit linked to
// scope. Parent is nil for module-level units.
func newCompileUnit(scope *symtable.Scope, name, qualName string, firstLineNo int32, parent *compileUnit) *compileUnit {
	return &compileUnit{
		Scope:       scope,
		Seq:         ir.NewInstrSeq(),
		Name:        name,
		QualName:    qualName,
		FirstLineNo: firstLineNo,
		Parent:      parent,
	}
}

// Generate is the visitor pipeline's entry point. It builds a
// compileUnit for the module, dispatches into the visit_<node>
// family, and returns the populated InstrSeq for the caller to feed
// into compiler/optimize.Run and compiler/assemble.Assemble.
//
// At v0.7.0 the visitor returns ErrNotImplemented for every input
// (including empty modules). Subsequent v0.7.x releases light up
// arms one AST node category at a time, mirroring CPython 3.14
// Python/codegen.c::compiler_visit_stmt and compiler_visit_expr1.
//
// SOURCE: CPython 3.14 Python/compile.c::_PyCompile_CodeGen.
func Generate(mod *ast.Module, scope *symtable.Scope, opts GenerateOptions) (*ir.InstrSeq, error) {
	if mod == nil {
		return nil, errors.New("codegen.Generate: nil module")
	}
	if scope == nil {
		return nil, errors.New("codegen.Generate: nil scope")
	}
	u := newCompileUnit(scope, opts.Name, opts.QualName, opts.FirstLineNo, nil)
	u.Seq.FirstLineNo = opts.FirstLineNo
	if err := visitModule(u, mod); err != nil {
		return nil, err
	}
	return u.Seq, nil
}

// visitModule is the top-level AST walker entrypoint. v0.7.0
// returns ErrNotImplemented unconditionally; v0.7.1 starts handling
// the empty / no-op / docstring shapes via visit_Constant /
// visit_Name and an explicit visit over mod.Body.
//
// SOURCE: CPython 3.14 Python/codegen.c::compiler_codegen.
func visitModule(u *compileUnit, mod *ast.Module) error {
	_ = u
	_ = mod
	return ErrNotImplemented
}
