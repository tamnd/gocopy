package bytecode

// Docstring lowering. The CPython compiler treats a leading single
// string literal as the module's `__doc__` binding:
//
//   RESUME 0
//   LOAD_CONST <docstring>      (const index 0)
//   STORE_NAME __doc__          (name index 0)
//   ... rest of the no-op tail ...
//   LOAD_CONST None             (const index 1)
//   RETURN_VALUE
//
// The const tuple is `(docstring, None)` and the names tuple is
// `('__doc__',)`. The trailing no-op tail of t statements adds
// max(0, t-1) NOPs because the last tail statement absorbs the
// LOAD_CONST None + RETURN_VALUE pair (its line table entry covers
// 2 code units).

// DocstringBytecode returns the instruction stream for a module
// whose body is a leading docstring followed by `tailStmts` no-op
// statements. The result is always the docstring's RESUME +
// LOAD_CONST + STORE_NAME + max(0, tailStmts-1) NOPs +
// LOAD_CONST None + RETURN_VALUE; each instruction is two bytes.
func DocstringBytecode(tailStmts int) []byte {
	if tailStmts < 0 {
		panic("bytecode.DocstringBytecode: tailStmts must be >= 0")
	}
	nops := 0
	if tailStmts > 1 {
		nops = tailStmts - 1
	}
	out := make([]byte, 0, 10+2*nops)
	out = append(out,
		byte(RESUME), 0,
		byte(LOAD_CONST), 0,
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

// DocstringLineTable returns the PEP 626 line table for a module
// whose first statement is a docstring on `docLine` ending on
// `docEndLine` at column `docEndCol`, followed by `tail` no-op
// statements.
//
// The docstring entry covers two code units (LOAD_CONST + STORE_NAME)
// when the tail is non-empty, four when not. Single-line docstrings
// (docEndLine == docLine) use the compact ONE_LINE_* dispatch in
// appendNoOpEntry. Multi-line docstrings always use a LONG entry
// because the ONE_LINE codes implicitly mean end_line == start_line;
// `appendDocstringLong` handles that path.
//
// Tail line deltas are computed from the docstring's *start* line
// (the v0.0.4 entry-to-entry rule), not its end line.
func DocstringLineTable(docLine, docEndLine int, docEndCol byte, tail []NoOpStmt) []byte {
	if docLine < 1 || docEndLine < docLine {
		panic("bytecode.DocstringLineTable: bad docLine/docEndLine")
	}
	out := make([]byte, 0, 5+5+4*len(tail))
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)
	docLength := 2
	if len(tail) == 0 {
		docLength = 4
	}
	if docEndLine == docLine {
		out = appendNoOpEntry(out, docLine, docLength, docEndCol)
	} else {
		out = appendDocstringLong(out, docLine, docEndLine-docLine, docLength, docEndCol)
	}
	prevLine := docLine
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

// appendDocstringLong writes a LONG entry for a multi-line statement
// at column 0, using the docstring's line_delta from the previous
// entry, the given end_line_delta, length, and end_col. start_col is
// implicit 0 for our docstring grammar.
func appendDocstringLong(out []byte, lineDelta, endLineDelta, length int, endCol byte) []byte {
	out = append(out, entryHeader(codeLong, length))
	out = appendSignedVarint(out, lineDelta)
	out = appendVarint(out, uint(endLineDelta))
	out = appendVarint(out, 1)
	out = appendVarint(out, uint(endCol)+1)
	return out
}
