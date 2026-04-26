package bytecode

// PEP 626 line-table emit helpers.
//
// The format is byte-oriented with one entry per code-unit run. Each entry
// starts with a "header" byte:
//
//	1 cccc lll
//	  ^^^^ ^^^
//	  code length-1
//
// where the top bit marks the start of an entry, `cccc` (4 bits) is the
// entry kind, and `lll+1` is the number of code units (one code unit = two
// bytes = one instruction word) the entry covers.
//
// Entry kinds we use here:
//
//	14 (LONG)       payload: 4 svarints = (line_delta, end_line_delta,
//	                                       start_col, end_col)
//	11 (ONE_LINE1)  payload: 2 raw bytes (start_col, end_col); line_delta
//	                                       is implicitly +1 from prev entry
//
// svarint encoding (CPython Objects/locations.md):
//
//	x' := (x << 1) | (x < 0)
//	then little-endian base-128 with continuation bit on every byte
//	except the last.
//
// We hand-roll the small-value cases the v0.0.x rungs need; the general
// encoder lands once multi-statement bodies do.

// LineTableEmpty returns the line table CPython emits for an empty module
// body (no real statements; just the synthetic RESUME / LOAD_CONST None /
// RETURN_VALUE prologue). Bytes verified against
// `python3.14 -m py_compile` on an empty source file.
//
//	f2 03 01 01 01
//	|  |  |  |  |
//	|  |  |  |  end_col svarint = 0
//	|  |  |  start_col svarint = 0
//	|  |  end_line_delta svarint = 0
//	|  line_delta svarint = -1 (synthetic line is firstlineno-1)
//	header: code 14 (LONG), length-1 = 2 (covers 3 code units = 6 bytes)
func LineTableEmpty() []byte {
	return []byte{0xf2, 0x03, 0x01, 0x01, 0x01}
}

// LineTableSingleNoOp returns the line table for a module body of exactly
// one no-op statement (a `pass` or a bare non-string non-bytes constant
// expression statement) on line 1 starting at column 0, with end column
// `endCol`. Verified against `python3.14 -m py_compile` for `pass`,
// `None`, `True`, `False`, `...`, integer / float / complex literals.
//
//	f0 03 01 01 01     LONG entry, 1 code unit (synthetic RESUME)
//	d9 00 endCol       ONE_LINE1 entry, 2 code units (LOAD_CONST + RETURN_VALUE)
//	                   line_delta implicit +1 (puts us on line 1),
//	                   start_col=0, end_col=endCol
//
// Pre: endCol fits in a byte. The caller must enforce that; the spec's
// in-scope statement set has end columns 1..5 so this is comfortably true.
func LineTableSingleNoOp(endCol byte) []byte {
	return []byte{
		0xf0, 0x03, 0x01, 0x01, 0x01,
		0xd9, 0x00, endCol,
	}
}
