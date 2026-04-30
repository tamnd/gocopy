package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
)

// emitCompareValue lowers `<left> <cmp> <right>` to IR for the
// single-Compare Name-vs-Name shape. Emits LOAD_NAME + LOAD_NAME +
// (COMPARE_OP / IS_OP / CONTAINS_OP) into the current block.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_expr_compare
// (single-comparator branch).
func emitCompareValue(u *compileUnit, c *ast.Compare, line uint32, _ uint16) error {
	if len(c.Ops) != 1 || len(c.Comparators) != 1 {
		return ErrNotImplemented
	}
	left, ok := c.Left.(*ast.Name)
	if !ok {
		return ErrNotImplemented
	}
	right, ok := c.Comparators[0].(*ast.Name)
	if !ok {
		return ErrNotImplemented
	}
	if l := len(left.Id); l < 1 || l > 15 {
		return ErrNotImplemented
	}
	if l := len(right.Id); l < 1 || l > 15 {
		return ErrNotImplemented
	}
	if left.P.Col > 255 || right.P.Col > 255 {
		return ErrNotImplemented
	}
	op, oparg, ok := cmpOpFromAstOp(c.Ops[0])
	if !ok {
		return ErrNotImplemented
	}

	leftLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(left.P.Col), EndCol: uint16(left.P.Col) + uint16(len(left.Id)),
	}
	rightLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(right.P.Col), EndCol: uint16(right.P.Col) + uint16(len(right.Id)),
	}
	cmpLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(left.P.Col), EndCol: uint16(right.P.Col) + uint16(len(right.Id)),
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(left.Id), Loc: leftLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(right.Id), Loc: rightLoc},
		ir.Instr{Op: op, Arg: uint32(oparg), Loc: cmpLoc},
	)
	return nil
}

// emitBoolOpValue lowers `<left> and|or <right>` (two Name operands)
// to multi-block IR with a forward conditional jump. Emits:
//
//   - into the current block: LOAD_NAME left, COPY 1, TO_BOOL 0,
//     POP_JUMP_IF_FALSE|POP_JUMP_IF_TRUE → endLabel.
//   - into a fresh right-branch block: NOT_TAKEN 0, POP_TOP 0,
//     LOAD_NAME right.
//   - into a fresh merge block bound to endLabel — left as
//     u.currentBlock() for the visitor tail (STORE_NAME, LOAD_CONST
//     None, RETURN_VALUE) to flow into.
//
// resolveJumps later rewrites POP_JUMP_IF_FALSE's Arg from the
// LabelID to the byte distance (3 in the canonical shape).
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_expr_boolop.
func emitBoolOpValue(u *compileUnit, b *ast.BoolOp, line uint32, _ uint16) error {
	if len(b.Values) != 2 {
		return ErrNotImplemented
	}
	left, ok := b.Values[0].(*ast.Name)
	if !ok {
		return ErrNotImplemented
	}
	right, ok := b.Values[1].(*ast.Name)
	if !ok {
		return ErrNotImplemented
	}
	if l := len(left.Id); l < 1 || l > 15 {
		return ErrNotImplemented
	}
	if l := len(right.Id); l < 1 || l > 15 {
		return ErrNotImplemented
	}
	if left.P.Col > 255 || right.P.Col > 255 {
		return ErrNotImplemented
	}
	var jumpOp bytecode.Opcode
	switch b.Op {
	case "And":
		jumpOp = bytecode.POP_JUMP_IF_FALSE
	case "Or":
		jumpOp = bytecode.POP_JUMP_IF_TRUE
	default:
		return ErrNotImplemented
	}

	leftLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(left.P.Col), EndCol: uint16(left.P.Col) + uint16(len(left.Id)),
	}
	spanLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(left.P.Col), EndCol: uint16(right.P.Col) + uint16(len(right.Id)),
	}
	rightLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(right.P.Col), EndCol: uint16(right.P.Col) + uint16(len(right.Id)),
	}

	endLabel := u.Seq.AllocLabel()
	entry := u.currentBlock()
	entry.Instrs = append(entry.Instrs,
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(left.Id), Loc: leftLoc},
		ir.Instr{Op: bytecode.COPY, Arg: 1, Loc: spanLoc},
		ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: spanLoc},
	)
	entry.AddJump(jumpOp, endLabel, spanLoc)

	rightBlock := u.Seq.AddBlock()
	rightBlock.Instrs = append(rightBlock.Instrs,
		ir.Instr{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: spanLoc},
		ir.Instr{Op: bytecode.POP_TOP, Arg: 0, Loc: spanLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(right.Id), Loc: rightLoc},
	)

	merge := u.Seq.AddBlock()
	u.Seq.BindLabel(endLabel, merge)
	return nil
}

