package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitWhileStmt lowers a top-level `while cond: name = val` loop
// to multi-block IR with a backward edge. The Name-cond +
// single-Assign(Name, smallInt) body restriction matches the
// v0.6.23 modWhile classifier shape byte-for-byte.
//
// Structure (mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt_while for the body-only,
// no-else case):
//
//   - Allocate topLabel (loop top, target of JUMP_BACKWARD) and
//     exitLabel (loop exit, target of POP_JUMP_IF_FALSE).
//   - AddBlock + BindLabel(topLabel). The entry block holds
//     RESUME from visitModule and falls through here. Emit the
//     cond head (LOAD_NAME + TO_BOOL + POP_JUMP_IF_FALSE →
//     exitLabel) into topBlock via emitJumpIfFalse.
//   - AddBlock for the body. Emit NOT_TAKEN @ condLoc, the
//     body's `Assign(Name, smallInt)` as
//     LOAD_SMALL_INT @ valLoc + STORE_NAME @ varLoc via
//     emitIfBranchAssign, then JUMP_BACKWARD @ varLoc → topLabel.
//   - AddBlock + BindLabel(exitLabel) and emit
//     LOAD_CONST None @ condLoc + RETURN_VALUE @ condLoc as the
//     loop-exit tail. condLoc carries the cond's source position
//     so the PEP 626 encoder produces a LONG entry with negative
//     lineDelta attributing the exit's bytes back to the cond's
//     line.
//   - Set u.tailEmitted = true so visitModule does not append its
//     own trailing LOAD_CONST None + RETURN_VALUE pair.
//
// Returns the cond's Loc; visitModule does not consume the return
// when tailEmitted is set, but the value is preserved for shape
// compatibility with other statement visitors.
func visitWhileStmt(u *compileUnit, s *ast.While, source []byte, isLast bool) (bytecode.Loc, error) {
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

	line := uint32(s.P.Line)
	if line < 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	topLabel := u.Seq.AllocLabel()
	exitLabel := u.Seq.AllocLabel()

	topBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(topLabel, topBlock)

	condLoc, err := emitJumpIfFalse(u, s.Test, line, exitLabel)
	if err != nil {
		return bytecode.Loc{}, err
	}

	body := u.Seq.AddBlock()
	body.Instrs = append(body.Instrs, ir.Instr{
		Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc,
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
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: condLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: condLoc},
	)

	u.tailEmitted = true
	return condLoc, nil
}
