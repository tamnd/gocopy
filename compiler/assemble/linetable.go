package assemble

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// EncodeLineTable emits the PEP 626 / 657 location table for an
// InstrSeq, mirroring CPython 3.14
// Python/assemble.c::write_location_info_entry and helpers.
//
// CPython merges runs of consecutive instructions sharing the same
// Loc into a single entry whose length covers all of them (capped at
// 8 code units per entry; runs longer than 8 emit additional entries
// with the same Loc until the run is exhausted). The decoder's
// per-code-unit Loc table treats those runs as a single entry, and
// the round-trip relies on this byte-level merging.
//
// State (`prevLine`) tracks the last lineno written; SHORT_FORM and
// NONE preserve it, every other form updates it.
func EncodeLineTable(seq *ir.InstrSeq) []byte {
	if seq == nil {
		return nil
	}
	var out []byte
	prevLine := seq.FirstLineNo

	var runLoc bytecode.Loc
	runUnits := 0
	flush := func() {
		for runUnits > 8 {
			out = writeEntry(out, runLoc, 8, &prevLine)
			runUnits -= 8
		}
		if runUnits > 0 {
			out = writeEntry(out, runLoc, runUnits, &prevLine)
			runUnits = 0
		}
	}

	for _, b := range seq.Blocks {
		for _, instr := range b.Instrs {
			units := instructionUnits(instr)
			if runUnits == 0 {
				runLoc = instr.Loc
				runUnits = units
				continue
			}
			if instr.Loc == runLoc {
				runUnits += units
				continue
			}
			flush()
			runLoc = instr.Loc
			runUnits = units
		}
	}
	flush()
	return out
}

// writeEntry emits one PEP 626 entry covering `length` (1..8) code
// units. Mirrors CPython's write_location_info_entry exactly.
func writeEntry(out []byte, loc bytecode.Loc, length int, prevLine *int32) []byte {
	if loc.IsZero() {
		// NONE — code 15. No payload. prevLine unchanged. Used for
		// synthetic instructions with no source location (MAKE_CELL,
		// COPY_FREE_VARS in closures).
		return append(out, entryHeader(15, length))
	}
	lineDelta := int32(loc.Line) - *prevLine
	col := loc.Col
	endCol := loc.EndCol
	noCol := col == bytecode.NoCol || endCol == bytecode.NoCol

	if noCol {
		if loc.EndLine == loc.Line || loc.EndLine == 0 {
			out = append(out, entryHeader(13, length))
			out = appendSignedVarint(out, int(lineDelta))
			*prevLine = int32(loc.Line)
			return out
		}
		// Fall through to LONG — col == NoCol but multi-line.
	} else if loc.EndLine == loc.Line {
		// SHORT_FORM: line_delta == 0, col < 80, end_col-col < 16,
		// end_col >= col.
		if lineDelta == 0 && col < 80 && endCol >= col && endCol-col < 16 {
			columnGroup := byte(col >> 3)
			columnLow := byte(col & 7)
			endDelta := byte(endCol - col)
			out = append(out, entryHeader(int(columnGroup), length))
			out = append(out, (columnLow<<4)|endDelta)
			// SHORT_FORM does not update prevLine; line_delta == 0
			// means loc.Line == *prevLine already.
			return out
		}
		// ONE_LINE_{0,1,2}: line_delta in [0,3), col<128, end_col<128.
		if lineDelta >= 0 && lineDelta < 3 && col < 128 && endCol < 128 {
			out = append(out, entryHeader(10+int(lineDelta), length))
			out = append(out, byte(col), byte(endCol))
			*prevLine = int32(loc.Line)
			return out
		}
	}
	// LONG fallback. Encodes:
	//   line_delta (svarint)
	//   end_line - line (varint)
	//   col + 1 (varint; 0 reserved for "no column")
	//   end_col + 1 (varint)
	out = append(out, entryHeader(14, length))
	out = appendSignedVarint(out, int(lineDelta))
	endLineDelta := int32(loc.EndLine) - int32(loc.Line)
	if loc.EndLine == 0 || endLineDelta < 0 {
		endLineDelta = 0
	}
	out = appendVarint(out, uint(endLineDelta))
	colPayload := uint(col) + 1
	endColPayload := uint(endCol) + 1
	if col == bytecode.NoCol {
		colPayload = 0
	}
	if endCol == bytecode.NoCol {
		endColPayload = 0
	}
	out = appendVarint(out, colPayload)
	out = appendVarint(out, endColPayload)
	*prevLine = int32(loc.Line)
	return out
}

func entryHeader(code, length int) byte {
	return 0x80 | byte(code<<3) | byte(length-1)
}

// appendVarint mirrors bytecode/linetable.go's appendVarint.
func appendVarint(b []byte, x uint) []byte {
	for x >= 64 {
		b = append(b, 0x40|byte(x&0x3f))
		x >>= 6
	}
	return append(b, byte(x))
}

// appendSignedVarint mirrors bytecode/linetable.go's
// appendSignedVarint (zigzag with low bit = sign).
func appendSignedVarint(b []byte, x int) []byte {
	var u uint
	if x < 0 {
		u = (uint(-x) << 1) | 1
	} else {
		u = uint(x) << 1
	}
	return appendVarint(b, u)
}

// instructionUnits returns the on-disk code-unit size of one
// instruction, including EXTENDED_ARG chain words and trailing
// inline cache words. Mirrors compiler/flowgraph/cfg.go's
// instructionUnits without exporting that one.
func instructionUnits(instr ir.Instr) int {
	units := 1
	if instr.Arg > 0xFF {
		if instr.Arg > 0xFFFF {
			units++
			if instr.Arg > 0xFFFFFF {
				units++
			}
		}
		units++
	}
	units += int(bytecode.CacheSize[instr.Op])
	return units
}
