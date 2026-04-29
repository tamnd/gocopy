package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// visitFuncBodyDef lowers a top-level FunctionDef whose body is a
// sequence of Assign / AugAssign / If / Return / bare-Call
// statements — the surface the v0.6 classifier handled via
// compiler/func_body.go's funcState. v0.7.10 promotes that surface
// into the visitor so func_body.go can be deleted.
//
// Returns ErrNotImplemented if the AST falls outside the supported
// surface; the FunctionDef arm of visit_stmt.go falls through to
// visitClosureDef and then visitFunctionDef when this returns
// ErrNotImplemented.
//
// Structure (mirrors CPython 3.14
// Python/codegen.c::codegen_function_body):
//
//   - Resolve the matching child symtable.Scope by source-order
//     pairing with u.Scope.Children. The scope's Params / Cells /
//     Frees + Symbols drive name resolution via scope_ops.go.
//   - Push a child compileUnit linked to the parent. Wire Scope
//     onto the unit so emitNameLoad / emitNameStore consult it.
//     LocalsPlusNames / LocalsPlusKinds come from Scope.
//   - Emit RESUME 0 at Loc{defLine, defLine, 0, 0}.
//   - Walk body statements via visitFuncStmt.
//   - addConst(nil) so co_consts[0] = None matches CPython's
//     compiler_codegen plant.
//   - Pop the child via popChildUnit. ArgCount = len(args.Args).
//     Flags = CO_OPTIMIZED | CO_NEWLOCALS (no CO_NESTED — closure
//     promotion is visitClosureDef's job).
//   - Emit LOAD_CONST funcCode + MAKE_FUNCTION + STORE_NAME at
//     module level using outerLoc spanning defLine..lastBodyLine.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_function_body +
// Python/codegen.c::codegen_visit_stmt_function_def +
// Python/compile.c::compiler_enter_scope/compiler_exit_scope.
func visitFuncBodyDef(u *compileUnit, s *ast.FunctionDef, source []byte, isLast bool) (bytecode.Loc, error) {
	if !isLast {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(s.DecoratorList) != 0 || s.Returns != nil || len(s.TypeParams) != 0 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	args := s.Args
	if args == nil || len(args.PosOnly) != 0 || len(args.KwOnly) != 0 ||
		args.Vararg != nil || args.Kwarg != nil || len(args.Defaults) != 0 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(args.Args) > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	for _, a := range args.Args {
		if a.Annotation != nil || a.P.Col > 255 || len(a.Name) < 1 || len(a.Name) > 15 {
			return bytecode.Loc{}, ErrNotImplemented
		}
	}
	if len(s.Name) < 1 || len(s.Name) > 15 || s.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(s.Body) < 2 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	lastIdx := len(s.Body) - 1
	ret, ok := s.Body[lastIdx].(*ast.Return)
	if !ok || ret.Value == nil {
		return bytecode.Loc{}, ErrNotImplemented
	}
	retName, ok := ret.Value.(*ast.Name)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if ret.P.Col > 255 || retName.P.Col > 255 ||
		len(retName.Id) < 1 || len(retName.Id) > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	for i := 0; i < lastIdx; i++ {
		asgn, ok := s.Body[i].(*ast.Assign)
		if !ok {
			return bytecode.Loc{}, ErrNotImplemented
		}
		if len(asgn.Targets) != 1 {
			return bytecode.Loc{}, ErrNotImplemented
		}
		tn, ok := asgn.Targets[0].(*ast.Name)
		if !ok || tn.P.Col > 255 || len(tn.Id) < 1 || len(tn.Id) > 15 {
			return bytecode.Loc{}, ErrNotImplemented
		}
		switch v := asgn.Value.(type) {
		case *ast.Name:
			if v.P.Col > 255 || len(v.Id) < 1 || len(v.Id) > 15 {
				return bytecode.Loc{}, ErrNotImplemented
			}
		default:
			return bytecode.Loc{}, ErrNotImplemented
		}
	}

	childScope := findFunctionChildScope(u.Scope, s)
	if childScope == nil {
		return bytecode.Loc{}, ErrNotImplemented
	}

	defLine := s.P.Line
	if defLine < 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	// Stamp Symbol.Index by computing the slot table once.
	localsPlusNames := childScope.LocalsPlusNames()
	localsPlusKinds := childScope.LocalsPlusKinds()
	if len(localsPlusNames) > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	for _, n := range localsPlusNames {
		if len(n) > 15 {
			return bytecode.Loc{}, ErrNotImplemented
		}
	}

	child := u.pushChildUnit(s.Name, s.Name, int32(defLine))
	child.Scope = childScope
	childBlock := child.Seq.AddBlock()

	resumeLoc := bytecode.Loc{
		Line: uint32(defLine), EndLine: uint32(defLine),
		Col: 0, EndCol: 0,
	}
	childBlock.Instrs = append(childBlock.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: resumeLoc},
	)

	for i := 0; i < lastIdx; i++ {
		asgn := s.Body[i].(*ast.Assign)
		if err := emitFuncBodyAssign(child, asgn); err != nil {
			return bytecode.Loc{}, err
		}
	}
	if err := emitFuncBodyReturn(child, ret, retName); err != nil {
		return bytecode.Loc{}, err
	}
	child.addConst(nil)

	bodyEndLine := ret.P.Line
	bodyEndCol := uint16(retName.P.Col) + uint16(len(retName.Id))

	funcCode, err := u.popChildUnit(child, assemble.Options{
		ArgCount:        int32(len(args.Args)),
		Flags:           bytecode.CO_OPTIMIZED | bytecode.CO_NEWLOCALS,
		LocalsPlusNames: localsPlusNames,
		LocalsPlusKinds: localsPlusKinds,
		Filename:        u.Filename,
		Name:            s.Name,
		QualName:        s.Name,
	})
	if err != nil {
		return bytecode.Loc{}, err
	}

	outerLoc := bytecode.Loc{
		Line:    uint32(defLine),
		EndLine: uint32(bodyEndLine),
		Col:     0,
		EndCol:  bodyEndCol,
	}

	funcCodeIdx := u.addConst(funcCode)
	funcNameIdx := u.addName(s.Name)

	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: funcCodeIdx, Loc: outerLoc},
		ir.Instr{Op: bytecode.MAKE_FUNCTION, Arg: 0, Loc: outerLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: funcNameIdx, Loc: outerLoc},
	)

	_ = source
	return outerLoc, nil
}

