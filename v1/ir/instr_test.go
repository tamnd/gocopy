package ir

import (
	"reflect"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
)

// TestInstrBytesNoArg covers an opcode whose oparg fits in one
// byte and that has no inline cache. Output is the canonical
// (op, arg) pair.
func TestInstrBytesNoArg(t *testing.T) {
	in := Instr{Op: bytecode.NOP, Arg: 0}
	got := in.Bytes()
	want := []byte{byte(bytecode.NOP), 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bytes = %v, want %v", got, want)
	}
}

// TestInstrBytesWithCache covers a cached opcode (CALL has 3 cache
// words = 6 zero bytes appended).
func TestInstrBytesWithCache(t *testing.T) {
	in := Instr{Op: bytecode.CALL, Arg: 1}
	got := in.Bytes()
	want := []byte{byte(bytecode.CALL), 1, 0, 0, 0, 0, 0, 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bytes = %v, want %v", got, want)
	}
}

// TestInstrBytes16Bit forces one EXTENDED_ARG byte before the real
// opcode.
func TestInstrBytes16Bit(t *testing.T) {
	in := Instr{Op: bytecode.LOAD_CONST, Arg: 0x0102}
	got := in.Bytes()
	want := []byte{byte(extendedArg), 0x01, byte(bytecode.LOAD_CONST), 0x02}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bytes = %v, want %v", got, want)
	}
}

// TestInstrBytes24Bit forces two EXTENDED_ARG bytes.
func TestInstrBytes24Bit(t *testing.T) {
	in := Instr{Op: bytecode.LOAD_CONST, Arg: 0x010203}
	got := in.Bytes()
	want := []byte{
		byte(extendedArg), 0x01,
		byte(extendedArg), 0x02,
		byte(bytecode.LOAD_CONST), 0x03,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bytes = %v, want %v", got, want)
	}
}

// TestInstrBytes32Bit forces the maximum-length EXTENDED_ARG chain.
func TestInstrBytes32Bit(t *testing.T) {
	in := Instr{Op: bytecode.LOAD_CONST, Arg: 0x01020304}
	got := in.Bytes()
	want := []byte{
		byte(extendedArg), 0x01,
		byte(extendedArg), 0x02,
		byte(extendedArg), 0x03,
		byte(bytecode.LOAD_CONST), 0x04,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bytes = %v, want %v", got, want)
	}
}

func TestAllocLabelDense(t *testing.T) {
	s := NewInstrSeq()
	for i := uint32(1); i <= 5; i++ {
		got := s.AllocLabel()
		if got != LabelID(i) {
			t.Fatalf("AllocLabel = %d, want %d", got, i)
		}
	}
}

func TestAddBlockSequential(t *testing.T) {
	s := NewInstrSeq()
	b0 := s.AddBlock()
	b1 := s.AddBlock()
	if b0.ID != 0 || b1.ID != 1 {
		t.Fatalf("Block IDs = (%d, %d), want (0, 1)", b0.ID, b1.ID)
	}
	if len(s.Blocks) != 2 {
		t.Fatalf("Blocks len = %d, want 2", len(s.Blocks))
	}
}

func TestAddOpAppends(t *testing.T) {
	s := NewInstrSeq()
	b := s.AddBlock()
	b.AddOp(bytecode.NOP, 0, bytecode.Loc{})
	b.AddOp(bytecode.RETURN_VALUE, 0, bytecode.Loc{})
	if len(b.Instrs) != 2 {
		t.Fatalf("Instrs len = %d, want 2", len(b.Instrs))
	}
	if b.Instrs[0].Op != bytecode.NOP || b.Instrs[1].Op != bytecode.RETURN_VALUE {
		t.Fatalf("ops = %v %v", b.Instrs[0].Op, b.Instrs[1].Op)
	}
}
