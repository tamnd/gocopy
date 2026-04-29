package flowgraph

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

func TestOptimizeLoadConst_IntZero(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadConst(seq, []any{int64(0)})
	got := seq.Blocks[0].Instrs[0]
	if got.Op != bytecode.LOAD_SMALL_INT || got.Arg != 0 {
		t.Errorf("got %s arg=%d, want LOAD_SMALL_INT 0",
			bytecode.MetaOf(got.Op).Name, got.Arg)
	}
}

func TestOptimizeLoadConst_IntInRange(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadConst(seq, []any{int64(255)})
	got := seq.Blocks[0].Instrs[0]
	if got.Op != bytecode.LOAD_SMALL_INT || got.Arg != 255 {
		t.Errorf("got %s arg=%d, want LOAD_SMALL_INT 255",
			bytecode.MetaOf(got.Op).Name, got.Arg)
	}
}

func TestOptimizeLoadConst_IntTooLarge(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadConst(seq, []any{int64(256)})
	got := seq.Blocks[0].Instrs[0]
	if got.Op != bytecode.LOAD_CONST || got.Arg != 0 {
		t.Errorf("got %s arg=%d, want LOAD_CONST 0 (256 out of range)",
			bytecode.MetaOf(got.Op).Name, got.Arg)
	}
}

func TestOptimizeLoadConst_IntNegative(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadConst(seq, []any{int64(-1)})
	got := seq.Blocks[0].Instrs[0]
	if got.Op != bytecode.LOAD_CONST {
		t.Errorf("got %s, want LOAD_CONST (negative int out of range)",
			bytecode.MetaOf(got.Op).Name)
	}
}

func TestOptimizeLoadConst_None(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadConst(seq, []any{nil})
	got := seq.Blocks[0].Instrs[0]
	if got.Op != bytecode.LOAD_CONST {
		t.Errorf("got %s, want LOAD_CONST (None is not int)",
			bytecode.MetaOf(got.Op).Name)
	}
}

func TestOptimizeLoadConst_String(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadConst(seq, []any{"foo"})
	got := seq.Blocks[0].Instrs[0]
	if got.Op != bytecode.LOAD_CONST {
		t.Errorf("got %s, want LOAD_CONST (string is not int)",
			bytecode.MetaOf(got.Op).Name)
	}
}

func TestOptimizeLoadConst_NonLoadConstUntouched(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_FAST, Arg: 7, Loc: locL(1)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Loc: locL(1)},
	)
	OptimizeLoadConst(seq, []any{int64(5)})
	got := seq.Blocks[0].Instrs[0]
	if got.Op != bytecode.LOAD_FAST || got.Arg != 7 {
		t.Errorf("got %s arg=%d, want LOAD_FAST 7 (only LOAD_CONST is rewritten)",
			bytecode.MetaOf(got.Op).Name, got.Arg)
	}
}

func TestOptimizeLoadConst_NilSeq(t *testing.T) {
	OptimizeLoadConst(nil, []any{int64(0)})
}

func TestOptimizeLoadConst_OparOutOfRange(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 5, Loc: locL(1)},
	)
	OptimizeLoadConst(seq, []any{int64(0)})
	got := seq.Blocks[0].Instrs[0]
	if got.Op != bytecode.LOAD_CONST || got.Arg != 5 {
		t.Errorf("got %s arg=%d, want LOAD_CONST 5 unchanged when oparg outside pool",
			bytecode.MetaOf(got.Op).Name, got.Arg)
	}
}
