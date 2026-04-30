package flowgraph

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

// TestRemoveUnreachable_OrphanBlock covers the headline use case:
// a block appearing AFTER an unconditional return (no jumps target
// it) gets its instrs zeroed.
func TestRemoveUnreachable_OrphanBlock(t *testing.T) {
	seq := ir.NewInstrSeq()
	a := seq.AddBlock()
	a.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 1}},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 1}},
	}
	dead := seq.AddBlock()
	dead.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 2}},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 2}},
	}
	removeUnreachable(seq)
	if len(a.Instrs) != 2 {
		t.Errorf("entry block touched: %v", a.Instrs)
	}
	if dead.Instrs != nil {
		t.Errorf("dead block not zeroed: %v", dead.Instrs)
	}
}

// TestRemoveUnreachable_KeepsJumpTarget covers the positive case: a
// block reached only via a jump (not slice-adjacent) survives.
func TestRemoveUnreachable_KeepsJumpTarget(t *testing.T) {
	seq := ir.NewInstrSeq()
	a := seq.AddBlock()
	target := seq.AddBlock()
	targetLabel := seq.AllocLabel()
	seq.BindLabel(targetLabel, target)
	a.Instrs = []ir.Instr{
		{Op: bytecode.JUMP_FORWARD, Arg: uint32(targetLabel), Loc: bytecode.Loc{Line: 1}},
	}
	target.Instrs = []ir.Instr{
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 2}},
	}
	removeUnreachable(seq)
	if len(target.Instrs) != 1 {
		t.Errorf("jump target zeroed: %v", target.Instrs)
	}
}

// TestRemoveUnreachable_FallthroughChain covers the BFS chain: a
// non-terminator block falls through to its slice successor, which
// must survive.
func TestRemoveUnreachable_FallthroughChain(t *testing.T) {
	seq := ir.NewInstrSeq()
	a := seq.AddBlock()
	b := seq.AddBlock()
	c := seq.AddBlock()
	a.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 1}},
	}
	b.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 2}},
	}
	c.Instrs = []ir.Instr{
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 3}},
	}
	removeUnreachable(seq)
	for i, blk := range []*ir.Block{a, b, c} {
		if len(blk.Instrs) == 0 {
			t.Errorf("block %d unexpectedly zeroed", i)
		}
	}
}
