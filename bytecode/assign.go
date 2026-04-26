package bytecode

// Assignment lowering. CPython compiles a top-level `name = literal`
// assignment to:
//
//   RESUME 0
//   LOAD_CONST <value>          (const index 0)
//   STORE_NAME <name>           (name index 0)
//   ... rest of the no-op tail ...
//   LOAD_CONST <None>           (const index 0 if value is None, else 1)
//   RETURN_VALUE
//
// `consts` is `(value,)` when value is None and `(value, None)` otherwise
// (the value gets a const slot even though LOAD_CONST None reuses index 0
// in the None case). `names` is `(name,)`.
//
// The trailing tail of t no-op statements adds `max(0, t-1)` NOPs because
// the last tail statement absorbs the LOAD_CONST None + RETURN_VALUE pair
// (same rule as the docstring lowering in v0.0.5).

// AssignBytecode returns the instruction stream for `name = value`
// followed by `tailStmts` no-op statements. `noneIdx` is the const
// index for the implicit `LOAD_CONST None` at the end: 0 when the
// assigned value itself is None (consts collapses to `(None,)`), 1
// otherwise. The value is always at const index 0.
func AssignBytecode(noneIdx byte, tailStmts int) []byte {
	return AssignBytecodeAt(0, noneIdx, tailStmts)
}

// AssignBytecodeAt is the general form of AssignBytecode. valueIdx is
// the const index for the value (0 in the common case; 2 when CPython's
// constant folder places the folded result after the original literal
// and None, as in `x = -1` where consts = (1, None, -1)).
func AssignBytecodeAt(valueIdx, noneIdx byte, tailStmts int) []byte {
	if tailStmts < 0 {
		panic("bytecode.AssignBytecodeAt: tailStmts must be >= 0")
	}
	nops := 0
	if tailStmts > 1 {
		nops = tailStmts - 1
	}
	out := make([]byte, 0, 10+2*nops)
	out = append(out,
		byte(RESUME), 0,
		byte(LOAD_CONST), valueIdx,
		byte(STORE_NAME), 0,
	)
	for range nops {
		out = append(out, byte(NOP), 0)
	}
	out = append(out,
		byte(LOAD_CONST), noneIdx,
		byte(RETURN_VALUE), 0,
	)
	return out
}

// AssignSmallIntBytecode returns the instruction stream for `name = <int>`
// where the integer value fits in 0..255. CPython uses LOAD_SMALL_INT with
// the value embedded in the oparg instead of LOAD_CONST; None is always at
// const index 1 (the consts tuple is `(int_val, None)`).
func AssignSmallIntBytecode(val byte, tailStmts int) []byte {
	if tailStmts < 0 {
		panic("bytecode.AssignSmallIntBytecode: tailStmts must be >= 0")
	}
	nops := 0
	if tailStmts > 1 {
		nops = tailStmts - 1
	}
	out := make([]byte, 0, 10+2*nops)
	out = append(out,
		byte(RESUME), 0,
		byte(LOAD_SMALL_INT), val,
		byte(STORE_NAME), 0,
	)
	for range nops {
		out = append(out, byte(NOP), 0)
	}
	out = append(out,
		byte(LOAD_CONST), 1,
		byte(RETURN_VALUE), 0,
	)
	return out
}

// AssignLineTable returns the PEP 626 line table for a single
// `name = value` assignment on `line`, where the value occupies
// columns `valStartCol`..`valEndCol` and the name occupies columns
// 0..`nameLen` (so `nameLen` is the number of bytes in the name).
// `tail` is the trailing no-op statements after the assignment.
//
// Layout:
//
//	prologue: LONG length 1 covering RESUME
//	LOAD_CONST entry: ONE_LINE1 length 1, cols (valStartCol, valEndCol)
//	STORE_NAME entry: SHORT0 length (3 if no tail else 1), cols (0, nameLen)
//	tail: same v0.0.4 rule as no-op bodies, prev_line starts at `line`
func AssignLineTable(line int, nameLen, valStartCol, valEndCol byte, tail []NoOpStmt) []byte {
	if line < 1 {
		panic("bytecode.AssignLineTable: line must be >= 1")
	}
	if nameLen == 0 || nameLen > 15 {
		panic("bytecode.AssignLineTable: nameLen out of SHORT0 range (1..15)")
	}
	out := make([]byte, 0, 5+3+2+4*len(tail))
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)
	out = appendValueEntry(out, line, valStartCol, valEndCol)
	storeLen := 3
	if len(tail) > 0 {
		storeLen = 1
	}
	out = appendShort0Entry(out, storeLen, 0, nameLen)
	prevLine := line
	for i, s := range tail {
		length := 1
		if i == len(tail)-1 {
			length = 2
		}
		out = appendNoOpEntry(out, s.Line-prevLine, length, s.EndCol)
		prevLine = s.Line
	}
	return out
}

