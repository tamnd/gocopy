package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// ErrNotImplemented signals that the visitor pipeline does not yet
// emit IR for this AST node. Distinct from ErrUnsupported (which
// belongs to the legacy Build path), so callers can tell the two
// transition paths apart while v0.7.x ratchets visitor coverage up.
var ErrNotImplemented = errors.New("codegen: visitor not yet implemented for this AST node")

// GenerateOptions carries the metadata fields the AST does not encode
// for the visitor pipeline. Kept separate from the legacy Options
// struct so the legacy path can be deleted at v0.7.26 without a
// signature break here.
//
// Source is the raw source bytes; the visitor consumes it for
// end-of-line column lookup the same way the classifier does.
type GenerateOptions struct {
	Source      []byte
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
type compileUnit struct {
	Scope *symtable.Scope
	Seq   *ir.InstrSeq

	Consts []any
	Names  []string

	Source []byte

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

// addConst returns the index of v in u.Consts, appending it if it
// isn't already there. Mirrors CPython's compiler_add_const_o
// dedup-by-equality semantics: nil is unique, and other values dedup
// when their Go type matches and they are ==-equal. Boxed types like
// *big.Int that don't compare with == still dedup correctly because
// CPython's pool is keyed on Python identity for those — for v0.7.1
// the only callers append nil and string, both of which == as
// expected.
func (u *compileUnit) addConst(v any) uint32 {
	for i, c := range u.Consts {
		if c == nil && v == nil {
			return uint32(i)
		}
		if c == nil || v == nil {
			continue
		}
		if c == v {
			return uint32(i)
		}
	}
	u.Consts = append(u.Consts, v)
	return uint32(len(u.Consts) - 1)
}

// addName returns the index of s in u.Names, appending it if it
// isn't already there.
func (u *compileUnit) addName(s string) uint32 {
	for i, n := range u.Names {
		if n == s {
			return uint32(i)
		}
	}
	u.Names = append(u.Names, s)
	return uint32(len(u.Names) - 1)
}

// Generate is the visitor pipeline's entry point. It builds a
// compileUnit for the module, dispatches into the visit_<node>
// family, and returns the populated InstrSeq for the caller to feed
// into compiler/optimize.Run and compiler/assemble.Assemble.
//
// The Consts and Names tables populated on the unit are exposed via
// the Generate*Result helpers for assemble.Assemble.
//
// SOURCE: CPython 3.14 Python/compile.c::_PyCompile_CodeGen.
func Generate(mod *ast.Module, scope *symtable.Scope, opts GenerateOptions) (*ir.InstrSeq, []any, []string, error) {
	if mod == nil {
		return nil, nil, nil, errors.New("codegen.Generate: nil module")
	}
	if scope == nil {
		return nil, nil, nil, errors.New("codegen.Generate: nil scope")
	}
	u := newCompileUnit(scope, opts.Name, opts.QualName, opts.FirstLineNo, nil)
	u.Seq.FirstLineNo = opts.FirstLineNo
	u.Source = opts.Source
	if err := visitModule(u, mod); err != nil {
		return nil, nil, nil, err
	}
	consts := u.Consts
	if consts == nil {
		consts = []any{}
	}
	names := u.Names
	if names == nil {
		names = []string{}
	}
	return u.Seq, consts, names, nil
}

// visitModule is the top-level AST walker. It mirrors
// CPython 3.14 Python/codegen.c::compiler_codegen for module bodies:
// emit the synthetic RESUME prologue; if body[0] is a string-literal
// docstring, emit LOAD_CONST + STORE_NAME __doc__; walk the
// remaining statements emitting one NOP per stmt; then emit
// LOAD_CONST None + RETURN_VALUE at the last source position.
//
// Per CPython 3.14, the unoptimized form would emit one NOP per
// constant ExprStmt and Pass, plus a synthetic LOAD_CONST None +
// RETURN_VALUE at the end. CPython's optimize_cfg pass folds the
// trailing NOP into the LOAD_CONST/RETURN_VALUE pair (preserving
// line info on the last source line). v0.7.1 has no optimizer pass
// yet, so we emit the post-fold pattern directly: NOP for every
// non-last stmt, and reuse the last stmt's Loc on the
// LOAD_CONST None + RETURN_VALUE pair. v0.7.13 (optimize_cfg)
// generalizes this fold and removes the special case here.
//
// Returns ErrNotImplemented for any module body shape outside
// modEmpty / modNoOps / modDocstring.
func visitModule(u *compileUnit, mod *ast.Module) error {
	block := u.Seq.AddBlock()
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}

	block.Instrs = append(block.Instrs, ir.Instr{
		Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc,
	})

	if len(mod.Body) == 0 {
		idx := u.addConst(nil)
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: idx, Loc: syntheticLoc},
			ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: syntheticLoc},
		)
		return nil
	}

	bodyStart := 0
	var lastLoc bytecode.Loc
	if doc, ok := isModuleDocstring(mod.Body[0], u.Source); ok {
		textIdx := u.addConst(doc.Text)
		nameIdx := u.addName("__doc__")
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: textIdx, Loc: doc.Loc},
			ir.Instr{Op: bytecode.STORE_NAME, Arg: nameIdx, Loc: doc.Loc},
		)
		lastLoc = doc.Loc
		bodyStart = 1
	}

	tail := mod.Body[bodyStart:]
	for i, stmt := range tail {
		loc, err := stmtNopLoc(stmt, u.Source)
		if err != nil {
			return err
		}
		if i == len(tail)-1 {
			lastLoc = loc
			break
		}
		block.Instrs = append(block.Instrs, ir.Instr{
			Op: bytecode.NOP, Arg: 0, Loc: loc,
		})
	}

	noneIdx := u.addConst(nil)
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: lastLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: lastLoc},
	)
	return nil
}
