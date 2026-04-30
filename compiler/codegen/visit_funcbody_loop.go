package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// validateFuncBodyWhile reports whether w is a `while` shape the
// v0.7.10.12 funcbody visitor accepts. Required: Name test (1..15
// chars, col ≤ 255), no `else`, body of exactly one Assign whose RHS
// validates as an ordinary funcbody RHS.
//
// Out of scope (deferred to v0.7.10.13+): break/continue/non-empty
// while-else/multi-stmt body/non-Name test.
func validateFuncBodyWhile(w *ast.While) bool {
	if w == nil || w.P.Col > 255 || len(w.Orelse) != 0 {
		return false
	}
	tn, ok := w.Test.(*ast.Name)
	if !ok || tn.P.Col > 255 || len(tn.Id) < 1 || len(tn.Id) > 15 {
		return false
	}
	if len(w.Body) != 1 {
		return false
	}
	a, ok := w.Body[0].(*ast.Assign)
	if !ok {
		return false
	}
	return validateFuncBodyAssign(a)
}

// validateFuncBodyFor reports whether f is a `for` shape the
// v0.7.10.12 funcbody visitor accepts. Required: Name iter, Name
// target (1..15 chars each, col ≤ 255), no `else`, body of exactly
// one Assign whose RHS validates as an ordinary funcbody RHS.
//
// Out of scope (deferred to v0.7.10.13+): break/continue/non-empty
// for-else/multi-stmt body/Tuple target/Subscript iter.
func validateFuncBodyFor(f *ast.For) bool {
	if f == nil || f.P.Col > 255 || len(f.Orelse) != 0 {
		return false
	}
	iter, ok := f.Iter.(*ast.Name)
	if !ok || iter.P.Col > 255 || len(iter.Id) < 1 || len(iter.Id) > 15 {
		return false
	}
	tgt, ok := f.Target.(*ast.Name)
	if !ok || tgt.P.Col > 255 || len(tgt.Id) < 1 || len(tgt.Id) > 15 {
		return false
	}
	if len(f.Body) != 1 {
		return false
	}
	a, ok := f.Body[0].(*ast.Assign)
	if !ok {
		return false
	}
	return validateFuncBodyAssign(a)
}

// emitFuncBodyWhile lowers `while name: target = const` as the last
// statement of a function body. Mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt_while for the body-only,
// no-else case but emits STORE_FAST / LOAD_FAST_BORROW (via
// emitNameStore / emitNameLoad → optimize_load_fast pass) instead of
// the module-level STORE_NAME / LOAD_NAME the modWhile classifier
// emitted.
//
// Layout:
//
//	(top block) LOAD_FAST_BORROW cond + TO_BOOL + POP_JUMP_IF_FALSE → exit
//	(body)      NOT_TAKEN @ condLoc + body-Assign + JUMP_BACKWARD → top
//	(exit)      LOAD_CONST None + RETURN_VALUE — both at condLoc
//
// Returns the body's last varLoc and line so the outer FunctionDef
// Loc spans through the loop's last write.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_stmt_while.
func emitFuncBodyWhile(u *compileUnit, w *ast.While, lines [][]byte) (uint16, int, error) {
	cond := w.Test.(*ast.Name)
	bodyAssign := w.Body[0].(*ast.Assign)

	condCol := uint16(cond.P.Col)
	condEnd := condCol + uint16(len(cond.Id))
	condLoc := bytecode.Loc{
		Line: uint32(w.P.Line), EndLine: uint32(w.P.Line),
		Col: condCol, EndCol: condEnd,
	}

	topLabel := u.Seq.AllocLabel()
	exitLabel := u.Seq.AllocLabel()

	topBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(topLabel, topBlock)
	u.emitNameLoad(cond.Id, condLoc)
	topBlock.Instrs = append(topBlock.Instrs, ir.Instr{
		Op: bytecode.TO_BOOL, Arg: 0, Loc: condLoc,
	})
	topBlock.AddJump(bytecode.POP_JUMP_IF_FALSE, exitLabel, condLoc)

	body := u.Seq.AddBlock()
	body.Instrs = append(body.Instrs, ir.Instr{
		Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc,
	})
	if err := emitFuncBodyAssign(u, bodyAssign, lines); err != nil {
		return 0, 0, err
	}
	bodyVarLoc := bytecode.Loc{
		Line: uint32(bodyAssign.P.Line), EndLine: uint32(bodyAssign.P.Line),
		Col:    uint16(bodyAssign.Targets[0].(*ast.Name).P.Col),
		EndCol: uint16(bodyAssign.Targets[0].(*ast.Name).P.Col) + uint16(len(bodyAssign.Targets[0].(*ast.Name).Id)),
	}
	u.currentBlock().AddJump(bytecode.JUMP_BACKWARD, topLabel, bodyVarLoc)

	exitBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(exitLabel, exitBlock)
	noneIdx := u.addConst(nil)
	exitBlock.Instrs = append(exitBlock.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: condLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: condLoc},
	)
	// The function body's end line/col is the last source position of
	// the while body's last statement (its RHS), not the while header.
	// CPython's outer FunctionDef LOC spans through the body's end.
	bodyEndCol := uint16(astExprEndCol(lines, bodyAssign.P.Line, bodyAssign.Value))
	return bodyEndCol, bodyAssign.P.Line, nil
}

