package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
)

// visitForStmt lowers a top-level
// `for loopVar in iter: bodyVar = val` loop to multi-block IR with
// a backward edge. The Name-iter + Name-target + single-Assign(Name,
// smallInt) body restriction matches the v0.6.24 modFor classifier
// shape byte-for-byte.
//
// Structure (mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt_for for the no-else,
// no-break/continue case):
//
//   - The iterator-protocol prologue (LOAD_NAME iter + GET_ITER)
//     goes into the entry block — the one holding RESUME from
//     visitModule. Re-running LOAD_NAME + GET_ITER every iteration
//     would be wrong; only FOR_ITER itself is the loop-top
//     instruction.
//   - Allocate topLabel (loop top, target of JUMP_BACKWARD) and
//     exitLabel (loop exit, target of FOR_ITER's exhausted-iter
//     edge).
//   - AddBlock + BindLabel(topLabel). Emit FOR_ITER → exitLabel
//     into topBlock.
//   - AddBlock for the body. Emit STORE_NAME loopVar @ loopVarLoc
//     to consume the next item FOR_ITER pushed; then the body's
//     `Assign(Name, smallInt)` as LOAD_SMALL_INT @ valLoc +
//     STORE_NAME @ varLoc via emitIfBranchAssign; then
//     JUMP_BACKWARD @ bodyVarLoc → topLabel.
//   - AddBlock + BindLabel(exitLabel) and emit
//     END_FOR + POP_ITER + LOAD_CONST None + RETURN_VALUE — all at
//     iterLoc. END_FOR consumes the StopIteration sentinel; POP_ITER
//     pops the iterator itself before the trailing return.
//     iterLoc carries the iter expression's source position so the
//     PEP 626 encoder produces a LONG entry attributing the exit's
//     bytes back to the for line.
//   - Set u.tailEmitted = true so visitModule does not append its
//     own trailing LOAD_CONST None + RETURN_VALUE pair.
//
// Returns the iter Loc; visitModule does not consume the return when
// tailEmitted is set, but the value is preserved for shape
// compatibility with other statement visitors.
func visitForStmt(u *compileUnit, s *ast.For, source []byte, isLast bool) (bytecode.Loc, error) {
	if !isLast {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(s.Orelse) != 0 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if len(s.Body) != 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	bodyAssign, ok := s.Body[0].(*ast.Assign)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	iter, ok := s.Iter.(*ast.Name)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if l := len(iter.Id); l < 1 || l > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if iter.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	target, ok := s.Target.(*ast.Name)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if l := len(target.Id); l < 1 || l > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if target.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	line := uint32(s.P.Line)
	if line < 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	iterCol := uint16(iter.P.Col)
	iterLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col:    iterCol,
		EndCol: iterCol + uint16(len(iter.Id)),
	}
	loopVarCol := uint16(target.P.Col)
	loopVarLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col:    loopVarCol,
		EndCol: loopVarCol + uint16(len(target.Id)),
	}

	entry := u.currentBlock()
	entry.Instrs = append(entry.Instrs,
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(iter.Id), Loc: iterLoc},
		ir.Instr{Op: bytecode.GET_ITER, Arg: 0, Loc: iterLoc},
	)

	topLabel := u.Seq.AllocLabel()
	exitLabel := u.Seq.AllocLabel()

	topBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(topLabel, topBlock)
	topBlock.AddJump(bytecode.FOR_ITER, exitLabel, iterLoc)

	body := u.Seq.AddBlock()
	body.Instrs = append(body.Instrs, ir.Instr{
		Op: bytecode.STORE_NAME, Arg: u.addName(target.Id), Loc: loopVarLoc,
	})
	bodyVarLoc, err := emitIfBranchAssign(u, bodyAssign)
	if err != nil {
		return bytecode.Loc{}, err
	}
	body.AddJump(bytecode.JUMP_BACKWARD, topLabel, bodyVarLoc)

	exitBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(exitLabel, exitBlock)
	noneIdx := u.addConst(nil)
	exitBlock.Instrs = append(exitBlock.Instrs,
		ir.Instr{Op: bytecode.END_FOR, Arg: 0, Loc: iterLoc},
		ir.Instr{Op: bytecode.POP_ITER, Arg: 0, Loc: iterLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: iterLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: iterLoc},
	)

	u.tailEmitted = true
	_ = source
	return iterLoc, nil
}
