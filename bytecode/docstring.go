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
// whose first statement is a docstring on `docLine` ending at
// column `docEndCol`, followed by `tail` no-op statements.
//
// The docstring entry covers two code units (LOAD_CONST + STORE_NAME)
// when the tail is non-empty. When the tail is empty it absorbs the
// trailing LOAD_CONST None + RETURN_VALUE too, growing to four code
// units. Tail statements use the v0.0.4 single-statement encoding,
// with the final statement covering the LOAD_CONST None +
// RETURN_VALUE pair.
func DocstringLineTable(docLine int, docEndCol byte, tail []NoOpStmt) []byte {
	if docLine < 1 {
		panic("bytecode.DocstringLineTable: docLine must be >= 1")
	}
	out := make([]byte, 0, 5+4+4*len(tail))
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)
	docLength := 2
	if len(tail) == 0 {
		docLength = 4
	}
	out = appendNoOpEntry(out, docLine, docLength, docEndCol)
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
