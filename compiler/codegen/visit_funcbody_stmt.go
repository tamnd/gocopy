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
	_ = isLast
	if s.Returns != nil || len(s.TypeParams) != 0 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	for _, d := range s.DecoratorList {
		n, ok := d.(*ast.Name)
		if !ok || n.P.Col > 255 || len(n.Id) < 1 || len(n.Id) > 15 || n.P.Line < 1 {
			return bytecode.Loc{}, ErrNotImplemented
		}
	}
	args := s.Args
	if args == nil {
		return bytecode.Loc{}, ErrNotImplemented
	}
	validateDefaultValue := func(e ast.Expr) bool {
		switch v := e.(type) {
		case *ast.Constant:
			if v.P.Col > 255 {
				return false
			}
			_, ok := constantValue(v)
			return ok
		case *ast.Name:
			return v.P.Col <= 255 && len(v.Id) >= 1 && len(v.Id) <= 15
		}
		return false
	}
	for _, d := range args.Defaults {
		if !validateDefaultValue(d) {
			return bytecode.Loc{}, ErrNotImplemented
		}
	}
	if len(args.KwOnlyDef) != 0 && len(args.KwOnlyDef) != len(args.KwOnly) {
		return bytecode.Loc{}, ErrNotImplemented
	}
	for _, d := range args.KwOnlyDef {
		if d == nil {
			continue
		}
		if !validateDefaultValue(d) {
			return bytecode.Loc{}, ErrNotImplemented
		}
	}
	totalSlots := len(args.PosOnly) + len(args.Args) + len(args.KwOnly)
	if args.Vararg != nil {
		totalSlots++
	}
	if args.Kwarg != nil {
		totalSlots++
	}
	if totalSlots > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	validateArg := func(a *ast.Arg) bool {
		return a != nil && a.Annotation == nil && a.P.Col <= 255 &&
			len(a.Name) >= 1 && len(a.Name) <= 15
	}
	for _, a := range args.PosOnly {
		if !validateArg(a) {
			return bytecode.Loc{}, ErrNotImplemented
		}
	}
	for _, a := range args.Args {
		if !validateArg(a) {
			return bytecode.Loc{}, ErrNotImplemented
		}
	}
	for _, a := range args.KwOnly {
		if !validateArg(a) {
			return bytecode.Loc{}, ErrNotImplemented
		}
	}
	if args.Vararg != nil && !validateArg(args.Vararg) {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if args.Kwarg != nil && !validateArg(args.Kwarg) {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(s.Name) < 1 || len(s.Name) > 15 || s.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(s.Body) < 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	lastIdx := len(s.Body) - 1
	var ret *ast.Return
	var termIf *ast.If
	var raise *ast.Raise
	var assert *ast.Assert
	var termWhile *ast.While
	var termFor *ast.For
	var termPass *ast.Pass
	switch last := s.Body[lastIdx].(type) {
	case *ast.Return:
		if last.Value == nil || last.P.Col > 255 || !validateFuncBodyReturnValue(last.Value) {
			return bytecode.Loc{}, ErrNotImplemented
		}
		ret = last
	case *ast.If:
		if !validateIfStmt(last, true) {
			return bytecode.Loc{}, ErrNotImplemented
		}
		termIf = last
	case *ast.Raise:
		if !validateFuncBodyRaise(last) {
			return bytecode.Loc{}, ErrNotImplemented
		}
		raise = last
	case *ast.Assert:
		if !validateFuncBodyAssert(last) {
			return bytecode.Loc{}, ErrNotImplemented
		}
		assert = last
	case *ast.While:
		if !validateFuncBodyWhile(last) {
			return bytecode.Loc{}, ErrNotImplemented
		}
		termWhile = last
	case *ast.For:
		if !validateFuncBodyFor(last) {
			return bytecode.Loc{}, ErrNotImplemented
		}
		termFor = last
	case *ast.Pass:
		// CPython 3.14 codegen_function_body has no terminator
		// requirement; a Pass-terminated body falls through to the
		// implicit return planted by _PyCodegen_AddReturnAtEnd.
		// Pass survives the validation gate trivially (no fields to
		// check) — if its column or line are out of range, the
		// per-stmt emit still works; the visitor's outer loc inherits
		// the Pass span.
		if last.P.Col > 255 || last.P.Line < 1 {
			return bytecode.Loc{}, ErrNotImplemented
		}
		termPass = last
	default:
		return bytecode.Loc{}, ErrNotImplemented
	}
	for i := range lastIdx {
		switch st := s.Body[i].(type) {
		case *ast.Assign:
			if len(st.Targets) != 1 {
				return bytecode.Loc{}, ErrNotImplemented
			}
			tn, ok := st.Targets[0].(*ast.Name)
			if !ok || tn.P.Col > 255 || len(tn.Id) < 1 || len(tn.Id) > 15 {
				return bytecode.Loc{}, ErrNotImplemented
			}
			if !validateFuncBodyAssignRHS(st.Value) {
				return bytecode.Loc{}, ErrNotImplemented
			}
		case *ast.AugAssign:
			tn, ok := st.Target.(*ast.Name)
			if !ok || tn.P.Col > 255 || len(tn.Id) < 1 || len(tn.Id) > 15 {
				return bytecode.Loc{}, ErrNotImplemented
			}
			if _, ok := augOpargFromOp(st.Op); !ok {
				return bytecode.Loc{}, ErrNotImplemented
			}
			if !validateFuncBodyAssignRHS(st.Value) {
				return bytecode.Loc{}, ErrNotImplemented
			}
		case *ast.AnnAssign:
			if !validateFuncBodyAnnAssign(st) {
				return bytecode.Loc{}, ErrNotImplemented
			}
		case *ast.Pass:
			// No validation. CPython 3.14
			// Python/codegen.c::codegen_visit_stmt Pass_kind arm
			// is one line: ADDOP(c, LOC(s), NOP).
		case *ast.Global:
			if !validateFuncBodyGlobalNonlocal(st.Names, st.P) {
				return bytecode.Loc{}, ErrNotImplemented
			}
		case *ast.Nonlocal:
			if !validateFuncBodyGlobalNonlocal(st.Names, st.P) {
				return bytecode.Loc{}, ErrNotImplemented
			}
		case *ast.If:
			if !validateIfStmt(st, false) {
				return bytecode.Loc{}, ErrNotImplemented
			}
		case *ast.FunctionDef:
			if !validateFuncBodyInnerDef(st) {
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
	firstLineno := defLine
	if len(s.DecoratorList) > 0 {
		firstLineno = s.DecoratorList[0].(*ast.Name).P.Line
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

	child := u.pushChildUnit(s.Name, s.Name, int32(firstLineno))
	child.Scope = childScope
	childBlock := child.Seq.AddBlock()

	// MAKE_CELL prologue. CPython 3.14 Python/compile.c::compiler_make_cell
	// emits MAKE_CELL with the LocalsPlus slot index for every cell var
	// (in the order they appear in u_cellvars). Synthetic ops carry
	// NO_LOCATION which the line-table encoder serialises as the
	// implicit-prologue NO_INFO entry.
	for _, name := range childScope.Cells {
		if sym, ok := childScope.Symbols[name]; ok {
			childBlock.Instrs = append(childBlock.Instrs, ir.Instr{
				Op: bytecode.MAKE_CELL, Arg: uint32(sym.Index), Loc: bytecode.Loc{},
			})
		}
	}
	// COPY_FREE_VARS prologue. CPython 3.14
	// Python/compile.c::compiler_make_closure emits COPY_FREE_VARS N at
	// the top of every nested function whose code captures N free
	// variables from an enclosing scope.
	if n := len(childScope.Frees); n > 0 {
		childBlock.Instrs = append(childBlock.Instrs, ir.Instr{
			Op: bytecode.COPY_FREE_VARS, Arg: uint32(n), Loc: bytecode.Loc{},
		})
	}

	resumeLoc := bytecode.Loc{
		Line: uint32(firstLineno), EndLine: uint32(firstLineno),
		Col: 0, EndCol: 0,
	}
	childBlock.Instrs = append(childBlock.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: resumeLoc},
	)

	lines := splitLines(source)
	for i := range lastIdx {
		switch st := s.Body[i].(type) {
		case *ast.Assign:
			if err := emitFuncBodyAssign(child, st, lines); err != nil {
				return bytecode.Loc{}, err
			}
		case *ast.AugAssign:
			if err := emitFuncBodyAugAssign(child, st, lines); err != nil {
				return bytecode.Loc{}, err
			}
		case *ast.AnnAssign:
			if err := emitFuncBodyAnnAssign(child, st, lines); err != nil {
				return bytecode.Loc{}, err
			}
		case *ast.Pass:
			// CPython 3.14 Python/codegen.c::codegen_visit_stmt
			// Pass_kind: ADDOP(c, LOC(s), NOP). LOC(s) carries the
			// stmt's exact span (col_offset..end_col_offset). The
			// "pass" keyword is 4 chars wide.
			loc := bytecode.Loc{
				Line:    uint32(st.P.Line),
				EndLine: uint32(st.P.Line),
				Col:     uint16(st.P.Col),
				EndCol:  uint16(st.P.Col) + 4,
			}
			block := child.currentBlock()
			block.Instrs = append(block.Instrs, ir.Instr{
				Op: bytecode.NOP, Arg: 0, Loc: loc,
			})
		case *ast.Global, *ast.Nonlocal:
			// Pure symtable directives — no bytecode output.
			// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_stmt
			// Global_kind / Nonlocal_kind arms (both `break;`).
		case *ast.If:
			if _, _, _, err := codegenIf(child, st, lines); err != nil {
				return bytecode.Loc{}, err
			}
		case *ast.FunctionDef:
			if err := emitFuncBodyInnerDef(child, st, lines); err != nil {
				return bytecode.Loc{}, err
			}
		}
	}
	var bodyEndCol uint16
	var bodyEndLine int
	switch {
	case ret != nil:
		c, err := emitFuncBodyReturn(child, ret, lines)
		if err != nil {
			return bytecode.Loc{}, err
		}
		bodyEndCol = c
		bodyEndLine = ret.P.Line
	case raise != nil:
		c, err := emitFuncBodyRaise(child, raise, lines)
		if err != nil {
			return bytecode.Loc{}, err
		}
		bodyEndCol = c
		bodyEndLine = raise.P.Line
	case assert != nil:
		c, err := emitFuncBodyAssert(child, assert, lines)
		if err != nil {
			return bytecode.Loc{}, err
		}
		bodyEndCol = c
		bodyEndLine = assert.P.Line
	case termWhile != nil:
		c, line, err := emitFuncBodyWhile(child, termWhile, lines)
		if err != nil {
			return bytecode.Loc{}, err
		}
		bodyEndCol = c
		bodyEndLine = line
	case termFor != nil:
		c, line, err := emitFuncBodyFor(child, termFor, lines)
		if err != nil {
			return bytecode.Loc{}, err
		}
		bodyEndCol = c
		bodyEndLine = line
	case termPass != nil:
		// CPython 3.14 Python/codegen.c::codegen_visit_stmt Pass_kind:
		// ADDOP(c, LOC(s), NOP). The Pass span is fixed at the
		// keyword width (4 chars).
		loc := bytecode.Loc{
			Line:    uint32(termPass.P.Line),
			EndLine: uint32(termPass.P.Line),
			Col:     uint16(termPass.P.Col),
			EndCol:  uint16(termPass.P.Col) + 4,
		}
		child.currentBlock().Instrs = append(child.currentBlock().Instrs,
			ir.Instr{Op: bytecode.NOP, Arg: 0, Loc: loc},
		)
		bodyEndCol = uint16(termPass.P.Col) + 4
		bodyEndLine = termPass.P.Line
	default:
		c, line, _, err := codegenIf(child, termIf, lines)
		if err != nil {
			return bytecode.Loc{}, err
		}
		bodyEndCol = c
		bodyEndLine = line
	}

	// CPython 3.14 Python/codegen.c:1378 codegen_function_body
	// always calls _PyCompile_OptimizeAndAssemble(c, 1), which in turn
	// invokes Python/codegen.c:6473 _PyCodegen_AddReturnAtEnd to plant
	// LOAD_CONST None + RETURN_VALUE at NO_LOCATION on a fresh trailing
	// block. When the body already returned (via Return / Raise /
	// Assert / While-exit / For-exit), this trailing block is
	// unreachable and Python/flowgraph.c:996 remove_unreachable zeroes
	// its instructions, leaving byte parity intact. When the body fell
	// through (e.g. a Pass-terminated body), the trailing block becomes
	// the implicit return.
	addReturnAtEnd(child, true)
	if len(child.Consts) == 0 {
		child.addConst(nil)
	}

	flags := bytecode.CO_OPTIMIZED | bytecode.CO_NEWLOCALS
	if args.Vararg != nil {
		flags |= bytecode.CO_VARARGS
	}
	if args.Kwarg != nil {
		flags |= bytecode.CO_VARKEYWORDS
	}

	funcCode, err := u.popChildUnit(child, assemble.Options{
		ArgCount:        int32(len(args.PosOnly) + len(args.Args)),
		PosOnlyArgCount: int32(len(args.PosOnly)),
		KwOnlyArgCount:  int32(len(args.KwOnly)),
		Flags:           flags,
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

	defaultValueLoc := func(e ast.Expr) bytecode.Loc {
		switch v := e.(type) {
		case *ast.Constant:
			endCol := astExprEndCol(lines, v.P.Line, v)
			return bytecode.Loc{
				Line:    uint32(v.P.Line),
				EndLine: uint32(v.P.Line),
				Col:     uint16(v.P.Col),
				EndCol:  uint16(endCol),
			}
		case *ast.Name:
			return bytecode.Loc{
				Line:    uint32(v.P.Line),
				EndLine: uint32(v.P.Line),
				Col:     uint16(v.P.Col),
				EndCol:  uint16(v.P.Col) + uint16(len(v.Id)),
			}
		}
		return outerLoc
	}
	emitDefaultLoad := func(e ast.Expr) {
		loc := defaultValueLoc(e)
		switch v := e.(type) {
		case *ast.Constant:
			val, _ := constantValue(v)
			emitConstLoad(u, val, loc)
		case *ast.Name:
			u.emitNameLoad(v.Id, loc)
		}
	}

	decoLoc := func(n *ast.Name) bytecode.Loc {
		return bytecode.Loc{
			Line:    uint32(n.P.Line),
			EndLine: uint32(n.P.Line),
			Col:     uint16(n.P.Col),
			EndCol:  uint16(n.P.Col) + uint16(len(n.Id)),
		}
	}
	for _, d := range s.DecoratorList {
		n := d.(*ast.Name)
		u.emitNameLoad(n.Id, decoLoc(n))
	}

	hasDefaults := len(args.Defaults) > 0
	if hasDefaults {
		for _, d := range args.Defaults {
			emitDefaultLoad(d)
		}
		u.currentBlock().Instrs = append(u.currentBlock().Instrs,
			ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: uint32(len(args.Defaults)), Loc: outerLoc},
		)
	}
	kwdefCount := 0
	for _, d := range args.KwOnlyDef {
		if d != nil {
			kwdefCount++
		}
	}
	if kwdefCount > 0 {
		for i, d := range args.KwOnlyDef {
			if d == nil {
				continue
			}
			emitConstLoad(u, args.KwOnly[i].Name, outerLoc)
			emitDefaultLoad(d)
		}
		u.currentBlock().Instrs = append(u.currentBlock().Instrs,
			ir.Instr{Op: bytecode.BUILD_MAP, Arg: uint32(kwdefCount), Loc: outerLoc},
		)
	}

	funcCodeIdx := u.addConst(funcCode)
	funcNameIdx := u.addName(s.Name)

	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: funcCodeIdx, Loc: outerLoc},
		ir.Instr{Op: bytecode.MAKE_FUNCTION, Arg: 0, Loc: outerLoc},
	)
	if kwdefCount > 0 {
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.SET_FUNCTION_ATTRIBUTE, Arg: 2, Loc: outerLoc},
		)
	}
	if hasDefaults {
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.SET_FUNCTION_ATTRIBUTE, Arg: 1, Loc: outerLoc},
		)
	}
	for i := len(s.DecoratorList) - 1; i >= 0; i-- {
		n := s.DecoratorList[i].(*ast.Name)
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.CALL, Arg: 0, Loc: decoLoc(n)},
		)
	}
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.STORE_NAME, Arg: funcNameIdx, Loc: outerLoc},
	)

	_ = source
	return outerLoc, nil
}