// appendValueEntry writes one PEP 626 entry covering exactly one code
// unit at line `prevLine + lineDelta`, with explicit start and end
// columns. Used for the LOAD_CONST <value> entry of an assignment,
// where the value occupies its own column range (unlike the v0.0.4
// no-op grammar where every statement starts at column 0).
func appendValueEntry(out []byte, lineDelta int, startCol, endCol byte) []byte {
	switch lineDelta {
	case 0:
		out = append(out, entryHeader(codeOneLine0, 1), startCol, endCol)
	case 1:
		out = append(out, entryHeader(codeOneLine1, 1), startCol, endCol)
	case 2:
		out = append(out, entryHeader(codeOneLine2, 1), startCol, endCol)
	default:
		out = append(out, entryHeader(codeLong, 1))
		out = appendSignedVarint(out, lineDelta)
		out = append(out, 0x00) // end_line_delta = 0
		out = appendVarint(out, uint(startCol)+1)
		out = appendVarint(out, uint(endCol)+1)
	}
	return out
}

// appendShort0Entry writes one PEP 626 SHORT0 entry covering `length`
// code units (1..8). SHORT0 carries an implicit line_delta of 0 and a
// single-byte payload encoding (start_col, end_col) where the entry's
// code value (0 for SHORT0) contributes the high three bits of
// start_col. The grammar we emit always has start_col=0, which fits
// the SHORT0 slot (codes 0..9 cover start_col 0..79 in 8-col steps).
func appendShort0Entry(out []byte, length int, startCol, endCol byte) []byte {
	if startCol >= 8 {
		panic("bytecode: SHORT0 start_col must be < 8 (use SHORT1..9 for higher columns)")
	}
	if endCol < startCol || endCol-startCol > 15 {
		panic("bytecode: SHORT0 end_col offset out of range (0..15)")
	}
	out = append(out, entryHeader(codeShort0, length))
	payload := byte((startCol&0x07)<<4) | (endCol - startCol)
	return append(out, payload)
}

const codeShort0 = 0

// AssignInfo describes one `name = value` assignment for multi-assign
// line-table generation.
type AssignInfo struct {
	Line     int    // 1-indexed source line
	NameLen  byte   // number of bytes in the name (1..15)
	ValStart byte   // 0-indexed start column of the value text
	ValEnd   byte   // 0-indexed exclusive end column of the value text
}

// MultiAssignLineTable returns the PEP 626 line table for a sequence of
// N >= 1 `name = value` assignments followed by optional tail no-ops.
// It generalises AssignLineTable to the multi-assignment case.
func MultiAssignLineTable(asgns []AssignInfo, tail []NoOpStmt) []byte {
	out := make([]byte, 0, 5+5*len(asgns)+4*len(tail))
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	prevLine := 0
	for i, a := range asgns {
		out = appendValueEntry(out, a.Line-prevLine, a.ValStart, a.ValEnd)
		prevLine = a.Line
		storeLen := 1
		if i == len(asgns)-1 && len(tail) == 0 {
			storeLen = 3
		}
		out = appendShort0Entry(out, storeLen, 0, a.NameLen)
	}
	for i, s := range tail {
		length := 1
		if i == len(tail)-1 {
			length = 2
		}
		out = appendNoOpEntry(out, s.Line-prevLine, length, s.EndCol)
		prevLine = s.Line
	}
	return out
}
