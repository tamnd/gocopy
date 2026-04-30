package flowgraph

import (
	"reflect"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

func TestFoldTupleOfConstants_LoadConstChain(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: 2, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	consts := []any{int64(1), int64(2)}
	got := FoldTupleOfConstants(seq, consts)

	if len(got) != 3 {
		t.Fatalf("consts len = %d, want 3", len(got))
	}
	tup, ok := got[2].(bytecode.ConstTuple)
	if !ok {
		t.Fatalf("got[2] type = %T, want bytecode.ConstTuple", got[2])
	}
	if !reflect.DeepEqual([]any(tup), []any{int64(1), int64(2)}) {
		t.Errorf("folded tuple = %v, want (1, 2)", []any(tup))
	}
	instrs := seq.Blocks[0].Instrs
	if len(instrs) != 2 {
		t.Fatalf("instrs len = %d, want 2 (loaders dropped)", len(instrs))
	}
	if instrs[0].Op != bytecode.LOAD_CONST || instrs[0].Arg != 2 {
		t.Errorf("instrs[0] = %s arg=%d, want LOAD_CONST 2",
			bytecode.MetaOf(instrs[0].Op).Name, instrs[0].Arg)
	}
	if instrs[1].Op != bytecode.RETURN_VALUE {
		t.Errorf("instrs[1] = %s, want RETURN_VALUE",
			bytecode.MetaOf(instrs[1].Op).Name)
	}
}

func TestFoldTupleOfConstants_LoadSmallIntChain(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: 2, Loc: locL(1)},
		ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: 2, Loc: locL(1)},
	)
	got := FoldTupleOfConstants(seq, nil)
	if len(got) != 1 {
		t.Fatalf("consts len = %d, want 1", len(got))
	}
	tup := got[0].(bytecode.ConstTuple)
	if !reflect.DeepEqual([]any(tup), []any{int64(1), int64(2)}) {
		t.Errorf("folded tuple = %v, want (1, 2)", []any(tup))
	}
	if len(seq.Blocks[0].Instrs) != 1 ||
		seq.Blocks[0].Instrs[0].Op != bytecode.LOAD_CONST {
		t.Fatalf("instrs after fold = %v, want single LOAD_CONST", seq.Blocks[0].Instrs)
	}
}

func TestFoldTupleOfConstants_SingleElement(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: 1, Loc: locL(1)},
	)
	got := FoldTupleOfConstants(seq, []any{int64(7)})
	if len(got) != 2 {
		t.Fatalf("consts len = %d, want 2", len(got))
	}
	tup := got[1].(bytecode.ConstTuple)
	if !reflect.DeepEqual([]any(tup), []any{int64(7)}) {
		t.Errorf("folded tuple = %v, want (7,)", []any(tup))
	}
}

func TestFoldTupleOfConstants_MixedLoaderRefuses(t *testing.T) {
	// LOAD_NAME + LOAD_CONST + BUILD_TUPLE 2 — must NOT fold.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: 2, Loc: locL(1)},
	)
	consts := []any{int64(1)}
	got := FoldTupleOfConstants(seq, consts)
	if len(got) != 1 {
		t.Fatalf("consts len = %d, want 1 (no fold)", len(got))
	}
	if seq.Blocks[0].Instrs[2].Op != bytecode.BUILD_TUPLE {
		t.Errorf("BUILD_TUPLE rewritten unexpectedly: %s",
			bytecode.MetaOf(seq.Blocks[0].Instrs[2].Op).Name)
	}
}

func TestFoldTupleOfConstants_TooFewLoaders(t *testing.T) {
	// Only one LOAD_CONST before BUILD_TUPLE 2 — refuse.
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: 2, Loc: locL(1)},
	)
	got := FoldTupleOfConstants(seq, []any{int64(1)})
	if len(got) != 1 {
		t.Fatalf("consts len = %d, want 1 (no fold)", len(got))
	}
	if seq.Blocks[0].Instrs[1].Op != bytecode.BUILD_TUPLE {
		t.Errorf("BUILD_TUPLE rewritten unexpectedly")
	}
}

func TestFoldTupleOfConstants_NoBuildTuple(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	consts := []any{int64(1)}
	got := FoldTupleOfConstants(seq, consts)
	if !reflect.DeepEqual(got, consts) {
		t.Errorf("consts changed unexpectedly: got %v want %v", got, consts)
	}
}

func TestFoldTupleOfConstants_NilSeq(t *testing.T) {
	got := FoldTupleOfConstants(nil, []any{int64(1)})
	if len(got) != 1 {
		t.Errorf("got %v, want unchanged", got)
	}
}

func TestFoldTupleOfConstants_TwoBuildTuplesInOneBlock(t *testing.T) {
	// First chain: LOAD_CONST 0 + LOAD_CONST 1 + BUILD_TUPLE 2 → fold (1,2)
	// Second chain: LOAD_CONST 2 + BUILD_TUPLE 1 → fold ("x",)
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1, Loc: locL(1)},
		ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: 2, Loc: locL(1)},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 2, Loc: locL(1)},
		ir.Instr{Op: bytecode.BUILD_TUPLE, Arg: 1, Loc: locL(1)},
	)
	consts := []any{int64(1), int64(2), "x"}
	got := FoldTupleOfConstants(seq, consts)
	// Expect two new tuple consts appended.
	if len(got) != 5 {
		t.Fatalf("consts len = %d, want 5", len(got))
	}
	if !reflect.DeepEqual([]any(got[3].(bytecode.ConstTuple)), []any{int64(1), int64(2)}) {
		t.Errorf("first fold = %v, want (1,2)", got[3])
	}
	if !reflect.DeepEqual([]any(got[4].(bytecode.ConstTuple)), []any{"x"}) {
		t.Errorf("second fold = %v, want (\"x\",)", got[4])
	}
	// After fold the block should hold just two LOAD_CONST instrs.
	instrs := seq.Blocks[0].Instrs
	if len(instrs) != 2 {
		t.Fatalf("instrs len = %d, want 2", len(instrs))
	}
	if instrs[0].Op != bytecode.LOAD_CONST || instrs[0].Arg != 3 {
		t.Errorf("instrs[0] = %s arg=%d, want LOAD_CONST 3",
			bytecode.MetaOf(instrs[0].Op).Name, instrs[0].Arg)
	}
	if instrs[1].Op != bytecode.LOAD_CONST || instrs[1].Arg != 4 {
		t.Errorf("instrs[1] = %s arg=%d, want LOAD_CONST 4",
			bytecode.MetaOf(instrs[1].Op).Name, instrs[1].Arg)
	}
}