// emitFuncBodyAssign emits the IR for `target = value` inside a
// function body. v0.7.10 step 4 supports a single Name target with
// a bare-Name or simple-Constant RHS. The local-Name RHS uses
// LOAD_FAST (move semantics) — CPython's optimize_load_fast does
// NOT promote the load-into-store path to LOAD_FAST_BORROW because
// STORE_FAST kills the borrowed reference. Richer RHS expressions
// land in later v0.7.10 steps; v0.7.10.11 routes the target store
// through emitNameStore so a `global x; x = 1` declaration emits
// STORE_GLOBAL instead of STORE_FAST.
func emitFuncBodyAssign(u *compileUnit, a *ast.Assign, lines [][]byte) error {
	target := a.Targets[0].(*ast.Name)
	tgtLoc := bytecode.Loc{
		Line: uint32(target.P.Line), EndLine: uint32(target.P.Line),
		Col: uint16(target.P.Col), EndCol: uint16(target.P.Col) + uint16(len(target.Id)),
	}

	switch v := a.Value.(type) {
	case *ast.Name:
		rhsLoc := bytecode.Loc{
			Line: uint32(v.P.Line), EndLine: uint32(v.P.Line),
			Col: uint16(v.P.Col), EndCol: uint16(v.P.Col) + uint16(len(v.Id)),
		}
		rhsKind, rhsArg := u.resolveNameOp(v.Id)
		if rhsKind != nameOpFast {
			return ErrNotImplemented
		}
		u.currentBlock().Instrs = append(u.currentBlock().Instrs,
			ir.Instr{Op: bytecode.LOAD_FAST, Arg: rhsArg, Loc: rhsLoc})
	default:
		if _, _, err := visitFuncExpr(u, a.Value, lines); err != nil {
			return err
		}
	}
	u.emitNameStore(target.Id, tgtLoc)
	return nil
}