// emitIfExpValue lowers `<true> if <cond> else <false>` (three Name
// operands) to multi-block IR with a forward conditional jump and a
// forward unconditional jump. Emits:
//
//   - into the current block: LOAD_NAME cond, TO_BOOL 0,
//     POP_JUMP_IF_FALSE → falseLabel.
//   - into a fresh true-branch block: NOT_TAKEN 0, LOAD_NAME true,
//     JUMP_FORWARD → endLabel.
//   - into a fresh false-branch block bound to falseLabel:
//     LOAD_NAME false.
//   - into a fresh merge block bound to endLabel — left as
//     u.currentBlock() for the visitor tail to flow into.
//
// inlineSmallOrNoLinenoBlocks duplicates the merge-block tail into both
// branches; resolveJumps then rewrites POP_JUMP_IF_FALSE's Arg
// (5 in the canonical shape) and drops the consumed JUMP_FORWARD.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_visit_expr_ifexp.
func emitIfExpValue(u *compileUnit, ifx *ast.IfExp, line uint32, _ uint16) error {
	cond, ok := ifx.Test.(*ast.Name)
	if !ok {
		return ErrNotImplemented
	}
	trueN, ok := ifx.Body.(*ast.Name)
	if !ok {
		return ErrNotImplemented
	}
	falseN, ok := ifx.OrElse.(*ast.Name)
	if !ok {
		return ErrNotImplemented
	}
	if l := len(cond.Id); l < 1 || l > 15 {
		return ErrNotImplemented
	}
	if l := len(trueN.Id); l < 1 || l > 15 {
		return ErrNotImplemented
	}
	if l := len(falseN.Id); l < 1 || l > 15 {
		return ErrNotImplemented
	}
	if cond.P.Col > 255 || trueN.P.Col > 255 || falseN.P.Col > 255 {
		return ErrNotImplemented
	}

	condLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(cond.P.Col), EndCol: uint16(cond.P.Col) + uint16(len(cond.Id)),
	}
	trueLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(trueN.P.Col), EndCol: uint16(trueN.P.Col) + uint16(len(trueN.Id)),
	}
	falseLoc := bytecode.Loc{
		Line: line, EndLine: line,
		Col: uint16(falseN.P.Col), EndCol: uint16(falseN.P.Col) + uint16(len(falseN.Id)),
	}

	falseLabel := u.Seq.AllocLabel()
	endLabel := u.Seq.AllocLabel()

	entry := u.currentBlock()
	entry.Instrs = append(entry.Instrs,
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(cond.Id), Loc: condLoc},
		ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: condLoc},
	)
	entry.AddJump(bytecode.POP_JUMP_IF_FALSE, falseLabel, condLoc)

	trueBlock := u.Seq.AddBlock()
	trueBlock.Instrs = append(trueBlock.Instrs,
		ir.Instr{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(trueN.Id), Loc: trueLoc},
	)
	trueBlock.AddJump(bytecode.JUMP_FORWARD, endLabel, trueLoc)

	falseBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(falseLabel, falseBlock)
	falseBlock.Instrs = append(falseBlock.Instrs,
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: u.addName(falseN.Id), Loc: falseLoc},
	)

	merge := u.Seq.AddBlock()
	u.Seq.BindLabel(endLabel, merge)
	return nil
}
