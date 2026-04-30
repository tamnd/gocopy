package flowgraph

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

func TestOptimizeLoadFast_TrivialReturn(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadFast(seq)
	got := seq.Blocks[0].Instrs
	if got[1].Op != bytecode.LOAD_FAST_BORROW {
		t.Errorf("got[1].Op = %s, want LOAD_FAST_BORROW",
			bytecode.MetaOf(got[1].Op).Name)
	}
}

func TestOptimizeLoadFast_StoredAsLocal(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.STORE_FAST, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadFast(seq)
	got := seq.Blocks[0].Instrs
	if got[0].Op != bytecode.LOAD_FAST {
		t.Errorf("got[0].Op = %s, want LOAD_FAST (loaded value is stored as local)",
			bytecode.MetaOf(got[0].Op).Name)
	}
}

func TestOptimizeLoadFast_BinopBothBorrow(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.BINARY_OP, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadFast(seq)
	got := seq.Blocks[0].Instrs
	if got[0].Op != bytecode.LOAD_FAST_BORROW {
		t.Errorf("got[0].Op = %s, want LOAD_FAST_BORROW",
			bytecode.MetaOf(got[0].Op).Name)
	}
	if got[1].Op != bytecode.LOAD_FAST_BORROW {
		t.Errorf("got[1].Op = %s, want LOAD_FAST_BORROW",
			bytecode.MetaOf(got[1].Op).Name)
	}
}

func TestOptimizeLoadFast_LFLF_Pair(t *testing.T) {
	// Pre-fused LOAD_FAST_LOAD_FAST consumed by BINARY_OP + RETURN.
	// Both refs are consumed before block end → promote to LFLBLFLB.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST_LOAD_FAST, Arg: (0 << 4) | 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.BINARY_OP, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadFast(seq)
	got := seq.Blocks[0].Instrs
	if got[0].Op != bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW {
		t.Errorf("got[0].Op = %s, want LOAD_FAST_BORROW_LOAD_FAST_BORROW",
			bytecode.MetaOf(got[0].Op).Name)
	}
}

func TestOptimizeLoadFast_RefUnconsumed(t *testing.T) {
	// LOAD_FAST with no consumer before block end. The block does
	// not terminate with anything that pops the ref — pretend it's
	// a YIELD-like point. Should stay LOAD_FAST.
	//
	// Use NOP as a non-consuming, non-terminator; the block has no
	// fallthrough successor (it's the only block) so the ref remains
	// at end-of-block.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.NOP, Loc: locL(1)},
	)
	OptimizeLoadFast(seq)
	got := seq.Blocks[0].Instrs
	if got[0].Op != bytecode.LOAD_FAST {
		t.Errorf("got[0].Op = %s, want LOAD_FAST (ref unconsumed at block end)",
			bytecode.MetaOf(got[0].Op).Name)
	}
}

func TestOptimizeLoadFast_KilledByStore(t *testing.T) {
	// LOAD_FAST 0; LOAD_FAST 1; STORE_FAST 2; RETURN_VALUE.
	//
	// SF 2 pops the LF 1 ref (storedAsLocal flag set on instr 1) and
	// kills any refs with local=2 (none — flags[0] untouched here).
	// Then RETURN_VALUE pops the remaining LF 0 ref cleanly. Result:
	//   - LF 0 has no flags → promoted to LF_BORROW.
	//   - LF 1 has storedAsLocal → stays LOAD_FAST.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.STORE_FAST, Arg: 2, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadFast(seq)
	got := seq.Blocks[0].Instrs
	if got[0].Op != bytecode.LOAD_FAST_BORROW {
		t.Errorf("got[0].Op = %s, want LOAD_FAST_BORROW",
			bytecode.MetaOf(got[0].Op).Name)
	}
	if got[1].Op != bytecode.LOAD_FAST {
		t.Errorf("got[1].Op = %s, want LOAD_FAST (storedAsLocal)",
			bytecode.MetaOf(got[1].Op).Name)
	}
}

func TestOptimizeLoadFast_NilSeq(t *testing.T) {
	OptimizeLoadFast(nil) // must not panic
}