// emitFuncBodyAugAssign emits the IR for `target op= value` inside
// a function body. Mirrors compiler/func_body.go's AugAssign arm:
//
//   - LOAD_FAST target — Loc spans target's identifier. The
//     LOAD_FAST → LOAD_FAST_BORROW promotion (and the LFLF →
//     LFLBLFLB promotion that follows when the pair fuses) is owned
//     by the optimize_load_fast pass downstream.
//   - Recurse RHS via visitFuncExpr.
//   - BINARY_OP NbInplace* — Loc (target.startCol, rhs.endCol).
//   - STORE_FAST target — Loc spans target's identifier.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_stmt_aug_assign.
func emitFuncBodyAugAssign(u *compileUnit, a *ast.AugAssign, lines [][]byte) error {
	target := a.Target.(*ast.Name)
	oparg, ok := augOpargFromOp(a.Op)
	if !ok {
		return ErrNotImplemented
	}
	tgtLoc := bytecode.Loc{
		Line: uint32(target.P.Line), EndLine: uint32(target.P.Line),
		Col: uint16(target.P.Col), EndCol: uint16(target.P.Col) + uint16(len(target.Id)),
	}
	tgtKind, tgtArg := u.resolveNameOp(target.Id)
	if tgtKind != nameOpFast {
		return ErrNotImplemented
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: bytecode.LOAD_FAST, Arg: tgtArg, Loc: tgtLoc,
	})
	_, rhsEnd, err := visitFuncExpr(u, a.Value, lines)
	if err != nil {
		return err
	}
	binOpLoc := bytecode.Loc{
		Line: uint32(a.P.Line), EndLine: uint32(a.P.Line),
		Col: uint16(target.P.Col), EndCol: rhsEnd,
	}
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.BINARY_OP, Arg: uint32(oparg), Loc: binOpLoc},
		ir.Instr{Op: bytecode.STORE_FAST, Arg: tgtArg, Loc: tgtLoc},
	)
	return nil
}

