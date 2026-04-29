package optimize

import (
	"reflect"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

func TestRunIdentityNil(t *testing.T) {
	if got := Run(nil); got != nil {
		t.Fatalf("Run(nil) = %v, want nil", got)
	}
}

func TestRunIdentityNonNil(t *testing.T) {
	seq := ir.NewInstrSeq()
	if got := Run(seq); got != seq {
		t.Fatalf("Run(seq) = %p, want same pointer %p", got, seq)
	}
}

func TestRunPreservesSingleBlock(t *testing.T) {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0},
		{Op: bytecode.LOAD_CONST, Arg: 0},
		{Op: bytecode.RETURN_VALUE, Arg: 0},
	}
	want := append([]ir.Instr(nil), b.Instrs...)
	got := Run(seq)
	if got != seq {
		t.Fatalf("Run dropped seq pointer")
	}
	if len(got.Blocks) != 1 {
		t.Fatalf("Run dropped blocks: blocks=%d, want 1", len(got.Blocks))
	}
	if !reflect.DeepEqual(got.Blocks[0].Instrs, want) {
		t.Fatalf("Run mutated single-block input:\n  got  %v\n  want %v", got.Blocks[0].Instrs, want)
	}
}

// TestRunBoolOpShape mirrors the visitor emit for `x = a and b` and
// verifies the optimizer produces the byte-resolved layout the v0.6
// classifier compileBoolOp emits.
func TestRunBoolOpShape(t *testing.T) {
	seq := ir.NewInstrSeq()
	b0 := seq.AddBlock()
	b1 := seq.AddBlock()
	b2 := seq.AddBlock()
	endLabel := seq.AllocLabel()
	seq.BindLabel(endLabel, b2)

	loc := bytecode.Loc{Line: 1, EndLine: 1, Col: 4, EndCol: 11}
	b0.Instrs = []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0},
		{Op: bytecode.LOAD_NAME, Arg: 0, Loc: loc},
		{Op: bytecode.COPY, Arg: 1, Loc: loc},
		{Op: bytecode.TO_BOOL, Arg: 0, Loc: loc},
	}
	b0.AddJump(bytecode.POP_JUMP_IF_FALSE, endLabel, loc)
	b1.Instrs = []ir.Instr{
		{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: loc},
		{Op: bytecode.POP_TOP, Arg: 0, Loc: loc},
		{Op: bytecode.LOAD_NAME, Arg: 1, Loc: loc},
	}
	b2.Instrs = []ir.Instr{
		{Op: bytecode.STORE_NAME, Arg: 2, Loc: loc},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: loc},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: loc},
	}

	got := Run(seq)
	if len(got.Blocks) != 1 {
		t.Fatalf("BoolOp: expected single flat block after Run, got %d", len(got.Blocks))
	}
	want := []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0},
		{Op: bytecode.LOAD_NAME, Arg: 0, Loc: loc},
		{Op: bytecode.COPY, Arg: 1, Loc: loc},
		{Op: bytecode.TO_BOOL, Arg: 0, Loc: loc},
		{Op: bytecode.POP_JUMP_IF_FALSE, Arg: 3, Loc: loc},
		{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: loc},
		{Op: bytecode.POP_TOP, Arg: 0, Loc: loc},
		{Op: bytecode.LOAD_NAME, Arg: 1, Loc: loc},
		{Op: bytecode.STORE_NAME, Arg: 2, Loc: loc},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: loc},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: loc},
	}
	if !reflect.DeepEqual(got.Blocks[0].Instrs, want) {
		t.Fatalf("BoolOp Run output mismatch:\n  got  %v\n  want %v", got.Blocks[0].Instrs, want)
	}
}

// TestRunIfExpShape mirrors the visitor emit for `x = a if c else b`
// and verifies the optimizer:
//   - inline_small_exit_blocks duplicates the merge-block tail
//     (STORE_NAME + LOAD_CONST None + RETURN_VALUE) into both
//     branches and drops the JUMP_FORWARD;
//   - resolve_jumps rewrites POP_JUMP_IF_FALSE's Arg from a
//     LabelID to the byte distance 5 the v0.6 classifier
//     compileTernary hard-codes.
func TestRunIfExpShape(t *testing.T) {
	seq := ir.NewInstrSeq()
	b0 := seq.AddBlock()
	b1 := seq.AddBlock()
	b2 := seq.AddBlock()
	b3 := seq.AddBlock()
	falseLabel := seq.AllocLabel()
	endLabel := seq.AllocLabel()
	seq.BindLabel(falseLabel, b2)
	seq.BindLabel(endLabel, b3)

	condLoc := bytecode.Loc{Line: 1, EndLine: 1, Col: 9, EndCol: 10}
	trueLoc := bytecode.Loc{Line: 1, EndLine: 1, Col: 4, EndCol: 5}
	falseLoc := bytecode.Loc{Line: 1, EndLine: 1, Col: 16, EndCol: 17}
	targetLoc := bytecode.Loc{Line: 1, EndLine: 1, Col: 0, EndCol: 1}

	b0.Instrs = []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0},
		{Op: bytecode.LOAD_NAME, Arg: 0, Loc: condLoc},
		{Op: bytecode.TO_BOOL, Arg: 0, Loc: condLoc},
	}
	b0.AddJump(bytecode.POP_JUMP_IF_FALSE, falseLabel, condLoc)

	b1.Instrs = []ir.Instr{
		{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc},
		{Op: bytecode.LOAD_NAME, Arg: 1, Loc: trueLoc},
	}
	b1.AddJump(bytecode.JUMP_FORWARD, endLabel, trueLoc)

	b2.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_NAME, Arg: 2, Loc: falseLoc},
	}

	b3.Instrs = []ir.Instr{
		{Op: bytecode.STORE_NAME, Arg: 3, Loc: targetLoc},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	}

	got := Run(seq)
	if len(got.Blocks) != 1 {
		t.Fatalf("IfExp: expected single flat block after Run, got %d", len(got.Blocks))
	}
	want := []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0},
		{Op: bytecode.LOAD_NAME, Arg: 0, Loc: condLoc},
		{Op: bytecode.TO_BOOL, Arg: 0, Loc: condLoc},
		{Op: bytecode.POP_JUMP_IF_FALSE, Arg: 5, Loc: condLoc},
		{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc},
		{Op: bytecode.LOAD_NAME, Arg: 1, Loc: trueLoc},
		{Op: bytecode.STORE_NAME, Arg: 3, Loc: targetLoc},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
		{Op: bytecode.LOAD_NAME, Arg: 2, Loc: falseLoc},
		{Op: bytecode.STORE_NAME, Arg: 3, Loc: targetLoc},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	}
	if !reflect.DeepEqual(got.Blocks[0].Instrs, want) {
		t.Fatalf("IfExp Run output mismatch:\n  got  %v\n  want %v", got.Blocks[0].Instrs, want)
	}
}

// TestRunRejectsBackwardJump asserts the v0.7.4 tripwire: backward
// jumps panic in resolveJumps until JUMP_BACKWARD lands at v0.7.6
// with visit_While.
func TestRunRejectsBackwardJump(t *testing.T) {
	seq := ir.NewInstrSeq()
	b0 := seq.AddBlock()
	b1 := seq.AddBlock()
	loopLabel := seq.AllocLabel()
	seq.BindLabel(loopLabel, b0)
	b0.Instrs = []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0},
	}
	b1.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0},
	}
	b1.AddJump(bytecode.JUMP_BACKWARD, loopLabel, bytecode.Loc{})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from backward jump, got none")
		}
	}()
	_ = Run(seq)
}
