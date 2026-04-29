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

// deferredPatch records a LOAD_CONST instruction whose Arg must be
// rewritten at module finalize: instructions emitted while folding
// BinOp(Const,op,Const) or UnaryOp(USub,numeric Const) point to a
// const-pool slot that lands AFTER None, so the index is unknown at
// emit time.
type deferredPatch struct {
	block    *ir.Block
	instrIdx int
	value    any
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

	// phantomDone tracks whether co_consts[0] has been claimed by
	// the first const-emitting expression in this unit. CPython
	// 3.14 leaves the first const a module body emits at index 0
	// even when LOAD_SMALL_INT replaces the actual LOAD_CONST.
	// v0.7.16's optimize_cfg pass replaces this in-visitor logic
	// with the canonical post-CFG fold pass.
	phantomDone bool

	// deferredPatches collects LOAD_CONST instructions whose Arg
	// must be rewritten once the final const-pool index of the
	// referenced value is known. Folded results from
	// BinOp(Const,op,Const) and UnaryOp(USub,numeric) that don't
	// fit LOAD_SMALL_INT land in co_consts AFTER None; the
	// placeholder Arg is rewritten in finalizeDeferred.
	deferredPatches []deferredPatch
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
// CPython's pool is keyed on Python identity for those — for v0.7.2
// the only callers append nil, int64, float64, complex128, string,
// []byte, bool, and bytecode.EllipsisType. []byte deduplication uses
// content equality.
func (u *compileUnit) addConst(v any) uint32 {
	if bv, ok := v.([]byte); ok {
		for i, c := range u.Consts {
			if cb, ok2 := c.([]byte); ok2 && bytesEqual(bv, cb) {
				return uint32(i)
			}
		}
		u.Consts = append(u.Consts, v)
		return uint32(len(u.Consts) - 1)
	}
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

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

// finalizeDeferred resolves every LOAD_CONST instruction queued in
// u.deferredPatches by appending its target value to u.Consts (with
// dedup) and rewriting the instruction's Arg to the resulting index.
// The unique-by-==-equality dedup matches u.addConst.
func (u *compileUnit) finalizeDeferred() {
	if len(u.deferredPatches) == 0 {
		return
	}
	for _, dp := range u.deferredPatches {
		idx := u.addConst(dp.value)
		dp.block.Instrs[dp.instrIdx].Arg = idx
	}
	u.deferredPatches = nil
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

// checkBodyShape rejects module bodies whose stmt ordering does not
// match the classifier's accepted shapes at v0.7.2. Specifically:
//
//   - A no-op statement (Pass, or *ast.ExprStmt wrapping a non-string
//     Constant or a tail string Constant) may appear only AFTER all
//     "real" stmts in the body. Real stmts before tail no-ops follow
//     CPython's optimize_cfg trailing-NOP fold; the reverse ordering
//     would emit a NOP that doesn't fold to byte parity until the
//     v0.7.13 optimize_cfg pass lands.
//   - An *ast.AugAssign is valid only when it is preceded by a real
//     statement. A bare AugAssign at body[0] falls outside the
//     classifier's modAugAssign shape (which requires an init Assign
//     at body[0..0] and the AugAssign at body[1]).
//
// Returns ErrNotImplemented when the shape falls outside this scope
// so runVisitorShadow falls back to the classifier path. Statement
// kinds the visitor doesn't recognize at all (AnnAssign, If, ...)
// are accepted by checkBodyShape and rejected later by visitStmt.
func checkBodyShape(body []ast.Stmt) error {
	sawNoOp := false
	sawReal := false
	for _, stmt := range body {
		switch stmt.(type) {
		case *ast.Pass:
			sawNoOp = true
		case *ast.ExprStmt:
			sawNoOp = true
		case *ast.Assign:
			if sawNoOp {
				return ErrNotImplemented
			}
			sawReal = true
		case *ast.AugAssign:
			if sawNoOp {
				return ErrNotImplemented
			}
			if !sawReal {
				return ErrNotImplemented
			}
			sawReal = true
		}
	}
	return nil
}

// visitModule is the top-level AST walker. It mirrors
// CPython 3.14 Python/codegen.c::compiler_codegen for module bodies.
//
// Layout:
//
//   - RESUME 0 prologue.
//   - If body[0] is a string-literal docstring, emit
//     LOAD_CONST + STORE_NAME __doc__.
//   - For every remaining statement, dispatch through visitStmt.
//     Last statement's Loc anchors the trailing terminator
//     (CPython's optimize_cfg trailing-NOP fold).
//   - finalizeDeferred resolves any folded-but-too-large constants
//     into their final co_consts index.
//   - LOAD_CONST None + RETURN_VALUE.
//
// Returns ErrNotImplemented for any AST node visitStmt does not
// recognize; the caller falls back to the classifier path.
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
		u.phantomDone = true
		lastLoc = doc.Loc
		bodyStart = 1
	}

	tail := mod.Body[bodyStart:]
	if err := checkBodyShape(tail); err != nil {
		return err
	}
	for i, stmt := range tail {
		isLast := i == len(tail)-1
		loc, err := visitStmt(u, stmt, u.Source, isLast)
		if err != nil {
			return err
		}
		if isLast {
			lastLoc = loc
		}
	}

	noneIdx := u.addConst(nil)
	u.finalizeDeferred()
	tailBlock := u.currentBlock()
	tailBlock.Instrs = append(tailBlock.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: lastLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: lastLoc},
	)
	return nil
}