// validateFuncBodyAssign reports whether a is a single-Name-target
// assignment with an RHS the function-body visitor accepts.
func validateFuncBodyAssign(a *ast.Assign) bool {
	if len(a.Targets) != 1 {
		return false
	}
	tn, ok := a.Targets[0].(*ast.Name)
	if !ok || tn.P.Col > 255 || len(tn.Id) < 1 || len(tn.Id) > 15 {
		return false
	}
	return validateFuncBodyAssignRHS(a.Value)
}

// validateFuncBodyReturnValue reports whether e is a value the
// function-body visitor accepts as a Return value. Returns are the
// only position where a ternary IfExp is allowed: outside of return
// position, the visitor falls through (the legacy classifier still
// handles those rare shapes).
//
// Ternary IfExp must have a single-op single-comparator Compare
// Test (non-IS_OP), and both Body and OrElse must validate as
// ordinary RHS expressions.
func validateFuncBodyReturnValue(e ast.Expr) bool {
	if ifexpr, ok := e.(*ast.IfExp); ok {
		if ifexpr.P.Col > 255 {
			return false
		}
		cmp, ok := ifexpr.Test.(*ast.Compare)
		if !ok || cmp.P.Col > 255 || len(cmp.Ops) != 1 || len(cmp.Comparators) != 1 {
			return false
		}
		op, _, ok := cmpOpFromAstOp(cmp.Ops[0])
		if !ok || op == bytecode.IS_OP {
			return false
		}
		if !validateFuncBodyAssignRHS(cmp.Left) || !validateFuncBodyAssignRHS(cmp.Comparators[0]) {
			return false
		}
		return validateFuncBodyAssignRHS(ifexpr.Body) && validateFuncBodyAssignRHS(ifexpr.OrElse)
	}
	return validateFuncBodyAssignRHS(e)
}

