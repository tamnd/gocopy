package flowgraph

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// TestPropagateLineNumbers_WithinBlock covers rule 1: a NO_LOCATION
// instruction in the middle of a block inherits the prior instr's
// loc.
func TestPropagateLineNumbers_WithinBlock(t *testing.T) {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 7, EndLine: 7, Col: 0, EndCol: 4}},
		{Op: bytecode.NOP, Arg: 0, Loc: bytecode.Loc{}},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{}},
	}
	propagateLineNumbers(seq)
	for i, want := range []uint32{7, 7, 7} {
		if got := b.Instrs[i].Loc.Line; got != want {
			t.Errorf("instr[%d] line: got %d want %d", i, got, want)
		}
	}
}

// TestPropagateLineNumbers_FallthroughSuccessor covers rule 2: a
// successor block with one predecessor inherits the predecessor's
// trailing loc into its first NO_LOCATION instruction.
func TestPropagateLineNumbers_FallthroughSuccessor(t *testing.T) {
	seq := ir.NewInstrSeq()
	a := seq.AddBlock()
	a.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 5}},
	}
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{}},
	}
	propagateLineNumbers(seq)
	if got := b.Instrs[0].Loc.Line; got != 5 {
		t.Errorf("successor first instr line: got %d want 5", got)
	}
}

// TestPropagateLineNumbers_JumpTarget covers rule 3: a jump target
// with one predecessor inherits the jump's loc.
func TestPropagateLineNumbers_JumpTarget(t *testing.T) {
	seq := ir.NewInstrSeq()
	a := seq.AddBlock()
	target := seq.AddBlock()
	targetLabel := seq.AllocLabel()
	seq.BindLabel(targetLabel, target)
	a.Instrs = []ir.Instr{
		{Op: bytecode.JUMP_FORWARD, Arg: uint32(targetLabel), Loc: bytecode.Loc{Line: 9}},
	}
	target.Instrs = []ir.Instr{
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{}},
	}
	propagateLineNumbers(seq)
	if got := target.Instrs[0].Loc.Line; got != 9 {
		t.Errorf("jump-target first instr line: got %d want 9", got)
	}
}

// TestPropagateLineNumbers_MultiPredecessorNotInherited covers the
// negative case: a block with 2+ predecessors does NOT inherit a
// loc, since the predecessors disagree.
func TestPropagateLineNumbers_MultiPredecessorNotInherited(t *testing.T) {
	seq := ir.NewInstrSeq()
	a := seq.AddBlock()
	b := seq.AddBlock()
	merge := seq.AddBlock()
	mergeLabel := seq.AllocLabel()
	seq.BindLabel(mergeLabel, merge)
	a.Instrs = []ir.Instr{
		{Op: bytecode.JUMP_FORWARD, Arg: uint32(mergeLabel), Loc: bytecode.Loc{Line: 3}},
	}
	b.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 5}},
	}
	merge.Instrs = []ir.Instr{
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{}},
	}
	propagateLineNumbers(seq)
	if got := merge.Instrs[0].Loc.Line; got != 0 {
		t.Errorf("multi-pred merge first instr line: got %d want 0 (untouched)", got)
	}
}
