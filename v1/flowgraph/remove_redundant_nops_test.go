package flowgraph

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

// TestRemoveRedundantNops_NoLocation: rule 1 — NOP at NO_LOCATION
// drops unconditionally.
func TestRemoveRedundantNops_NoLocation(t *testing.T) {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 1}},
		{Op: bytecode.NOP, Arg: 0, Loc: bytecode.Loc{}},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 1}},
	}
	removeRedundantNops(seq)
	if len(b.Instrs) != 2 {
		t.Fatalf("expected 2 instrs, got %d: %v", len(b.Instrs), b.Instrs)
	}
	if b.Instrs[0].Op != bytecode.LOAD_CONST || b.Instrs[1].Op != bytecode.RETURN_VALUE {
		t.Errorf("wrong surviving instrs: %v", b.Instrs)
	}
}

// TestRemoveRedundantNops_SamePrevLine: rule 2 — NOP whose line
// matches the prior instr's line drops.
func TestRemoveRedundantNops_SamePrevLine(t *testing.T) {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 5}},
		{Op: bytecode.NOP, Arg: 0, Loc: bytecode.Loc{Line: 5}},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 5}},
	}
	removeRedundantNops(seq)
	if len(b.Instrs) != 2 {
		t.Fatalf("expected 2 instrs, got %d: %v", len(b.Instrs), b.Instrs)
	}
}

// TestRemoveRedundantNops_SameNextLine: rule 3 — NOP whose line
// matches the next instr's line drops.
func TestRemoveRedundantNops_SameNextLine(t *testing.T) {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.NOP, Arg: 0, Loc: bytecode.Loc{Line: 9}},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 9}},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 9}},
	}
	removeRedundantNops(seq)
	if len(b.Instrs) != 2 {
		t.Fatalf("expected 2 instrs, got %d: %v", len(b.Instrs), b.Instrs)
	}
	if b.Instrs[0].Op != bytecode.LOAD_CONST {
		t.Errorf("wrong head: %v", b.Instrs)
	}
}

// TestRemoveRedundantNops_NextNoLocation: rule 3b — NOP at line N,
// next instr at NO_LOCATION: NOP drops, next inherits line N.
func TestRemoveRedundantNops_NextNoLocation(t *testing.T) {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.NOP, Arg: 0, Loc: bytecode.Loc{Line: 12, Col: 4}},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{}},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 13}},
	}
	removeRedundantNops(seq)
	if len(b.Instrs) != 2 {
		t.Fatalf("expected 2 instrs, got %d", len(b.Instrs))
	}
	if got := b.Instrs[0].Loc.Line; got != 12 {
		t.Errorf("inherited loc line: got %d want 12", got)
	}
}

// TestRemoveRedundantNops_KeepsDistinct: NOP whose line is unique
// stays.
func TestRemoveRedundantNops_KeepsDistinct(t *testing.T) {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 1}},
		{Op: bytecode.NOP, Arg: 0, Loc: bytecode.Loc{Line: 2}},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 3}},
	}
	removeRedundantNops(seq)
	if len(b.Instrs) != 3 {
		t.Errorf("expected 3 instrs (NOP at unique line keeps), got %d", len(b.Instrs))
	}
}

// TestRemoveRedundantNops_TrailingMatchesNextBlock: rule 4 — last
// NOP in a block whose line matches the first significant instr of
// the next non-empty block drops.
func TestRemoveRedundantNops_TrailingMatchesNextBlock(t *testing.T) {
	seq := ir.NewInstrSeq()
	a := seq.AddBlock()
	b := seq.AddBlock()
	a.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: bytecode.Loc{Line: 1}},
		{Op: bytecode.NOP, Arg: 0, Loc: bytecode.Loc{Line: 4}},
	}
	b.Instrs = []ir.Instr{
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{Line: 4}},
	}
	removeRedundantNops(seq)
	if len(a.Instrs) != 1 {
		t.Errorf("expected NOP elided, got: %v", a.Instrs)
	}
}