// emitFuncBodyReturn emits the IR for `return <expr>` inside a
// function body. The expression is recursed via visitFuncExpr so any
// shape validateFuncBodyAssignRHS accepts is supported. The trailing
// RETURN_VALUE consumes the value, which is why Name reads come out
// as LOAD_FAST_BORROW (CPython 3.14 optimize_load_fast promotes
// last-use loads — RETURN_VALUE is one such consumer).
//
// retLoc spans (return-keyword col, value-end col) for non-constant
// return values. When the return value is a *ast.Constant, CPython
// 3.14 attributes RETURN_VALUE to the constant's span instead of
// the return-keyword span — matching that here keeps the linetable
// byte-identical.
//
// *ast.IfExp is dispatched to emitFuncBodyTernaryReturn, which
// expands `return Body if Test else OrElse` into a branched IR with
// two RETURN_VALUE instructions.
func emitFuncBodyReturn(u *compileUnit, ret *ast.Return, lines [][]byte) (uint16, error) {
	if ifexpr, ok := ret.Value.(*ast.IfExp); ok {
		return emitFuncBodyTernaryReturn(u, ret, ifexpr, lines)
	}
	_, valEnd, err := visitFuncExpr(u, ret.Value, lines)
	if err != nil {
		return 0, err
	}
	startCol := uint16(ret.P.Col)
	if c, ok := ret.Value.(*ast.Constant); ok {
		startCol = uint16(c.P.Col)
	}
	retLoc := bytecode.Loc{
		Line: uint32(ret.P.Line), EndLine: uint32(ret.P.Line),
		Col: startCol, EndCol: valEnd,
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: retLoc},
	)
	return valEnd, nil
}

// validateFuncBodyRaise reports whether r is a `raise` statement
// shape the v0.7.10 function-body visitor accepts. Three shapes:
// bare (`raise`), with-value (`raise X`), with-cause
// (`raise X from Y`). Both X and Y must be `*ast.Name` (col ≤ 255,
// 1..15 identifier chars). `raise from Y` (Cause without Exc) is
// illegal Python and rejected.
func validateFuncBodyRaise(r *ast.Raise) bool {
	if r.P.Col > 255 || r.P.Line < 1 {
		return false
	}
	if r.Exc == nil {
		return r.Cause == nil
	}
	n, ok := r.Exc.(*ast.Name)
	if !ok || n.P.Col > 255 || len(n.Id) < 1 || len(n.Id) > 15 {
		return false
	}
	if r.Cause != nil {
		cn, ok := r.Cause.(*ast.Name)
		if !ok || cn.P.Col > 255 || len(cn.Id) < 1 || len(cn.Id) > 15 {
			return false
		}
	}
	return true
}

// emitFuncBodyRaise emits the IR for a `raise` statement inside a
// function body. Mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt Raise_kind arm:
//
//   - `raise`            → RAISE_VARARGS 0
//   - `raise X`          → visit X, RAISE_VARARGS 1
//   - `raise X from Y`   → visit X, visit Y, RAISE_VARARGS 2
//
// The RAISE_VARARGS instruction's Loc spans the full statement
// (`raise`-keyword col through the last operand's end col, or +5
// for a bare raise) — matching CPython's `LOC(s)`. Operand loads
// use each Name's own span via visitFuncExpr (which dispatches to
// LOAD_FAST / LOAD_DEREF / LOAD_GLOBAL / LOAD_NAME based on the
// symbol-table kind).
//
// Returns the bodyEndCol — the end col of the raise statement, used
// by the caller to stamp the outer function-def Loc's EndCol.
//
// SOURCE: CPython 3.14 Python/codegen.c:3035-3048
// (codegen_visit_stmt Raise_kind).
func emitFuncBodyRaise(u *compileUnit, r *ast.Raise, lines [][]byte) (uint16, error) {
	startCol := uint16(r.P.Col)
	endCol := startCol + uint16(len("raise"))
	var n uint32
	if r.Exc != nil {
		_, exEnd, err := visitFuncExpr(u, r.Exc, lines)
		if err != nil {
			return 0, err
		}
		n = 1
		endCol = exEnd
		if r.Cause != nil {
			_, caEnd, err := visitFuncExpr(u, r.Cause, lines)
			if err != nil {
				return 0, err
			}
			n = 2
			endCol = caEnd
		}
	}
	raiseLoc := bytecode.Loc{
		Line: uint32(r.P.Line), EndLine: uint32(r.P.Line),
		Col: startCol, EndCol: endCol,
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RAISE_VARARGS, Arg: n, Loc: raiseLoc},
	)
	return endCol, nil
}

