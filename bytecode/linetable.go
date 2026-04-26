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
//	11 (ONE_LINE1)  payload: 2 raw bytes (start_col, end_col); line_delta
//	                                       is implicitly +1 from prev entry
//	14 (LONG)       payload: 4 svarints = (line_delta, end_line_delta,
//	                                       start_col, end_col)
//
// svarint encoding (CPython Objects/locations.md):
//
//	x' := (x << 1) | (x < 0)
//	then little-endian base-128 with continuation bit on every byte
//	except the last.
//
// We hand-roll the small-value cases the v0.0.x rungs need; the general
// encoder lands once multi-statement bodies start using non-`+1` line
// deltas (blank lines or comment lines between statements).

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

// LineTableNoOps returns the line table for a module body of exactly N >= 1
// no-op statements on consecutive lines (line 1 .. line N), each starting
// at column 0, with end columns supplied in endCols.
//
// Layout:
//
//	f0 03 01 01 01            LONG entry, 1 code unit (synthetic RESUME)
//	(N-1) x: d8 00 endCols[i] ONE_LINE1 entry, 1 code unit (one NOP),
//	                           line delta +1, cols 0..endCols[i]
//	d9 00 endCols[N-1]        ONE_LINE1 entry, 2 code units (LOAD_CONST +
//	                           RETURN_VALUE), line delta +1, cols 0..endCol
//
// Pre: len(endCols) >= 1; every endCols[i] is the end column of statement
// i+1 (1-based) and fits in a byte. Verified against
// `python3.14 -m py_compile` for one through five consecutive
// `pass` / `None` / `True` / `False` / `...` / numeric literals.
func LineTableNoOps(endCols []byte) []byte {
	if len(endCols) < 1 {
		panic("bytecode.LineTableNoOps: need at least one end column")
	}
	out := make([]byte, 0, 5+3*len(endCols))
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)
	for i := range len(endCols) - 1 {
		out = append(out, 0xd8, 0x00, endCols[i])
	}
	out = append(out, 0xd9, 0x00, endCols[len(endCols)-1])
	return out
}

// LineTableSingleNoOp is the N=1 case of LineTableNoOps. Kept as a named
// alias because v0.0.2's spec calls it out and it makes the call site
// at the single-statement code path read clearly.
func LineTableSingleNoOp(endCol byte) []byte {
	return LineTableNoOps([]byte{endCol})
}

// NoOpBytecode returns the raw instruction stream for a module body of
// exactly N >= 1 no-op statements:
//
//	RESUME 0  +  (N-1) x NOP 0  +  LOAD_CONST 0  +  RETURN_VALUE 0
//
// Each instruction is two bytes (opcode + oparg), so the result is
// 6 + 2*(N-1) bytes long. Verified against `python3.14 -m py_compile`.
func NoOpBytecode(n int) []byte {
	if n < 1 {
		panic("bytecode.NoOpBytecode: need at least one statement")
	}
	out := make([]byte, 0, 6+2*(n-1))
	out = append(out, byte(RESUME), 0)
	for range n - 1 {
		out = append(out, byte(NOP), 0)
	}
	out = append(out, byte(LOAD_CONST), 0, byte(RETURN_VALUE), 0)
	return out
}
