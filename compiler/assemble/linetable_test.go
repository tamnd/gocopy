package assemble

import (
	"bytes"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

func mkSeq(firstLine int32, instrs ...ir.Instr) *ir.InstrSeq {
	seq := ir.NewInstrSeq()
	seq.FirstLineNo = firstLine
	b := seq.AddBlock()
	b.Instrs = append(b.Instrs, instrs...)
	return seq
}

func TestEncodeLineTableEmpty(t *testing.T) {
	got := EncodeLineTable(mkSeq(1))
	if len(got) != 0 {
		t.Fatalf("empty seq linetable = %x, want empty", got)
	}
}

func TestEncodeLineTableNone(t *testing.T) {
	// Loc with all-zero fields encodes as NONE (code 15). One
	// instruction (1 code unit) → entry header 0xf8.
	seq := mkSeq(1, ir.Instr{Op: bytecode.NOP, Arg: 0, Loc: bytecode.Loc{}})
	got := EncodeLineTable(seq)
	want := []byte{0xf8}
	if !bytes.Equal(got, want) {
		t.Fatalf("NONE encoding = %x, want %x", got, want)
	}
}

func TestEncodeLineTableShortForm(t *testing.T) {
	// SHORT_FORM is selected when line_delta == 0, col < 80,
	// end_col-col < 16, end_col >= col. firstLine=5, instr at line 5.
	loc := bytecode.Loc{Line: 5, EndLine: 5, Col: 16, EndCol: 20}
	seq := mkSeq(5, ir.Instr{Op: bytecode.NOP, Loc: loc})
	got := EncodeLineTable(seq)
	// col=16 → group=2, low=0; end_col-col=4. Header = 0x80|(2<<3)|0 = 0x90.
	// Payload byte = (low<<4)|delta = 0x04.
	want := []byte{0x90, 0x04}
	if !bytes.Equal(got, want) {
		t.Fatalf("SHORT_FORM encoding = %x, want %x", got, want)
	}
}

func TestEncodeLineTableOneLine(t *testing.T) {
	// ONE_LINE_1 when line_delta == 1.
	loc := bytecode.Loc{Line: 6, EndLine: 6, Col: 0, EndCol: 16}
	seq := mkSeq(5, ir.Instr{Op: bytecode.NOP, Loc: loc})
	got := EncodeLineTable(seq)
	// header = 0x80|(11<<3)|0 = 0xd8. payload = 0x00, 0x10.
	want := []byte{0xd8, 0x00, 0x10}
	if !bytes.Equal(got, want) {
		t.Fatalf("ONE_LINE_1 encoding = %x, want %x", got, want)
	}
}

func TestEncodeLineTableLongFallback(t *testing.T) {
	// Synthetic prologue: Loc {Line:0, EndLine:1, Col:0, EndCol:0}.
	// firstLine=1 → line_delta=-1.
	loc := bytecode.Loc{Line: 0, EndLine: 1, Col: 0, EndCol: 0}
	seq := mkSeq(1, ir.Instr{Op: bytecode.RESUME, Loc: loc})
	got := EncodeLineTable(seq)
	// header = 0x80|(14<<3)|0 = 0xf0. svarint(-1)=0x03. varint(1)=0x01.
	// col_payload = 0+1 = 0x01. end_col_payload = 0+1 = 0x01.
	want := []byte{0xf0, 0x03, 0x01, 0x01, 0x01}
	if !bytes.Equal(got, want) {
		t.Fatalf("LONG synthetic prologue = %x, want %x", got, want)
	}
}

func TestEncodeLineTableRunMerging(t *testing.T) {
	// Two consecutive instructions sharing a Loc collapse into one
	// entry covering 2 code units.
	loc := bytecode.Loc{Line: 1, EndLine: 1, Col: 0, EndCol: 4}
	seq := mkSeq(1,
		ir.Instr{Op: bytecode.NOP, Loc: loc},
		ir.Instr{Op: bytecode.NOP, Loc: loc},
	)
	got := EncodeLineTable(seq)
	// SHORT_FORM, length-1=1. col=0 → group=0, low=0; end-col=4.
	// header = 0x80|(0<<3)|1 = 0x81. payload = 0x04.
	want := []byte{0x81, 0x04}
	if !bytes.Equal(got, want) {
		t.Fatalf("run merging = %x, want %x", got, want)
	}
}

func TestEncodeLineTableRunSplitOver8(t *testing.T) {
	// A run spanning 10 code units splits into 8 + 2.
	loc := bytecode.Loc{Line: 1, EndLine: 1, Col: 0, EndCol: 4}
	instrs := make([]ir.Instr, 10)
	for i := range instrs {
		instrs[i] = ir.Instr{Op: bytecode.NOP, Loc: loc}
	}
	seq := mkSeq(1, instrs...)
	got := EncodeLineTable(seq)
	// First entry: SHORT_FORM length 8 → header 0x87, payload 0x04.
	// Second entry: SHORT_FORM length 2 → header 0x81, payload 0x04.
	want := []byte{0x87, 0x04, 0x81, 0x04}
	if !bytes.Equal(got, want) {
		t.Fatalf("split-over-8 = %x, want %x", got, want)
	}
}
