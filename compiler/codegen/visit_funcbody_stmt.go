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
	if len(s.Body) < 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	lastIdx := len(s.Body) - 1
	ret, ok := s.Body[lastIdx].(*ast.Return)
	if !ok || ret.Value == nil {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if ret.P.Col > 255 || !validateFuncBodyAssignRHS(ret.Value) {
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
		case *ast.If:
			if !validateFuncBodyIf(st) {
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
		case *ast.If:
			if err := emitFuncBodyIf(child, st, lines); err != nil {
				return bytecode.Loc{}, err
			}
		}
	}
	bodyEndCol, err := emitFuncBodyReturn(child, ret, lines)
	if err != nil {
		return bytecode.Loc{}, err
	}
	if len(child.Consts) == 0 {
		child.addConst(nil)
	}

	bodyEndLine := ret.P.Line

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
// a bare-Name or simple-Constant RHS. The local-Name RHS uses
// LOAD_FAST (move semantics) — CPython's optimize_load_fast does
// NOT promote the load-into-store path to LOAD_FAST_BORROW because
// STORE_FAST kills the borrowed reference. Globals and richer RHS
// expressions land in later v0.7.10 steps.
func emitFuncBodyAssign(u *compileUnit, a *ast.Assign, lines [][]byte) error {
	target := a.Targets[0].(*ast.Name)
	tgtLoc := bytecode.Loc{
		Line: uint32(target.P.Line), EndLine: uint32(target.P.Line),
		Col: uint16(target.P.Col), EndCol: uint16(target.P.Col) + uint16(len(target.Id)),
	}
	tgtKind, tgtArg := u.resolveNameOp(target.Id)

	block := u.currentBlock()
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
		block.Instrs = append(block.Instrs, ir.Instr{Op: bytecode.LOAD_FAST, Arg: rhsArg, Loc: rhsLoc})
	default:
		if _, _, err := visitFuncExpr(u, a.Value, lines); err != nil {
			return err
		}
	}
	if tgtKind != nameOpFast {
		return ErrNotImplemented
	}
	block.Instrs = append(block.Instrs, ir.Instr{Op: bytecode.STORE_FAST, Arg: tgtArg, Loc: tgtLoc})
	return nil
}

// emitFuncBodyAugAssign emits the IR for `target op= value` inside
// a function body. Mirrors compiler/func_body.go's AugAssign arm:
//
//   - LOAD_FAST_BORROW target — Loc spans target's identifier.
//     (When target and a Name RHS are both ≤ slot 15 the classifier
//     fuses LFLBLFLB; that fusion happens automatically via
//     visitFuncNameExpr's eager pair-fuse hook when the RHS recurse
//     emits a second LOAD_FAST_BORROW.)
//   - Recurse RHS via visitFuncExpr.
//   - BINARY_OP NbInplace* — Loc (target.startCol, rhs.endCol).
//   - STORE_FAST target — Loc spans target's identifier.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_stmt_aug_assign +
// optimize_load_fast (which the visitor pre-applies for byte parity).
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
		Op: bytecode.LOAD_FAST_BORROW, Arg: tgtArg, Loc: tgtLoc,
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

// validateFuncBodyIf reports whether ifs is one of the if-statement
// shapes the v0.7.10 visitor accepts:
//
//   - No Orelse (the trailing function-body return supplies the
//     "else").
//   - Body is exactly one *ast.Return with a non-nil value, OR a
//     single *ast.Assign with one Name target (validated via
//     validateFuncBodyAssign).
//   - Test is a single-op single-comparator *ast.Compare whose op
//     resolves via cmpOpFromAstOp and is not IS_OP (None checks
//     emit POP_JUMP_IF_NONE / POP_JUMP_IF_NOT_NONE in a separate
//     specialised path; not yet ported).
//   - Both Compare operands validate via validateFuncBodyAssignRHS,
//     as does the body's value when it is a Return.
func validateFuncBodyIf(ifs *ast.If) bool {
	if ifs.P.Col > 255 || len(ifs.Orelse) != 0 || len(ifs.Body) != 1 {
		return false
	}
	cmp, ok := ifs.Test.(*ast.Compare)
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
	switch body := ifs.Body[0].(type) {
	case *ast.Return:
		return body.Value != nil && body.P.Col <= 255 && validateFuncBodyAssignRHS(body.Value)
	case *ast.Assign:
		return validateFuncBodyAssign(body)
	}
	return false
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

// emitFuncBodyIf emits the gated-then If shape inside a function
// body. Mirrors compiler/func_body.go::isIfReturn / isIfAssign for
// the COMPARE_OP path:
//
//   - Walk Compare.Left and Compare.Comparators[0] via
//     visitFuncExpr (which produces LFLBLFLB for two-Name pairs).
//   - Emit COMPARE_OP with the conditional-context bit (oparg+16)
//     at Loc(condStart, condEnd).
//   - Emit POP_JUMP_IF_FALSE → afterThenLabel at the same Loc.
//   - AddBlock for the then-branch. Emit NOT_TAKEN at condLoc, then
//     the body Return / Assign.
//   - AddBlock + BindLabel(afterThenLabel) so the next body
//     statement (or trailing Return) emits into the kept-merge
//     block.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_stmt_if +
// the conditional-context COMPARE_OP specialisation in codegen_compare.
func emitFuncBodyIf(u *compileUnit, ifs *ast.If, lines [][]byte) error {
	cmp := ifs.Test.(*ast.Compare)
	op, base, _ := cmpOpFromAstOp(cmp.Ops[0])
	lc, _, err := visitFuncExpr(u, cmp.Left, lines)
	if err != nil {
		return err
	}
	_, re, err := visitFuncExpr(u, cmp.Comparators[0], lines)
	if err != nil {
		return err
	}
	condLoc := bytecode.Loc{
		Line: uint32(cmp.P.Line), EndLine: uint32(cmp.P.Line),
		Col: lc, EndCol: re,
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: op, Arg: uint32(base + 16), Loc: condLoc,
	})
	afterThenLabel := u.Seq.AllocLabel()
	block.AddJump(bytecode.POP_JUMP_IF_FALSE, afterThenLabel, condLoc)

	thenBlock := u.Seq.AddBlock()
	thenBlock.Instrs = append(thenBlock.Instrs, ir.Instr{
		Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc,
	})
	switch body := ifs.Body[0].(type) {
	case *ast.Return:
		if _, err := emitFuncBodyReturn(u, body, lines); err != nil {
			return err
		}
	case *ast.Assign:
		if err := emitFuncBodyAssign(u, body, lines); err != nil {
			return err
		}
	default:
		return ErrNotImplemented
	}

	afterBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(afterThenLabel, afterBlock)
	return nil
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
func emitFuncBodyReturn(u *compileUnit, ret *ast.Return, lines [][]byte) (uint16, error) {
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
	}
	return false
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