// emitFuncBodyTernaryReturn lowers `return Body if Test else OrElse`
// inside a function body. Mirrors compiler/func_body.go's IfExp arm
// of the Return handler:
//
//   - Pre-compute ternEnd from astExprEndCol(OrElse) before walking
//     OrElse so the trailing RETURN_VALUE's Loc spans (retCol,
//     ternEnd) — the whole-ternary span CPython attributes
//     RETURN_VALUE to.
//   - Walk Compare.Left + Comparators[0] via visitFuncExpr.
//   - Emit COMPARE_OP / CONTAINS_OP with the conditional-context
//     bit (oparg+16) at condLoc(left.start, right.end).
//   - POP_JUMP_IF_FALSE → falseLabel.
//   - True-branch block: NOT_TAKEN at condLoc, walk Body via
//     visitFuncExpr, JUMP_FORWARD → endLabel.
//   - False-branch block bound to falseLabel: walk OrElse via
//     visitFuncExpr (falls through to the merge block).
//   - End-merge block bound to endLabel: RETURN_VALUE at
//     retLoc(retCol, ternEnd).
//
// The end-merge block is the structural equivalent of CPython's
// `end:` label after codegen_ifexp returns. inlineSmallOrNoLinenoBlocks
// duplicates RETURN_VALUE into the JUMP_FORWARD predecessor (the
// true branch) but keeps the merge block alive for the
// fall-through (false) branch — that asymmetry is exactly what
// makes optimize_load_fast leave the false branch's LOAD_FAST as
// LOAD_FAST while promoting the true branch's to LOAD_FAST_BORROW.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_expr_ifexp +
// codegen_compare conditional-context specialisation.
func emitFuncBodyTernaryReturn(u *compileUnit, ret *ast.Return, ifexpr *ast.IfExp, lines [][]byte) (uint16, error) {
	ternEnd := uint16(astExprEndCol(lines, ret.P.Line, ifexpr.OrElse))

	cmp := ifexpr.Test.(*ast.Compare)
	op, base, _ := cmpOpFromAstOp(cmp.Ops[0])

	lc, _, err := visitFuncExpr(u, cmp.Left, lines)
	if err != nil {
		return 0, err
	}
	_, re, err := visitFuncExpr(u, cmp.Comparators[0], lines)
	if err != nil {
		return 0, err
	}
	condLoc := bytecode.Loc{
		Line: uint32(cmp.P.Line), EndLine: uint32(cmp.P.Line),
		Col: lc, EndCol: re,
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: op, Arg: uint32(base + 16), Loc: condLoc,
	})
	falseLabel := u.Seq.AllocLabel()
	endLabel := u.Seq.AllocLabel()
	block.AddJump(bytecode.POP_JUMP_IF_FALSE, falseLabel, condLoc)

	thenBlock := u.Seq.AddBlock()
	thenBlock.Instrs = append(thenBlock.Instrs, ir.Instr{
		Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc,
	})

	if _, _, err := visitFuncExpr(u, ifexpr.Body, lines); err != nil {
		return 0, err
	}
	retLoc := bytecode.Loc{
		Line: uint32(ret.P.Line), EndLine: uint32(ret.P.Line),
		Col: uint16(ret.P.Col), EndCol: ternEnd,
	}
	u.currentBlock().AddJump(bytecode.JUMP_FORWARD, endLabel, bytecode.Loc{})

	falseBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(falseLabel, falseBlock)

	if _, _, err := visitFuncExpr(u, ifexpr.OrElse, lines); err != nil {
		return 0, err
	}

	endBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(endLabel, endBlock)
	endBlock.Instrs = append(endBlock.Instrs, ir.Instr{
		Op: bytecode.RETURN_VALUE, Arg: 0, Loc: retLoc,
	})
	return ternEnd, nil
}

