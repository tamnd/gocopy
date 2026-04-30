package flowgraph

import (
	"reflect"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

func TestRemoveUnusedConsts_AllUsed(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	consts := []any{nil, "foo"}
	got := RemoveUnusedConsts(seq, consts)
	if !reflect.DeepEqual(got, []any{nil, "foo"}) {
		t.Errorf("got %v, want [nil, foo]", got)
	}
	if seq.Blocks[0].Instrs[0].Arg != 0 {
		t.Errorf("instr[0].Arg = %d, want 0", seq.Blocks[0].Instrs[0].Arg)
	}
	if seq.Blocks[0].Instrs[1].Arg != 1 {
		t.Errorf("instr[1].Arg = %d, want 1", seq.Blocks[0].Instrs[1].Arg)
	}
}

func TestRemoveUnusedConsts_Slot0AlwaysKept(t *testing.T) {
	// consts[0] = 7, never referenced (LOAD_CONST was rewritten to
	// LOAD_SMALL_INT by optimize_load_const). Pool must keep slot 0
	// — this is the docstring slot, always reserved.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: 7, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	consts := []any{int64(7)}
	got := RemoveUnusedConsts(seq, consts)
	if !reflect.DeepEqual(got, []any{int64(7)}) {
		t.Errorf("got %v, want [7] (slot 0 always kept)", got)
	}
}

func TestRemoveUnusedConsts_DropMiddle(t *testing.T) {
	// consts = [None, "unused", "kept"], LOAD_CONST 2 used, slot 0
	// kept. Slot 1 dropped, slot 2 remapped to 1.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 2, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	consts := []any{nil, "unused", "kept"}
	got := RemoveUnusedConsts(seq, consts)
	want := []any{nil, "kept"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if seq.Blocks[0].Instrs[0].Arg != 1 {
		t.Errorf("instr[0].Arg = %d, want 1 (LOAD_CONST 2 → LOAD_CONST 1)",
			seq.Blocks[0].Instrs[0].Arg)
	}
}

func TestRemoveUnusedConsts_DropTrailing(t *testing.T) {
	// consts = ["doc", "tail"], no LOAD_CONST references — slot 0
	// kept (docstring), slot 1 dropped.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	consts := []any{"doc", "tail"}
	got := RemoveUnusedConsts(seq, consts)
	want := []any{"doc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRemoveUnusedConsts_RemapAllSurviving(t *testing.T) {
	// consts = [None, A, B, C], LOAD_CONST 1 and LOAD_CONST 3 used.
	// Slot 0 kept, slot 2 dropped. Index map: 0→0, 1→1, 3→2.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 3, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	consts := []any{nil, "A", "B", "C"}
	got := RemoveUnusedConsts(seq, consts)
	want := []any{nil, "A", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if seq.Blocks[0].Instrs[0].Arg != 2 {
		t.Errorf("instr[0].Arg = %d, want 2 (LOAD_CONST 3 → 2)",
			seq.Blocks[0].Instrs[0].Arg)
	}
	if seq.Blocks[0].Instrs[1].Arg != 1 {
		t.Errorf("instr[1].Arg = %d, want 1 (LOAD_CONST 1 → 1)",
			seq.Blocks[0].Instrs[1].Arg)
	}
}

func TestRemoveUnusedConsts_EmptyPool(t *testing.T) {
	seq := mkSeq(ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)})
	got := RemoveUnusedConsts(seq, nil)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestRemoveUnusedConsts_NilSeq(t *testing.T) {
	got := RemoveUnusedConsts(nil, []any{int64(7)})
	if !reflect.DeepEqual(got, []any{int64(7)}) {
		t.Errorf("got %v, want [7]", got)
	}
}

func TestRemoveUnusedConsts_AcrossBlocks(t *testing.T) {
	// Two blocks, each referencing different const indices.
	seq := ir.NewInstrSeq()
	b0 := seq.AddBlock()
	b0.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 1, Loc: locL(1)},
	}
	b1 := seq.AddBlock()
	b1.Instrs = []ir.Instr{
		{Op: bytecode.LOAD_CONST, Arg: 2, Loc: locL(2)},
		{Op: bytecode.RETURN_VALUE, Loc: locL(2)},
	}
	consts := []any{nil, "A", "B", "unused"}
	got := RemoveUnusedConsts(seq, consts)
	want := []any{nil, "A", "B"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if seq.Blocks[0].Instrs[0].Arg != 1 {
		t.Errorf("blocks[0][0].Arg = %d, want 1", seq.Blocks[0].Instrs[0].Arg)
	}
	if seq.Blocks[1].Instrs[0].Arg != 2 {
		t.Errorf("blocks[1][0].Arg = %d, want 2", seq.Blocks[1].Instrs[0].Arg)
	}
}