// emitFuncBodyAssign emits the IR for `target = value` inside a
// function body. v0.7.10 step 4 supports a single Name target with
// a bare-Name RHS. The local-Name RHS uses LOAD_FAST (move
// semantics) — CPython's optimize_load_fast does NOT promote the
// load-into-store path to LOAD_FAST_BORROW because STORE_FAST
// kills the borrowed reference. Globals and other expressions land
// in later steps.
func emitFuncBodyAssign(u *compileUnit, a *ast.Assign) error {
	target := a.Targets[0].(*ast.Name)
	rhsName := a.Value.(*ast.Name)

	rhsLoc := bytecode.Loc{
		Line: uint32(rhsName.P.Line), EndLine: uint32(rhsName.P.Line),
		Col: uint16(rhsName.P.Col), EndCol: uint16(rhsName.P.Col) + uint16(len(rhsName.Id)),
	}
	tgtLoc := bytecode.Loc{
		Line: uint32(target.P.Line), EndLine: uint32(target.P.Line),
		Col: uint16(target.P.Col), EndCol: uint16(target.P.Col) + uint16(len(target.Id)),
	}

	rhsKind, rhsArg := u.resolveNameOp(rhsName.Id)
	tgtKind, tgtArg := u.resolveNameOp(target.Id)

	block := u.currentBlock()
	switch rhsKind {
	case nameOpFast:
		block.Instrs = append(block.Instrs, ir.Instr{Op: bytecode.LOAD_FAST, Arg: rhsArg, Loc: rhsLoc})
	default:
		return ErrNotImplemented
	}
	switch tgtKind {
	case nameOpFast:
		block.Instrs = append(block.Instrs, ir.Instr{Op: bytecode.STORE_FAST, Arg: tgtArg, Loc: tgtLoc})
	default:
		return ErrNotImplemented
	}
	return nil
}

// emitFuncBodyReturn emits the IR for `return name` inside a
// function body. v0.7.10 step 4 supports only a single Name value
// (param / local). The read uses LOAD_FAST_BORROW because the
// borrowed reference is consumed by RETURN_VALUE — this matches
// CPython 3.14 optimize_load_fast's promotion of last-use loads.
func emitFuncBodyReturn(u *compileUnit, ret *ast.Return, retName *ast.Name) error {
	kind, arg := u.resolveNameOp(retName.Id)
	if kind != nameOpFast {
		return ErrNotImplemented
	}
	loadLoc := bytecode.Loc{
		Line: uint32(retName.P.Line), EndLine: uint32(retName.P.Line),
		Col: uint16(retName.P.Col), EndCol: uint16(retName.P.Col) + uint16(len(retName.Id)),
	}
	retLoc := bytecode.Loc{
		Line: uint32(ret.P.Line), EndLine: uint32(ret.P.Line),
		Col: uint16(ret.P.Col), EndCol: uint16(retName.P.Col) + uint16(len(retName.Id)),
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_FAST_BORROW, Arg: arg, Loc: loadLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: retLoc},
	)
	return nil
}

// findFunctionChildScope returns the symtable.Scope corresponding
// to s within parent.Children, matching by source order: the i-th
// FunctionDef in the AST corresponds to the i-th ScopeFunction
// child of parent (CPython's symtable.c builds children in source
// order). Returns nil if parent is nil or no matching child exists.
//
// Source-order matching survives same-named re-definitions, but
// the v0.7.10 surface excludes that case anyway via duplicate-name
// checks elsewhere.
func findFunctionChildScope(parent *symtable.Scope, s *ast.FunctionDef) *symtable.Scope {
	if parent == nil {
		return nil
	}
	for _, c := range parent.Children {
		if c.Kind == symtable.ScopeFunction && c.Name == s.Name &&
			c.Loc.Line == uint32(s.P.Line) {
			return c
		}
	}
	return nil
}
