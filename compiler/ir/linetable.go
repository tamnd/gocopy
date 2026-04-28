package ir

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
)

// decodeLineTable walks a PEP 626 / PEP 657 location table and
// returns the Loc that applies at each *instruction word* (one
// word = two bytes). The result is indexed by code unit, matching
// CPython's bytecode-vs-line-table mapping.
//
// The wire format (CPython 3.14 Python/locations.md):
//
//	1 KKKK LLL
//
// is the lead byte: K is a 4-bit code, L is length-in-code-units
// minus 1. Codes:
//
//	0..9  — SHORT_FORM:    line_delta=0, one column byte
//	10/11/12 — ONE_LINE_{0,1,2}: line_delta = code-10, two column bytes
//	13   — NO_COLUMNS:     svarint line_delta, no columns
//	14   — LONG:           svarint line_delta, varint end_line_delta,
//	                       varint col, varint end_col
//	15   — NONE:           no source position
//
// The code 0..9 SHORT_FORM also encodes a column hint: code is a
// nibble combining (col >> 3) and (end_col_minus_col); see
// https://github.com/python/cpython/blob/3.14/Python/locations.md
// for the full layout. We mirror CPython's reader exactly so the
// per-instruction Loc round-trips.
func decodeLineTable(table []byte, firstLineNo int32, codeUnits int) ([]bytecode.Loc, error) {
	out := make([]bytecode.Loc, codeUnits)
	if len(table) == 0 {
		return out, nil
	}
	line := firstLineNo
	unitIdx := 0
	p := 0
	for p < len(table) {
		b := table[p]
		if b&0x80 == 0 {
			return nil, errors.New("ir: malformed line table (lead byte missing high bit)")
		}
		code := int((b >> 3) & 0x0F)
		length := int(b&0x07) + 1
		p++

		var loc bytecode.Loc
		switch {
		case code == 15:
			// NONE — no source position. line_delta = 0, all
			// fields zero except they are explicitly "invalid".
			loc = bytecode.Loc{}
		case code == 14:
			dLine, ok := scanSvarint(table, &p)
			if !ok {
				return nil, errors.New("ir: line table truncated in LONG line_delta")
			}
			line += int32(dLine)
			endLineDelta, ok := scanVarint(table, &p)
			if !ok {
				return nil, errors.New("ir: line table truncated in LONG end_line_delta")
			}
			col, ok := scanVarint(table, &p)
			if !ok {
				return nil, errors.New("ir: line table truncated in LONG col")
			}
			endCol, ok := scanVarint(table, &p)
			if !ok {
				return nil, errors.New("ir: line table truncated in LONG end_col")
			}
			loc = bytecode.Loc{
				Line:    uint32(line),
				EndLine: uint32(line + int32(endLineDelta)),
				Col:     uint16(col - 1),
				EndCol:  uint16(endCol - 1),
			}
		case code == 13:
			dLine, ok := scanSvarint(table, &p)
			if !ok {
				return nil, errors.New("ir: line table truncated in NO_COLUMNS")
			}
			line += int32(dLine)
			loc = bytecode.Loc{
				Line:    uint32(line),
				EndLine: uint32(line),
				Col:     bytecode.NoCol,
				EndCol:  bytecode.NoCol,
			}
		case code >= 10 && code <= 12:
			line += int32(code - 10)
			if p+1 >= len(table) {
				return nil, errors.New("ir: line table truncated in ONE_LINE column bytes")
			}
			col := uint16(table[p])
			endCol := uint16(table[p+1])
			p += 2
			loc = bytecode.Loc{
				Line:    uint32(line),
				EndLine: uint32(line),
				Col:     col,
				EndCol:  endCol,
			}
		default: // SHORT_FORM 0..9
			if p >= len(table) {
				return nil, errors.New("ir: line table truncated in SHORT_FORM column byte")
			}
			second := uint16(table[p])
			p++
			col := uint16(code)<<3 | (second >> 4 & 0x07)
			endCol := col + (second & 0x0F)
			loc = bytecode.Loc{
				Line:    uint32(line),
				EndLine: uint32(line),
				Col:     col,
				EndCol:  endCol,
			}
		}

		for u := 0; u < length && unitIdx+u < len(out); u++ {
			out[unitIdx+u] = loc
		}
		unitIdx += length
	}
	return out, nil
}

// scanVarint reads a base-64 varint (continuation bit 0x40,
// payload 0x3F) starting at *p. The return-value tuple is
// (value, ok); ok is false on truncation.
func scanVarint(buf []byte, p *int) (int, bool) {
	if *p >= len(buf) {
		return 0, false
	}
	read := buf[*p]
	val := int(read & 0x3F)
	shift := 0
	for read&0x40 != 0 {
		*p++
		if *p >= len(buf) {
			return val, false
		}
		shift += 6
		read = buf[*p]
		val |= int(read&0x3F) << shift
	}
	*p++
	return val, true
}

// scanSvarint reads a signed varint where the low bit is the sign.
func scanSvarint(buf []byte, p *int) (int, bool) {
	uval, ok := scanVarint(buf, p)
	if !ok {
		return 0, false
	}
	if uval&1 != 0 {
		return -(uval >> 1), true
	}
	return uval >> 1, true
}