// validateFuncBodyAssignRHS reports whether e is an expression
// shape the v0.7.10 function-body visitor accepts on the RHS of an
// Assign. Accepted shapes:
//
//   - *ast.Name with 1..15-byte id and col ≤ 255.
//   - *ast.Constant whose value resolves via constantValue and col
//     ≤ 255.
//   - *ast.BinOp with a known oparg whose Left and Right operands
//     each recursively validate.
//
// Wider shapes return false and the caller falls through to the
// next FunctionDef arm in visit_stmt.go.
func validateFuncBodyAssignRHS(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Name:
		return v.P.Col <= 255 && len(v.Id) >= 1 && len(v.Id) <= 15
	case *ast.Constant:
		if v.P.Col > 255 {
			return false
		}
		_, ok := constantValue(v)
		return ok
	case *ast.BinOp:
		if v.P.Col > 255 {
			return false
		}
		if _, ok := binOpargFromOp(v.Op); !ok {
			return false
		}
		return validateFuncBodyAssignRHS(v.Left) && validateFuncBodyAssignRHS(v.Right)
	case *ast.UnaryOp:
		if v.P.Col > 255 {
			return false
		}
		switch v.Op {
		case "USub", "Invert", "Not":
			return validateFuncBodyAssignRHS(v.Operand)
		}
		return false
	case *ast.Compare:
		if v.P.Col > 255 {
			return false
		}
		if len(v.Ops) != 1 || len(v.Comparators) != 1 {
			return false
		}
		if _, _, ok := cmpOpFromAstOp(v.Ops[0]); !ok {
			return false
		}
		return validateFuncBodyAssignRHS(v.Left) && validateFuncBodyAssignRHS(v.Comparators[0])
	case *ast.Call:
		if v.P.Col > 255 {
			return false
		}
		if len(v.Keywords) != 0 {
			return false
		}
		switch fn := v.Func.(type) {
		case *ast.Name:
			if len(fn.Id) < 1 || len(fn.Id) > 15 || fn.P.Col > 255 {
				return false
			}
		case *ast.Attribute:
			if len(fn.Attr) < 1 || len(fn.Attr) > 15 || fn.P.Col > 255 {
				return false
			}
			if !validateFuncBodyAssignRHS(fn.Value) {
				return false
			}
		default:
			return false
		}
		for _, a := range v.Args {
			if !validateFuncBodyAssignRHS(a) {
				return false
			}
		}
		return true
	case *ast.Attribute:
		if v.P.Col > 255 {
			return false
		}
		if len(v.Attr) < 1 || len(v.Attr) > 15 {
			return false
		}
		return validateFuncBodyAssignRHS(v.Value)
	case *ast.Subscript:
		if v.P.Col > 255 {
			return false
		}
		return validateFuncBodyAssignRHS(v.Value) && validateFuncBodyAssignRHS(v.Slice)
	case *ast.Tuple:
		if v.P.Col > 255 || len(v.Elts) == 0 {
			return false
		}
		for _, e := range v.Elts {
			if !validateFuncBodyAssignRHS(e) {
				return false
			}
		}
		return true
	case *ast.BoolOp:
		if v.P.Col > 255 || len(v.Values) < 2 {
			return false
		}
		if v.Op != "And" && v.Op != "Or" {
			return false
		}
		for _, x := range v.Values {
			if _, nested := x.(*ast.BoolOp); nested {
				return false
			}
			if !validateFuncBodyAssignRHS(x) {
				return false
			}
		}
		return true
	}
	return false
}

// validateFuncBodyAnnAssign reports whether n is an AnnAssign shape
// the v0.7.10.11 function-body visitor accepts. CPython 3.14
// codegen_annassign drops the annotation entirely at function scope
// (PEP 526: types in function scopes are not evaluated), so the
// emit reduces to either a plain Assign (when Value is set) or a
// no-op (when Value is nil). Either way the target must be a bare
// Name (1..15-byte id, col ≤ 255), and Value — when present — must
// validate as an ordinary RHS.
//
// SOURCE: CPython 3.14 Python/compile.c::codegen_annassign,
// `simple && (scope == MODULE || scope == CLASS)` guard.
func validateFuncBodyAnnAssign(n *ast.AnnAssign) bool {
	if n.P.Col > 255 {
		return false
	}
	tn, ok := n.Target.(*ast.Name)
	if !ok || tn.P.Col > 255 || len(tn.Id) < 1 || len(tn.Id) > 15 {
		return false
	}
	if n.Value != nil && !validateFuncBodyAssignRHS(n.Value) {
		return false
	}
	return true
}

// validateFuncBodyGlobalNonlocal reports whether a `global` or
// `nonlocal` directive's identifier list is acceptable at the
// v0.7.10.11 funcbody surface. The directives emit no bytecode of
// their own; their effect is in the symtable (SymGlobal / SymFree).
// We only gate on identifier shape (1..15 bytes each) and a
// non-empty list (the parser already enforces non-empty, but it
// costs nothing to recheck here).
func validateFuncBodyGlobalNonlocal(names []string, p ast.Pos) bool {
	if p.Col > 255 || len(names) == 0 {
		return false
	}
	for _, n := range names {
		if len(n) < 1 || len(n) > 15 {
			return false
		}
	}
	return true
}

// validateFuncBodyAssert reports whether a is an `assert` statement
// shape the v0.7.10.11 function-body visitor accepts. The Test must
// be a bare *ast.Name (col ≤ 255, 1..15-byte id) — the surface
// codegen_jump_if's Name arm exercises with a TO_BOOL +
// POP_JUMP_IF_TRUE. The optional Msg must validate as an ordinary
// RHS expression.
func validateFuncBodyAssert(a *ast.Assert) bool {
	if a.P.Col > 255 || a.P.Line < 1 {
		return false
	}
	tn, ok := a.Test.(*ast.Name)
	if !ok || tn.P.Col > 255 || len(tn.Id) < 1 || len(tn.Id) > 15 {
		return false
	}
	if a.Msg != nil && !validateFuncBodyAssignRHS(a.Msg) {
		return false
	}
	return true
}

// emitFuncBodyAnnAssign emits the IR for an `target: annotation = value`
// statement inside a function body. CPython 3.14 codegen_annassign
// elides the annotation at function scope (PEP 526), so the emit is
// either:
//
//   - With value: visit value, emitNameStore target — exactly the
//     same shape as a plain Assign.
//   - Without value: no-op. Symtable still binds the target as
//     SymAnnotated (without SymLocal), so the slot is not allocated
//     in co_varnames; the statement is invisible in bytecode.
//
// SOURCE: CPython 3.14 Python/compile.c::codegen_annassign — the
// Name_kind arm with `s->v.AnnAssign.value` set.
func emitFuncBodyAnnAssign(u *compileUnit, n *ast.AnnAssign, lines [][]byte) error {
	if n.Value == nil {
		return nil
	}
	target := n.Target.(*ast.Name)
	if _, _, err := visitFuncExpr(u, n.Value, lines); err != nil {
		return err
	}
	tgtLoc := bytecode.Loc{
		Line: uint32(target.P.Line), EndLine: uint32(target.P.Line),
		Col: uint16(target.P.Col), EndCol: uint16(target.P.Col) + uint16(len(target.Id)),
	}
	u.emitNameStore(target.Id, tgtLoc)
	return nil
}

