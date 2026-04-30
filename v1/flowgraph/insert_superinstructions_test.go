package flowgraph

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

func locL(line uint32) bytecode.Loc {
	return bytecode.Loc{Line: line, EndLine: line}
}

func TestInsertSuperinstructions(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 3, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 5, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	InsertSuperinstructions(seq)
	got := seq.Blocks[0].Instrs
	if len(got) != 2 {
		t.Fatalf("instrs = %d, want 2 (fused pair + RETURN)", len(got))
	}
	if got[0].Op != bytecode.LOAD_FAST_LOAD_FAST {
		t.Errorf("got[0].Op = %s, want LOAD_FAST_LOAD_FAST", bytecode.MetaOf(got[0].Op).Name)
	}
	if got[0].Arg != (3<<4)|5 {
		t.Errorf("got[0].Arg = %d, want %d", got[0].Arg, (3<<4)|5)
	}
}

func TestInsertSuperinstructions_NoFuseSlot16Plus(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 16, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: locL(1)},
	)
	InsertSuperinstructions(seq)
	got := seq.Blocks[0].Instrs
	if len(got) != 2 {
		t.Fatalf("instrs = %d, want 2 (no fuse)", len(got))
	}
	if got[0].Op != bytecode.LOAD_FAST {
		t.Errorf("got[0].Op = %s, want LOAD_FAST (slot 16 must not fuse)", bytecode.MetaOf(got[0].Op).Name)
	}
}

func TestInsertSuperinstructions_StoreFastLoadFast(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.STORE_FAST, Arg: 2, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 7, Loc: locL(1)},
	)
	InsertSuperinstructions(seq)
	got := seq.Blocks[0].Instrs
	if len(got) != 1 || got[0].Op != bytecode.STORE_FAST_LOAD_FAST {
		t.Fatalf("got = %+v, want one STORE_FAST_LOAD_FAST", got)
	}
	if got[0].Arg != (2<<4)|7 {
		t.Errorf("Arg = %d, want %d", got[0].Arg, (2<<4)|7)
	}
}

func TestInsertSuperinstructions_StoreFastStoreFast(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.STORE_FAST, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.STORE_FAST, Arg: 4, Loc: locL(1)},
	)
	InsertSuperinstructions(seq)
	got := seq.Blocks[0].Instrs
	if len(got) != 1 || got[0].Op != bytecode.STORE_FAST_STORE_FAST {
		t.Fatalf("got = %+v, want one STORE_FAST_STORE_FAST", got)
	}
	if got[0].Arg != (1<<4)|4 {
		t.Errorf("Arg = %d, want %d", got[0].Arg, (1<<4)|4)
	}
}

func TestInsertSuperinstructions_LfbPairNotFused(t *testing.T) {
	// v0.7.10.4: insert_superinstructions no longer fuses
	// LOAD_FAST_BORROW pairs (the v0.7.10.3 bridge case is gone).
	// The visitor now emits raw LOAD_FAST and OptimizeLoadFast owns
	// the borrow promotion downstream of this pass.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST_BORROW, Arg: 1, Loc: locL(2)},
		ir.Instr{Op: bytecode.LOAD_FAST_BORROW, Arg: 2, Loc: locL(2)},
	)
	InsertSuperinstructions(seq)
	got := seq.Blocks[0].Instrs
	if len(got) != 2 {
		t.Fatalf("instrs = %d, want 2 (LFB+LFB no longer fused)", len(got))
	}
	if got[0].Op != bytecode.LOAD_FAST_BORROW || got[1].Op != bytecode.LOAD_FAST_BORROW {
		t.Errorf("ops = (%s, %s), want (LOAD_FAST_BORROW, LOAD_FAST_BORROW)",
			bytecode.MetaOf(got[0].Op).Name, bytecode.MetaOf(got[1].Op).Name)
	}
}

func TestInsertSuperinstructions_NoFuseAcrossLines(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: locL(3)},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 1, Loc: locL(4)},
	)
	InsertSuperinstructions(seq)
	got := seq.Blocks[0].Instrs
	if len(got) != 2 {
		t.Fatalf("instrs = %d, want 2 (lines differ → no fuse)", len(got))
	}
	if got[0].Op != bytecode.LOAD_FAST || got[1].Op != bytecode.LOAD_FAST {
		t.Errorf("ops = (%s, %s), want (LOAD_FAST, LOAD_FAST)",
			bytecode.MetaOf(got[0].Op).Name, bytecode.MetaOf(got[1].Op).Name)
	}
}

func TestInsertSuperinstructions_NoFuseUnrelatedPair(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.STORE_FAST, Arg: 1, Loc: locL(1)},
	)
	InsertSuperinstructions(seq)
	got := seq.Blocks[0].Instrs
	if len(got) != 2 {
		t.Fatalf("instrs = %d, want 2", len(got))
	}
	if got[0].Op != bytecode.LOAD_FAST || got[1].Op != bytecode.STORE_FAST {
		t.Errorf("ops = (%s, %s), want (LOAD_FAST, STORE_FAST)",
			bytecode.MetaOf(got[0].Op).Name, bytecode.MetaOf(got[1].Op).Name)
	}
}

func TestInsertSuperinstructions_NilSeq(t *testing.T) {
	InsertSuperinstructions(nil) // must not panic
}
