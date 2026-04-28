package bytecode

// Loc is a source-code location range. It mirrors the 4-tuple shape
// CPython exposes via PyCode_Addr2Location and code.co_positions():
// (start_line, end_line, start_col, end_col).
//
// Lines are 1-based when valid; line 0 means "no source position".
// Columns are 0-based byte offsets, or 0xFFFF for "no column".
//
// Line numbers use uint32 to match CPython's PyCodeObject layout in
// 3.14, where line numbers are stored as 32-bit ints. Columns use
// uint16 so the struct stays 16 bytes; CPython's line-table format
// caps the encodable column at 2047 anyway.
type Loc struct {
	Line    uint32
	EndLine uint32
	Col     uint16
	EndCol  uint16
}

// NoCol is the sentinel column meaning "unknown column". Matches
// CPython's behaviour when source-position tracking has lost the
// column for a particular instruction.
const NoCol uint16 = 0xFFFF

// Valid reports whether the location has a real source line.
// CPython treats line 0 as "no source information".
func (l Loc) Valid() bool {
	return l.Line != 0
}

// IsZero reports whether all fields are zero.
func (l Loc) IsZero() bool {
	return l.Line == 0 && l.EndLine == 0 && l.Col == 0 && l.EndCol == 0
}

// OneLine reports whether the start and end line are the same and
// the location is valid.
func (l Loc) OneLine() bool {
	return l.Valid() && l.Line == l.EndLine
}
