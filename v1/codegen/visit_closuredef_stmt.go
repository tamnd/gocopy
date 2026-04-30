package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/assemble"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
)

// visitClosureDef lowers a top-level
// `def f(x): def g(): return x; return g` definition where f, x,
// and g are 1..15-char identifiers, the inner function captures x
// as its sole free variable, and there are no decorators /
// annotations / type-params / defaults / vararg / kwarg anywhere.
// The shape matches the v0.6.26 modClosureDef classifier byte-for-
// byte.
//
// Returns ErrNotImplemented if the AST falls outside this surface;
// the visit_stmt.go FunctionDef arm then falls through to the
// simpler visitFunctionDef.
//
// Structure (mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt_function_def +
// Python/codegen.c::codegen_make_closure +
// Python/compile.c::compiler_make_cell +
// Python/compile.c::compiler_copy_free_vars):
//
//   - Push an outer compileUnit linked to the module unit. Inside,
//     push an inner compileUnit linked to the outer.
//   - Inner unit:
//
//     COPY_FREE_VARS 1 + RESUME 0 + LOAD_DEREF 0 + RETURN_VALUE 0,
//     co_consts=[None], LocalsPlusKinds=[LocalsKindFree],
//     Flags=0x13 (CO_OPTIMIZED|CO_NEWLOCALS|CO_NESTED).
//   - Outer unit:
//
//     MAKE_CELL 0 + RESUME 0 + LOAD_FAST 0 + BUILD_TUPLE 1 +
//     LOAD_CONST <innerCode> + MAKE_FUNCTION 0 +
//     SET_FUNCTION_ATTRIBUTE 8 + STORE_FAST 1 + LOAD_FAST 1 +
//     RETURN_VALUE 0. co_consts=[innerCode] (NO None — every path
//     returns explicitly via `return g`),
//     LocalsPlusKinds=[LocalsKindArgCell, LocalsKindLocal],
//     Flags=0x03. The LOAD_FAST → LOAD_FAST_BORROW promotion is
//     owned by the optimize_load_fast pass downstream.
//   - Module unit: emit LOAD_CONST <outerCode> + MAKE_FUNCTION 0 +
//     STORE_NAME f at moduleOuterLoc; visitModule's trailing
//     LOAD_CONST None + RETURN_VALUE share the same Loc and merge
//     into one 5-CU LONG entry — byte-identical to
//     bytecode.FuncDefModuleLineTable.
//
// Synthetic prologue ops (MAKE_CELL, COPY_FREE_VARS) carry a
// zero Loc; EncodeLineTable's IsZero branch emits the
// NO_INFO entry the classifier hand-built via
// entryHeader(codeNoInfo, 1).
//
// Returns moduleOuterLoc; visitModule uses it as lastLoc.
func visitClosureDef(u *compileUnit, s *ast.FunctionDef, source []byte, isLast bool) (bytecode.Loc, error) {
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
	if len(args.Args) != 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	arg := args.Args[0]
	if arg.Annotation != nil || arg.P.Col > 255 || len(arg.Name) < 1 || len(arg.Name) > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(s.Name) < 1 || len(s.Name) > 15 || s.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(s.Body) != 2 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	innerDef, ok := s.Body[0].(*ast.FunctionDef)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(innerDef.DecoratorList) != 0 || innerDef.Returns != nil || len(innerDef.TypeParams) != 0 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	innerArgs := innerDef.Args
	if innerArgs == nil || len(innerArgs.Args) != 0 || len(innerArgs.PosOnly) != 0 ||
		len(innerArgs.KwOnly) != 0 || innerArgs.Vararg != nil || innerArgs.Kwarg != nil ||
		len(innerArgs.Defaults) != 0 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(innerDef.Name) < 1 || len(innerDef.Name) > 15 || innerDef.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(innerDef.Body) != 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	innerRet, ok := innerDef.Body[0].(*ast.Return)
	if !ok || innerRet.Value == nil {
		return bytecode.Loc{}, ErrNotImplemented
	}
	innerRetName, ok := innerRet.Value.(*ast.Name)
	if !ok || innerRetName.Id != arg.Name {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if innerRet.P.Col > 255 || innerRetName.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	outerRet, ok := s.Body[1].(*ast.Return)
	if !ok || outerRet.Value == nil {
		return bytecode.Loc{}, ErrNotImplemented
	}
	outerRetName, ok := outerRet.Value.(*ast.Name)
	if !ok || outerRetName.Id != innerDef.Name {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if outerRet.P.Col > 255 || outerRetName.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	outerDefLine := s.P.Line
	innerDefLine := innerDef.P.Line
	innerRetLine := innerRet.P.Line
	outerRetLine := outerRet.P.Line
	if outerDefLine < 1 || innerDefLine < outerDefLine ||
		innerRetLine < innerDefLine || outerRetLine < innerRetLine {
		return bytecode.Loc{}, ErrNotImplemented
	}

	innerDefCol := uint16(innerDef.P.Col)
	innerFreeArgCol := uint16(innerRetName.P.Col)
	innerFreeArgEnd := innerFreeArgCol + uint16(len(innerRetName.Id))
	innerBodyEndCol := innerFreeArgEnd
	innerRetKwCol := uint16(innerRet.P.Col)
	outerRetArgCol := uint16(outerRetName.P.Col)
	outerRetArgEnd := outerRetArgCol + uint16(len(outerRetName.Id))
	outerRetKwCol := uint16(outerRet.P.Col)

	moduleOuterLoc := bytecode.Loc{
		Line:    uint32(outerDefLine),
		EndLine: uint32(outerRetLine),
		Col:     0,
		EndCol:  outerRetArgEnd,
	}

	outer := u.pushChildUnit(s.Name, s.Name, int32(outerDefLine))
	outerBlock := outer.Seq.AddBlock()

	innerQual := s.Name + ".<locals>." + innerDef.Name
	inner := outer.pushChildUnit(innerDef.Name, innerQual, int32(innerDefLine))
	innerBlock := inner.Seq.AddBlock()

	innerResumeLoc := bytecode.Loc{
		Line: uint32(innerDefLine), EndLine: uint32(innerDefLine),
		Col: 0, EndCol: 0,
	}
	innerLoadDerefLoc := bytecode.Loc{
		Line: uint32(innerRetLine), EndLine: uint32(innerRetLine),
		Col: innerFreeArgCol, EndCol: innerFreeArgEnd,
	}
	innerReturnLoc := bytecode.Loc{
		Line: uint32(innerRetLine), EndLine: uint32(innerRetLine),
		Col: innerRetKwCol, EndCol: innerFreeArgEnd,
	}
	innerBlock.Instrs = append(innerBlock.Instrs,
		ir.Instr{Op: bytecode.COPY_FREE_VARS, Arg: 1, Loc: bytecode.Loc{}},
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: innerResumeLoc},
		ir.Instr{Op: bytecode.LOAD_DEREF, Arg: 0, Loc: innerLoadDerefLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: innerReturnLoc},
	)
	inner.addConst(nil)

	innerCode, err := outer.popChildUnit(inner, assemble.Options{
		ArgCount:        0,
		Flags:           bytecode.CO_OPTIMIZED | bytecode.CO_NEWLOCALS | bytecode.CO_NESTED,
		LocalsPlusNames: []string{arg.Name},
		LocalsPlusKinds: []byte{bytecode.LocalsKindFree},
		Filename:        u.Filename,
		Name:            innerDef.Name,
		QualName:        innerQual,
	})
	if err != nil {
		return bytecode.Loc{}, err
	}

	outerResumeLoc := bytecode.Loc{
		Line: uint32(outerDefLine), EndLine: uint32(outerDefLine),
		Col: 0, EndCol: 0,
	}
	innerDefRunLoc := bytecode.Loc{
		Line:    uint32(innerDefLine),
		EndLine: uint32(innerRetLine),
		Col:     innerDefCol,
		EndCol:  innerBodyEndCol,
	}
	outerLoadGLoc := bytecode.Loc{
		Line: uint32(outerRetLine), EndLine: uint32(outerRetLine),
		Col: outerRetArgCol, EndCol: outerRetArgEnd,
	}
	outerReturnLoc := bytecode.Loc{
		Line: uint32(outerRetLine), EndLine: uint32(outerRetLine),
		Col: outerRetKwCol, EndCol: outerRetArgEnd,
	}

	innerCodeIdx := outer.addConst(innerCode)
	outerBlock.Instrs = append(outerBlock.Instrs,
		ir.Instr{Op: bytecode.MAKE_CELL, Arg: 0, Loc: bytecode.Loc{}},
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: outerResumeLoc},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: innerDefRunLoc},
		ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: 1, Loc: innerDefRunLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: innerCodeIdx, Loc: innerDefRunLoc},
		ir.Instr{Op: bytecode.MAKE_FUNCTION, Arg: 0, Loc: innerDefRunLoc},
		ir.Instr{Op: bytecode.SET_FUNCTION_ATTRIBUTE, Arg: 8, Loc: innerDefRunLoc},
		ir.Instr{Op: bytecode.STORE_FAST, Arg: 1, Loc: innerDefRunLoc},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 1, Loc: outerLoadGLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: outerReturnLoc},
	)

	outerCode, err := u.popChildUnit(outer, assemble.Options{
		ArgCount:        1,
		Flags:           bytecode.CO_OPTIMIZED | bytecode.CO_NEWLOCALS,
		LocalsPlusNames: []string{arg.Name, innerDef.Name},
		LocalsPlusKinds: []byte{bytecode.LocalsKindArgCell, bytecode.LocalsKindLocal},
		Filename:        u.Filename,
		Name:            s.Name,
		QualName:        s.Name,
	})
	if err != nil {
		return bytecode.Loc{}, err
	}

	outerCodeIdx := u.addConst(outerCode)
	outerNameIdx := u.addName(s.Name)

	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: outerCodeIdx, Loc: moduleOuterLoc},
		ir.Instr{Op: bytecode.MAKE_FUNCTION, Arg: 0, Loc: moduleOuterLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: outerNameIdx, Loc: moduleOuterLoc},
	)

	_ = source
	return moduleOuterLoc, nil
}
