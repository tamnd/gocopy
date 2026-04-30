package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/assemble"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
)

// visitFunctionDef lowers a top-level `def f(arg): return arg`
// definition where f and arg are 1..15-char identifiers, the body
// is a single `return <argName>` statement, and there are no
// decorators / annotations / type-params / defaults / vararg /
// kwarg. The shape matches the v0.6.25 modFuncDef classifier
// byte-for-byte.
//
// Structure (mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt_function_def +
// Python/compile.c::compiler_enter_scope/compiler_exit_scope for
// the no-closure case):
//
//   - Push a child compileUnit linked to the parent. The child's
//     Source/Filename are inherited; FirstLineNo is the def line.
//   - Emit RESUME 0 + LOAD_FAST 0 + RETURN_VALUE 0 into the
//     child's first block. Locs are chosen so EncodeLineTable
//     produces the same bytes as the classifier's
//     bytecode.FuncReturnArgLineTable hand-built table:
//       RESUME at Loc{defLine, defLine, 0, 0} — SHORT_FORM [0x80,
//       0x00] (lineDelta=0 vs FirstLineNo, col=endCol=0).
//       LOAD_FAST at Loc{bodyLine, bodyLine, argCol, argEnd}
//       — ONE_LINE_1 (lineDelta=1 from defLine).
//       RETURN_VALUE at Loc{bodyLine, bodyLine, retKwCol, argEnd} —
//       SHORT_FORM (lineDelta=0).
//     The LOAD_FAST → LOAD_FAST_BORROW promotion is owned by the
//     optimize_load_fast pass downstream.
//   - Add nil to the child's Consts pool so co_consts[0] = None
//     matches the classifier's funcCode.Consts shape (CPython's
//     compiler_codegen plants the implicit-return-None const at
//     index 0 of every function unit).
//   - Pop the child via popChildUnit, which runs optimize.Run +
//     assemble.Assemble with ArgCount=1, Flags=CO_OPTIMIZED|
//     CO_NEWLOCALS, LocalsPlusKinds=[LocalsKindArg]. Returns the
//     fully-assembled inner CodeObject.
//   - addConst(funcCode) into the parent unit (lands at
//     co_consts[0] at module scope) and addName(funcName) (lands
//     at co_names[0]).
//   - Emit LOAD_CONST funcCode + MAKE_FUNCTION + STORE_NAME funcName
//     into the parent's current block (the entry block holding
//     RESUME). All three share outerLoc = Loc{defLine, bodyLine,
//     0, argEnd}; visitModule's trailing LOAD_CONST None +
//     RETURN_VALUE share the same Loc and merge into one 5-CU
//     LONG line-table entry — byte-identical to
//     bytecode.FuncDefModuleLineTable.
//   - Do NOT set u.tailEmitted: visitModule's existing trailing
//     terminator emit at lastLoc=outerLoc completes the 5-CU run.
//
// Returns outerLoc; visitModule uses it as lastLoc.
func visitFunctionDef(u *compileUnit, s *ast.FunctionDef, source []byte, isLast bool) (bytecode.Loc, error) {
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
	if len(s.Body) != 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	ret, isReturn := s.Body[0].(*ast.Return)
	if !isReturn || ret.Value == nil {
		return bytecode.Loc{}, ErrNotImplemented
	}
	retName, isName := ret.Value.(*ast.Name)
	if !isName || retName.Id != arg.Name {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if ret.P.Col > 255 || retName.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	defLine := s.P.Line
	bodyLine := ret.P.Line
	if defLine < 1 || bodyLine < defLine {
		return bytecode.Loc{}, ErrNotImplemented
	}
	argCol := uint16(retName.P.Col)
	argEnd := argCol + uint16(len(retName.Id))
	retKwCol := uint16(ret.P.Col)

	outerLoc := bytecode.Loc{
		Line:    uint32(defLine),
		EndLine: uint32(bodyLine),
		Col:     0,
		EndCol:  argEnd,
	}

	child := u.pushChildUnit(s.Name, s.Name, int32(defLine))
	childBlock := child.Seq.AddBlock()

	resumeLoc := bytecode.Loc{
		Line: uint32(defLine), EndLine: uint32(defLine),
		Col: 0, EndCol: 0,
	}
	argLoc := bytecode.Loc{
		Line: uint32(bodyLine), EndLine: uint32(bodyLine),
		Col: argCol, EndCol: argEnd,
	}
	retLoc := bytecode.Loc{
		Line: uint32(bodyLine), EndLine: uint32(bodyLine),
		Col: retKwCol, EndCol: argEnd,
	}
	childBlock.Instrs = append(childBlock.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: resumeLoc},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: argLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: retLoc},
	)
	child.addConst(nil)

	funcCode, err := u.popChildUnit(child, assemble.Options{
		ArgCount:        1,
		Flags:           bytecode.CO_OPTIMIZED | bytecode.CO_NEWLOCALS,
		LocalsPlusNames: []string{arg.Name},
		LocalsPlusKinds: []byte{bytecode.LocalsKindArg},
		Filename:        u.Filename,
		Name:            s.Name,
		QualName:        s.Name,
	})
	if err != nil {
		return bytecode.Loc{}, err
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
