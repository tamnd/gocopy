package bytecode

// PEP 626 line-table emit helpers.
//
// The format is byte-oriented with one entry per code-unit run. Each
// entry starts with a header byte:
//
//	1 cccc lll
//	  ^^^^ ^^^
//	  code length-1
//
// where the top bit marks the start of an entry, `cccc` (4 bits) is
// the entry kind, and `lll+1` is the number of code units (one code
// unit = two bytes = one instruction word) the entry covers.
//
// Entry kinds we emit:
//
//	10 (ONE_LINE0)  payload: 2 raw bytes (start_col, end_col); line
//	                delta is implicitly +0 from previous entry.
//	11 (ONE_LINE1)  payload: 2 raw bytes; line delta implicitly +1.
//	12 (ONE_LINE2)  payload: 2 raw bytes; line delta implicitly +2.
//	14 (LONG)       payload: 4 varints = (line_delta as svarint,
//	                end_line_delta as svarint, start_col+1 as varint,
//	                end_col+1 as varint). Cols carry +1 because 0
//	                means "no column information".
//
// varint / svarint are CPython's own (Objects/locations.md):
//
//	varint:  while x >= 64: emit 0x40|(x & 0x3f); x >>= 6
//	         emit x
//	svarint: if x < 0: x = ((-x)<<1)|1 else x <<= 1; varint(x)

// NoOpStmt describes one no-op statement's source position. Line is
// 1-indexed; EndCol is the 0-indexed exclusive end column (i.e. the
// length of the statement text on its line).
type NoOpStmt struct {
	Line   int
	EndCol byte
}

// LineTableEmpty returns the line table CPython emits for an empty
// module body (no real statements; the synthetic RESUME / LOAD_CONST
// None / RETURN_VALUE prologue covers all three instructions in a
// single LONG entry). Verified against `python3.14 -m py_compile` on
// an empty source file.
//
//	f2 03 01 01 01
//	|  |  |  |  |
//	|  |  |  |  end_col+1 = 1 (col 0)
//	|  |  |  start_col+1 = 1 (col 0)
//	|  |  end_line_delta = 0
//	|  line_delta = -1 (synthetic line is firstlineno-1)
//	header: code 14 (LONG), length-1 = 2 (covers 3 code units)
func LineTableEmpty() []byte {
	return []byte{0xf2, 0x03, 0x01, 0x01, 0x01}
}

// LineTableNoOps returns the line table for a module body of N >= 1
// no-op statements at the given source positions. Bytecode for the
// body is `RESUME, (N-1) NOPs, LOAD_CONST 0, RETURN_VALUE`. Each
// statement contributes one entry: length 1 for non-last (covers one
// NOP) or length 2 for the last (covers LOAD_CONST + RETURN_VALUE).
//
// Statements must be in source order (Line strictly increasing for
// the v0.0.4 grammar; equal lines would also encode but we never emit
// them today). Verified against `python3.14 -m py_compile` for every
// gap configuration up through ten blank lines between statements.
func LineTableNoOps(stmts []NoOpStmt) []byte {
	if len(stmts) < 1 {
		panic("bytecode.LineTableNoOps: need at least one statement")
	}
	out := make([]byte, 0, 5+4*len(stmts))
	// Synthetic prologue. Same payload as LineTableEmpty but length=1
	// because the only synthetic instruction is the RESUME; LOAD_CONST
	// and RETURN_VALUE belong to the last real statement.
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)
	prevLine := 0 // synthetic line; firstlineno-1 in CPython terms
	for i, s := range stmts {
		length := 1
		if i == len(stmts)-1 {
			length = 2
		}
		out = appendNoOpEntry(out, s.Line-prevLine, length, s.EndCol)
		prevLine = s.Line
	}
	return out
}

// LineTableSingleNoOp is the N=1 case of LineTableNoOps with the
// statement on line 1. Kept as a named alias for readability.
func LineTableSingleNoOp(endCol byte) []byte {
	return LineTableNoOps([]NoOpStmt{{Line: 1, EndCol: endCol}})
}

// NoOpBytecode returns the raw instruction stream for a module body
// of exactly N >= 1 no-op statements:
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

// appendNoOpEntry writes one PEP 626 entry covering `length` code
// units (1 or 2) for a statement reached via the given line delta
// from the previous entry's line, ending at column endCol (start col
// is implicitly 0 for our no-op grammar).
func appendNoOpEntry(out []byte, lineDelta, length int, endCol byte) []byte {
	switch lineDelta {
	case 0:
		out = append(out, entryHeader(codeOneLine0, length), 0x00, endCol)
	case 1:
		out = append(out, entryHeader(codeOneLine1, length), 0x00, endCol)
	case 2:
		out = append(out, entryHeader(codeOneLine2, length), 0x00, endCol)
	default:
		out = append(out, entryHeader(codeLong, length))
		out = appendSignedVarint(out, lineDelta)
		out = append(out, 0x00) // end_line_delta = 0
		out = appendVarint(out, 1)
		out = appendVarint(out, uint(endCol)+1)
	}
	return out
}

const (
	codeOneLine0 = 10
	codeOneLine1 = 11
	codeOneLine2 = 12
	codeLong     = 14
)

func entryHeader(code, length int) byte {
	return 0x80 | byte(code<<3) | byte(length-1)
}

// appendVarint writes x in CPython's base-64 little-endian form.
func appendVarint(b []byte, x uint) []byte {
	for x >= 64 {
		b = append(b, 0x40|byte(x&0x3f))
		x >>= 6
	}
	return append(b, byte(x))
}

// appendSignedVarint writes a signed integer using CPython's zigzag
// (low bit = sign) encoding, then a varint.
func appendSignedVarint(b []byte, x int) []byte {
	var u uint
	if x < 0 {
		u = (uint(-x) << 1) | 1
	} else {
		u = uint(x) << 1
	}
	return appendVarint(b, u)
}
