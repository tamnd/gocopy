package codegen

import (
	"strconv"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitIfStmt lowers a top-level `if cond: body [elif cond: body
// ...] [else: body]` to multi-block IR. The Name-cond +
// single-Assign(name, smallInt) body restriction matches the
// v0.6.22 modIfElse classifier shape byte-for-byte.
//
// Structure (mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt_if + the per-branch
// implicit-return-None tail CPython produces by chaining
// codegen_visit_stmt_if into the module-level
// `compiler_unit_emit_implicit_return_none` and letting
// `inline_small_or_no_lineno_blocks` duplicate the small
// terminator into each branch):
//
//   - Allocate endLabel + nextLabel = endLabel in the no-orelse
//     case; allocate just nextLabel (fresh) in the with-orelse
//     case (endLabel is unused — every branch terminates with
//     RETURN_VALUE).
//   - Emit the cond head into the current block (LOAD_NAME +
//     TO_BOOL + POP_JUMP_IF_FALSE → nextLabel).
//   - AddBlock for the body. Emit NOT_TAKEN @ condLoc, the
//     body's `Assign(Name, smallInt)` as
//     LOAD_SMALL_INT @ valLoc + STORE_NAME @ varLoc, then the
//     implicit-return-None pair LOAD_CONST None @ varLoc +
//     RETURN_VALUE @ varLoc. The branch terminates here; no
//     JUMP_FORWARD is emitted.
//   - If has orelse: AddBlock + BindLabel(nextLabel) and recurse.
//     For `elif`, the orelse is a single nested If — call
//     visitIfStmt on it. For trailing `else`, the orelse is a
//     single Assign — emit the same body+return sequence at the
//     else's varLoc.
//   - For the no-orelse case, AddBlock + BindLabel(endLabel) and
//     fill the end block with LOAD_CONST None @ condLoc +
//     RETURN_VALUE @ condLoc (the kept-merge target the
//     conditional jump from the cond head lands on). For the
//     with-orelse case, no end block is emitted: every branch
//     terminates with RETURN_VALUE, so endLabel has no incoming
//     jumps and the seq tail is simply the last orelse block.
//   - Set u.tailEmitted = true so visitModule does not append its
//     own trailing LOAD_CONST None + RETURN_VALUE pair.
//
// Returns the first cond's Loc; visitModule does not consume the
// return when tailEmitted is set, but the value is preserved for
// shape-compatibility with other statement visitors.
func visitIfStmt(u *compileUnit, s *ast.If, source []byte, isLast bool) (bytecode.Loc, error) {
	if !isLast {
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

	hasOrelse := len(s.Orelse) > 0
	var endLabel, nextLabel ir.LabelID
	if hasOrelse {
		nextLabel = u.Seq.AllocLabel()
	} else {
		endLabel = u.Seq.AllocLabel()
		nextLabel = endLabel
	}

	condLoc, err := emitJumpIfFalse(u, s.Test, line, nextLabel)
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
	emitImplicitReturnNone(u, bodyVarLoc)

	if hasOrelse {
		nextBlock := u.Seq.AddBlock()
		u.Seq.BindLabel(nextLabel, nextBlock)

		if len(s.Orelse) != 1 {
			return bytecode.Loc{}, ErrNotImplemented
		}
		switch o := s.Orelse[0].(type) {
		case *ast.If:
			if _, err := visitIfStmt(u, o, source, true); err != nil {
				return bytecode.Loc{}, err
			}
		case *ast.Assign:
			elseVarLoc, err := emitIfBranchAssign(u, o)
			if err != nil {
				return bytecode.Loc{}, err
			}
			emitImplicitReturnNone(u, elseVarLoc)
		default:
			return bytecode.Loc{}, ErrNotImplemented
		}
	}

	if !hasOrelse {
		// No-else kept-merge: the conditional jump from the cond
		// head lands here. CPython attributes the trailing
		// implicit-return-None at this kept-merge to the cond's
		// source position (a LONG line-table entry with negative
		// lineDelta encodes it).
		endBlock := u.Seq.AddBlock()
		u.Seq.BindLabel(endLabel, endBlock)
		noneIdx := u.addConst(nil)
		endBlock.Instrs = append(endBlock.Instrs,
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: condLoc},
			ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: condLoc},
		)
	}

	u.tailEmitted = true
	return condLoc, nil
}

// emitJumpIfFalse mirrors the Name-test branch of CPython 3.14
// Python/codegen.c::codegen_jump_if (with cond=0): emits LOAD_NAME
// + TO_BOOL + POP_JUMP_IF_FALSE → label into the current block
// and returns the cond Name's Loc.
func emitJumpIfFalse(u *compileUnit, test ast.Expr, line uint32, label ir.LabelID) (bytecode.Loc, error) {
	name, ok := test.(*ast.Name)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if l := len(name.Id); l < 1 || l > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if name.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	condLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col:    uint16(name.P.Col),
		EndCol: uint16(name.P.Col) + uint16(len(name.Id)),
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(name.Id), Loc: condLoc},
		ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: condLoc},
	)
	block.AddJump(bytecode.POP_JUMP_IF_FALSE, label, condLoc)
	return condLoc, nil
}

// emitIfBranchAssign emits LOAD_SMALL_INT @ valLoc + STORE_NAME @
// varLoc for the v0.6.22 modIfElse body shape: a single
// `name = small_int` Assign where name is 1..15 chars at Col ≤ 255
// and the int is in 0..255 with Col ≤ 255. Mirrors the v0.6.22
// classifier's per-branch byte layout exactly. Returns the
// STORE_NAME's varLoc so the caller can anchor the implicit-return-
// None pair on it.
func emitIfBranchAssign(u *compileUnit, a *ast.Assign) (bytecode.Loc, error) {
	if len(a.Targets) != 1 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	varName, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if l := len(varName.Id); l < 1 || l > 15 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	if varName.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	constVal, ok := a.Value.(*ast.Constant)
	if !ok || constVal.Kind != "int" || constVal.P.Col > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}
	iv, ok := constVal.Value.(int64)
	if !ok || iv < 0 || iv > 255 {
		return bytecode.Loc{}, ErrNotImplemented
	}

	line := uint32(a.P.Line)
	valCol := uint16(constVal.P.Col)
	valLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col:    valCol,
		EndCol: valCol + uint16(len(strconv.Itoa(int(iv)))),
	}
	varCol := uint16(varName.P.Col)
	varLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col:    varCol,
		EndCol: varCol + uint16(len(varName.Id)),
	}

	if !u.phantomDone {
		u.addConst(iv)
		u.phantomDone = true
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: uint32(iv), Loc: valLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: u.addName(varName.Id), Loc: varLoc},
	)
	return varLoc, nil
}

// emitImplicitReturnNone emits LOAD_CONST None + RETURN_VALUE into
// the current block at loc. Used by visitIfStmt to terminate every
// branch of an if/elif/else chain; mirrors the per-branch tail
// CPython 3.14's `inline_small_or_no_lineno_blocks` produces by
// duplicating the module-level implicit return into each branch.
func emitImplicitReturnNone(u *compileUnit, loc bytecode.Loc) {
	noneIdx := u.addConst(nil)
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: loc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: loc},
	)
}