// emitFuncBodyFor lowers `for tgt in iter: bodyTgt = const` as the
// last statement of a function body. Mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt_for for the no-else,
// no-break/continue case.
//
// Layout:
//
//	(entry)     LOAD_FAST_BORROW iter + GET_ITER
//	(top block) FOR_ITER → exit
//	(body)      STORE_FAST tgt + body-Assign + JUMP_BACKWARD → top
//	(exit)      END_FOR + POP_ITER + LOAD_CONST None + RETURN_VALUE
//	            — all four at iterLoc
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_stmt_for.
func emitFuncBodyFor(u *compileUnit, f *ast.For, lines [][]byte) (uint16, int, error) {
	iter := f.Iter.(*ast.Name)
	tgt := f.Target.(*ast.Name)
	bodyAssign := f.Body[0].(*ast.Assign)

	line := f.P.Line
	iterCol := uint16(iter.P.Col)
	iterEnd := iterCol + uint16(len(iter.Id))
	iterLoc := bytecode.Loc{
		Line: uint32(line), EndLine: uint32(line),
		Col: iterCol, EndCol: iterEnd,
	}
	tgtCol := uint16(tgt.P.Col)
	tgtLoc := bytecode.Loc{
		Line: uint32(line), EndLine: uint32(line),
		Col:    tgtCol,
		EndCol: tgtCol + uint16(len(tgt.Id)),
	}

	entry := u.currentBlock()
	u.emitNameLoad(iter.Id, iterLoc)
	entry.Instrs = append(entry.Instrs, ir.Instr{
		Op: bytecode.GET_ITER, Arg: 0, Loc: iterLoc,
	})

	topLabel := u.Seq.AllocLabel()
	exitLabel := u.Seq.AllocLabel()
	topBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(topLabel, topBlock)
	topBlock.AddJump(bytecode.FOR_ITER, exitLabel, iterLoc)

	body := u.Seq.AddBlock()
	u.emitNameStore(tgt.Id, tgtLoc)
	_ = body
	if err := emitFuncBodyAssign(u, bodyAssign, lines); err != nil {
		return 0, 0, err
	}
	bodyTgt := bodyAssign.Targets[0].(*ast.Name)
	bodyVarLoc := bytecode.Loc{
		Line: uint32(bodyAssign.P.Line), EndLine: uint32(bodyAssign.P.Line),
		Col:    uint16(bodyTgt.P.Col),
		EndCol: uint16(bodyTgt.P.Col) + uint16(len(bodyTgt.Id)),
	}
	u.currentBlock().AddJump(bytecode.JUMP_BACKWARD, topLabel, bodyVarLoc)

	exitBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(exitLabel, exitBlock)
	noneIdx := u.addConst(nil)
	exitBlock.Instrs = append(exitBlock.Instrs,
		ir.Instr{Op: bytecode.END_FOR, Arg: 0, Loc: iterLoc},
		ir.Instr{Op: bytecode.POP_ITER, Arg: 0, Loc: iterLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: iterLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: iterLoc},
	)
	bodyEndCol := uint16(astExprEndCol(lines, bodyAssign.P.Line, bodyAssign.Value))
	return bodyEndCol, bodyAssign.P.Line, nil
}