// emitFuncBodyAssert emits the IR for a terminator-position
// `assert test[, msg]` statement inside a function body. Mirrors
// CPython 3.14 Python/codegen.c::codegen_assert (line 2932):
//
//	NEW_JUMP_TARGET_LABEL(c, end);
//	codegen_jump_if(c, LOC(s), test, end, /*cond=*/1);
//	ADDOP(c, LOC(s), LOAD_COMMON_CONSTANT, CONSTANT_ASSERTIONERROR);
//	if (msg) { VISIT(c, expr, msg); ADDOP_I(c, LOC(s), CALL, 0); }
//	ADDOP_I(c, LOC(test), RAISE_VARARGS, 1);
//	USE_LABEL(c, end);
//
// codegen_jump_if for a Name test (Python/codegen.c:1885) emits
// LOAD_*, TO_BOOL, POP_JUMP_IF_TRUE — all at LOC(test). The
// fall-through (`raise` branch) gets a NOT_TAKEN at the head
// (normalize_jumps_in_block, flowgraph.c:535) inheriting the jump's
// loc.
//
// Because assert is the terminator of the function body, this
// function also emits the implicit `return None` in the success
// branch (block bound to `end`). CPython's compile pass adds
// LOAD_CONST None + RETURN_VALUE there with NO_LOCATION, then
// propagate_line_numbers (flowgraph.c:3586) inherits LOC(test) into
// both. We set LOC(test) directly to skip porting that pass.
//
// Loc breakdown:
//
//	LOAD_*           @ LOC(test)
//	TO_BOOL          @ LOC(test)
//	POP_JUMP_IF_TRUE @ LOC(test)
//	NOT_TAKEN        @ LOC(test)
//	LOAD_COMMON_CONSTANT @ LOC(s) — full assert span
//	[opt msg load    @ LOC(msg)]
//	[opt CALL 0      @ LOC(s)]
//	RAISE_VARARGS 1  @ LOC(test)
//	LOAD_CONST None  @ LOC(test) — implicit return-None
//	RETURN_VALUE     @ LOC(test)
//
// Returns the bodyEndCol used by the outer FunctionDef Loc — the
// end col of the assert statement (msg.end if present, test.end
// otherwise).
//
// SOURCE: CPython 3.14 Python/codegen.c:2932-2954.
func emitFuncBodyAssert(u *compileUnit, a *ast.Assert, lines [][]byte) (uint16, error) {
	test := a.Test.(*ast.Name)
	testCol := uint16(test.P.Col)
	testEndCol := testCol + uint16(len(test.Id))
	testLoc := bytecode.Loc{
		Line: uint32(test.P.Line), EndLine: uint32(test.P.Line),
		Col: testCol, EndCol: testEndCol,
	}

	stmtCol := uint16(a.P.Col)
	stmtEndCol := testEndCol
	if a.Msg != nil {
		stmtEndCol = uint16(astExprEndCol(lines, a.P.Line, a.Msg))
	}
	stmtLoc := bytecode.Loc{
		Line: uint32(a.P.Line), EndLine: uint32(a.P.Line),
		Col: stmtCol, EndCol: stmtEndCol,
	}

	// codegen_jump_if Name arm: LOAD_*, TO_BOOL, POP_JUMP_IF_TRUE
	// — all three carry LOC(test).
	u.emitNameLoad(test.Id, testLoc)
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: testLoc},
	)
	endLabel := u.Seq.AllocLabel()
	block.AddJump(bytecode.POP_JUMP_IF_TRUE, endLabel, testLoc)

	// Fall-through (raise) block. NOT_TAKEN at LOC(test), then the
	// AssertionError construction at LOC(s).
	raiseBlock := u.Seq.AddBlock()
	raiseBlock.Instrs = append(raiseBlock.Instrs,
		ir.Instr{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: testLoc},
		ir.Instr{Op: bytecode.LOAD_COMMON_CONSTANT, Arg: 0, Loc: stmtLoc},
	)
	if a.Msg != nil {
		if _, _, err := visitFuncExpr(u, a.Msg, lines); err != nil {
			return 0, err
		}
		u.currentBlock().Instrs = append(u.currentBlock().Instrs,
			ir.Instr{Op: bytecode.CALL, Arg: 0, Loc: stmtLoc},
		)
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs,
		ir.Instr{Op: bytecode.RAISE_VARARGS, Arg: 1, Loc: testLoc},
	)

	// Success block bound to endLabel. Implicit return-None at
	// LOC(test) — the location propagate_line_numbers would inherit
	// from the jump source.
	endBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(endLabel, endBlock)
	noneIdx := u.addConst(nil)
	endBlock.Instrs = append(endBlock.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: testLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: testLoc},
	)
	return stmtEndCol, nil
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
