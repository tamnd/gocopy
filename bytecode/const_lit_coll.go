package bytecode

// Bytecode and linetable helpers for module-level constant-literal collection
// assignments: `x = ("a", "b", "c")` (tuple) and `x = ["a", "b", "c"]` (list).
//
// CPython emits different sequences based on element count and collection kind:
//
//   tuple (any N≥1):       LOAD_CONST <full-tuple>  + STORE_NAME + LOAD_CONST None + RETURN_VALUE
//   list N=1:              LOAD_CONST elem0 + BUILD_LIST 1 + STORE_NAME + LOAD_CONST None + RETURN_VALUE
//   list N=2:              LOAD_CONST e0 + LOAD_CONST e1 + BUILD_LIST 2 + STORE_NAME + ...
//   list N≥3:              BUILD_LIST 0 + LOAD_CONST <tuple> + LIST_EXTEND 1 + STORE_NAME + ...
//
// co_consts layout:
//   tuple or list N≥3:     (first_elem, None, ConstTuple{all_elems})
//   list N=1:              (elem0, None)
//   list N=2:              (elem0, elem1, None)

// ConstLitTupleBytecode returns the instruction stream for a tuple of N≥1
// constant elements: RESUME + LOAD_CONST 2 (full tuple at index 2) +
// STORE_NAME 0 + LOAD_CONST 1 (None) + RETURN_VALUE.
func ConstLitTupleBytecode() []byte {
	return AssignBytecodeAt(2, 1, 0)
}

// ConstLitList1Bytecode returns the instruction stream for a 1-element
// constant list: RESUME + LOAD_CONST 0 + BUILD_LIST 1 + STORE_NAME 0 +
// LOAD_CONST 1 (None) + RETURN_VALUE.
func ConstLitList1Bytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_CONST), 0,
		byte(BUILD_LIST), 1,
		byte(STORE_NAME), 0,
		byte(LOAD_CONST), 1,
		byte(RETURN_VALUE), 0,
	}
}

// ConstLitList2Bytecode returns the instruction stream for a 2-element
// constant list: RESUME + LOAD_CONST 0 + LOAD_CONST 1 + BUILD_LIST 2 +
// STORE_NAME 0 + LOAD_CONST 2 (None) + RETURN_VALUE.
func ConstLitList2Bytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_CONST), 0,
		byte(LOAD_CONST), 1,
		byte(BUILD_LIST), 2,
		byte(STORE_NAME), 0,
		byte(LOAD_CONST), 2,
		byte(RETURN_VALUE), 0,
	}
}

// ConstLitListExtendBytecode returns the instruction stream for a 3+ element
// constant list: RESUME + BUILD_LIST 0 + LOAD_CONST 2 (full tuple) +
// LIST_EXTEND 1 + STORE_NAME 0 + LOAD_CONST 1 (None) + RETURN_VALUE.
func ConstLitListExtendBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(BUILD_LIST), 0,
		byte(LOAD_CONST), 2,
		byte(LIST_EXTEND), 1,
		byte(STORE_NAME), 0,
		byte(LOAD_CONST), 1,
		byte(RETURN_VALUE), 0,
	}
}

// LargeListElt describes one element of a large (n≥31) all-string-constant list.
type LargeListElt struct {
	Line     int  // 1-indexed source line of the element
	StartCol byte // column of the opening quote
	EndCol   byte // exclusive end column (after closing quote)
}

// ConstLitLargeListBytecode returns the instruction stream for a large (n≥31)
// all-string-constant list: RESUME + BUILD_LIST 0 + N×(LOAD_CONST i + LIST_APPEND 1) +
// STORE_NAME 0 + LOAD_CONST N (None at index N) + RETURN_VALUE.
func ConstLitLargeListBytecode(n int) []byte {
	out := make([]byte, 0, 4+n*4+6)
	out = append(out, byte(RESUME), 0, byte(BUILD_LIST), 0)
	for i := range n {
		out = append(out, byte(LOAD_CONST), byte(i), byte(LIST_APPEND), 1)
	}
	out = append(out, byte(STORE_NAME), 0, byte(LOAD_CONST), byte(n), byte(RETURN_VALUE), 0)
	return out
}

// ConstLitLargeListLineTable returns the PEP 626 line table for a large list
// assignment where elements are each on their own line. openLine/closeLine are
// the 1-indexed lines of '[' and ']'; openCol is the column of '[';
// closeEndCol is the exclusive end column of ']' (1 for ']' at col 0).
func ConstLitLargeListLineTable(openLine, closeLine int, nameLen, openCol, closeEndCol byte, elts []LargeListElt) []byte {
	n := len(elts)
	out := make([]byte, 0, 5+6+n*14+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // RESUME prologue

	endLineDelta := closeLine - openLine

	// BUILD_LIST: LONG, 1 CU, spanning [openLine, closeLine].
	prevLine := 0
	out = appendListSpanEntry(out, 1, openLine-prevLine, endLineDelta, openCol, closeEndCol)
	prevLine = openLine

	for _, elt := range elts {
		// LOAD_CONST element: single-line entry.
		out = appendValueEntry(out, elt.Line-prevLine, elt.StartCol, elt.EndCol)
		prevLine = elt.Line
		// LIST_APPEND: back to openLine, same multi-line span.
		out = appendListSpanEntry(out, 1, openLine-prevLine, endLineDelta, openCol, closeEndCol)
		prevLine = openLine
	}

	// STORE_NAME + LOAD_CONST None + RETURN_VALUE: SHORT0 covering 3 CUs.
	out = appendShort0Entry(out, 3, 0, nameLen)
	return out
}

// appendListSpanEntry appends one LONG linetable entry covering a multi-line
// span (BUILD_LIST or LIST_APPEND) back to the opening bracket's line.
func appendListSpanEntry(out []byte, numCUs, lineDelta, endLineDelta int, openCol, closeEndCol byte) []byte {
	out = append(out, entryHeader(codeLong, numCUs))
	out = appendSignedVarint(out, lineDelta)
	out = appendVarint(out, uint(endLineDelta))
	out = appendVarint(out, uint(openCol)+1)
	out = appendVarint(out, uint(closeEndCol)+1)
	return out
}

// ConstLitTupleLineTable returns the PEP 626 line table for a constant-literal
// tuple assignment on `line`. openCol is the column of '(' and closeEnd is
// the lineEndCol (exclusive). This is identical to AssignLineTable(line, nameLen,
// openCol, closeEnd, nil) since LOAD_CONST <tuple> is one code unit.
func ConstLitTupleLineTable(line int, nameLen, openCol, closeEnd byte) []byte {
	return AssignLineTable(line, nameLen, openCol, closeEnd, nil)
}

// ConstLitListExtendLineTable returns the PEP 626 line table for a 3+ element
// constant-literal list assignment. BUILD_LIST 0 + LOAD_CONST + LIST_EXTEND
// together cover 3 code units and get one linetable entry spanning
// [openCol, closeEnd). STORE_NAME + LOAD_CONST None + RETURN_VALUE are 3
// more code units, covered by a SHORT0 entry at column [0, nameLen).
func ConstLitListExtendLineTable(line int, nameLen, openCol, closeEnd byte) []byte {
	out := make([]byte, 0, 5+3+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // RESUME prologue
	// 3-CU build group: ONE_LINE1 (lineDelta=1), 3 CUs, cols [openCol, closeEnd).
	out = append(out, entryHeader(codeOneLine1, 3), openCol, closeEnd)
	// 3-CU store+return group.
	out = appendShort0Entry(out, 3, 0, nameLen)
	return out
}

// ConstLitList1LineTable returns the PEP 626 line table for a 1-element
// constant-literal list assignment. elemCol/elemEnd are the column range of
// the single element literal (including quotes). openCol/closeEnd span the
// entire [...]  expression.
func ConstLitList1LineTable(line int, nameLen, openCol, closeEnd, elemCol, elemEnd byte) []byte {
	out := make([]byte, 0, 5+3+3+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // RESUME prologue
	// LOAD_CONST elem: ONE_LINE1, 1 CU.
	out = appendValueEntry(out, line, elemCol, elemEnd)
	// BUILD_LIST 1: same line, 1 CU, cols [openCol, closeEnd).
	out = appendSameLine(out, 1, openCol, closeEnd)
	// STORE_NAME + LOAD_CONST None + RETURN_VALUE: 3 CUs.
	out = appendShort0Entry(out, 3, 0, nameLen)
	return out
}

// ConstLitSeqStmt describes one constLitColl statement in a multi-statement
// sequence for linetable generation.
type ConstLitSeqStmt struct {
	Line      int          // 1-indexed source line of '[', '(', or value
	CloseLine int          // 1-indexed source line of ']' (large lists only)
	TargetLen byte         // byte length of the assignment target name
	OpenCol   byte         // column of '[' or '('
	CloseEnd  byte         // exclusive end column of ']' or ')'
	IsList    bool         // true for list, false for tuple
	N         int          // number of elements
	Elts      []LargeListElt // non-nil only for n≥31 lists
}

// FrozenSetSeqStmt describes one frozenset(arg).__contains__ statement in a
// multi-statement sequence for linetable generation.
type FrozenSetSeqStmt struct {
	Line         int  // 1-indexed source line
	TargetLen    byte // byte length of the assignment target name
	FrozensetCol byte // column of 'f' in 'frozenset'
	ArgCol       byte // column of first char of the arg name
	ArgLen       byte // byte length of the arg name
}

// ConstLitSeqLineTable returns the PEP 626 line table for a multi-statement
// module body consisting of an optional docstring, zero or more
// constant-literal collection assignments, and zero or more
// frozenset(arg).__contains__ assignments. hasDoc, docLine, docEndLine, and
// docEndCol describe the docstring (ignored when hasDoc is false). Multi-line
// docstrings (docEndLine > docLine) emit a LONG entry automatically.
func ConstLitSeqLineTable(
	hasDoc bool, docLine, docEndLine int, docEndCol byte,
	stmts []ConstLitSeqStmt,
	frozenSetStmts []FrozenSetSeqStmt,
) []byte {
	out := make([]byte, 0, 256)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // RESUME prologue
	prevLine := 0

	if hasDoc {
		// 2 CUs: LOAD_CONST docstring + STORE_NAME __doc__.
		if docEndLine == docLine {
			out = appendNoOpEntry(out, docLine-prevLine, 2, docEndCol)
		} else {
			out = appendDocstringLong(out, docLine-prevLine, docEndLine-docLine, 2, docEndCol)
		}
		prevLine = docLine
	}

	// Determine storeLen for each CLC stmt: last CLC uses 3 only if no frozenset stmts follow.
	for si, s := range stmts {
		storeLen := 1
		if si == len(stmts)-1 && len(frozenSetStmts) == 0 {
			storeLen = 3
		}

		if s.IsList && s.N >= 31 {
			endLineDelta := s.CloseLine - s.Line
			// BUILD_LIST: LONG 1 CU spanning full list.
			out = appendListSpanEntry(out, 1, s.Line-prevLine, endLineDelta, s.OpenCol, s.CloseEnd)
			prevLine = s.Line
			for _, elt := range s.Elts {
				out = appendValueEntry(out, elt.Line-prevLine, elt.StartCol, elt.EndCol)
				prevLine = elt.Line
				out = appendListSpanEntry(out, 1, s.Line-prevLine, endLineDelta, s.OpenCol, s.CloseEnd)
				prevLine = s.Line
			}
		} else if s.IsList {
			if s.CloseLine > s.Line {
				// Multi-line small list: LONG entry spanning [openLine, closeLine).
				out = appendListSpanEntry(out, 3, s.Line-prevLine, s.CloseLine-s.Line, s.OpenCol, s.CloseEnd)
			} else {
				// 3..30 elements on same line: BUILD_LIST + LOAD_CONST + LIST_EXTEND = 3 CUs.
				out = appendValueEntryN(out, 3, s.Line-prevLine, s.OpenCol, s.CloseEnd)
			}
			prevLine = s.Line
		} else {
			// Tuple: LOAD_CONST tuple = 1 CU at [openCol, closeEnd).
			out = appendValueEntry(out, s.Line-prevLine, s.OpenCol, s.CloseEnd)
			prevLine = s.Line
		}

		out = appendShort0Entry(out, storeLen, 0, s.TargetLen)
	}

	// Frozenset stmts: each emits 2+1+4+8+2 CUs for the expression, then
	// 1 CU STORE_NAME (non-last) or 3 CUs (last, absorbs LOAD_CONST None + RETURN_VALUE).
	for fi, fs := range frozenSetStmts {
		frozensetEnd := fs.FrozensetCol + 9  // len("frozenset") = 9
		argEnd := fs.ArgCol + fs.ArgLen
		callEnd := argEnd + 1                // end of ')'
		attrEnd := callEnd + 13              // len(".__contains__") = 13

		// LOAD_NAME frozenset + PUSH_NULL: 2 CUs.
		out = appendValueEntryN(out, 2, fs.Line-prevLine, fs.FrozensetCol, frozensetEnd)
		prevLine = fs.Line
		// LOAD_NAME arg: 1 CU.
		out = appendSameLine(out, 1, fs.ArgCol, argEnd)
		// CALL + 3 cache words: 4 CUs.
		out = appendSameLine(out, 4, fs.FrozensetCol, callEnd)
		// LOAD_ATTR + 9 cache words: 10 CUs split 8+2.
		out = appendSameLine(out, 8, fs.FrozensetCol, attrEnd)
		out = appendSameLine(out, 2, fs.FrozensetCol, attrEnd)
		// STORE_NAME: 1 CU (non-last) or 3 CUs (last).
		storeLen := 1
		if fi == len(frozenSetStmts)-1 {
			storeLen = 3
		}
		out = appendShort0Entry(out, storeLen, 0, fs.TargetLen)
	}

	return out
}

// ConstLitList2LineTable returns the PEP 626 line table for a 2-element
// constant-literal list assignment. col0/end0 and col1/end1 are the column
// ranges of the two element literals (including quotes). openCol/closeEnd
// span the entire [...] expression.
func ConstLitList2LineTable(line int, nameLen, openCol, closeEnd, col0, end0, col1, end1 byte) []byte {
	out := make([]byte, 0, 5+3+2+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // RESUME prologue
	// LOAD_CONST elem0: ONE_LINE1, 1 CU.
	out = appendValueEntry(out, line, col0, end0)
	// LOAD_CONST elem1: same line, 1 CU.
	out = appendSameLine(out, 1, col1, end1)
	// BUILD_LIST 2: same line, 1 CU.
	out = appendSameLine(out, 1, openCol, closeEnd)
	// STORE_NAME + LOAD_CONST None + RETURN_VALUE: 3 CUs.
	out = appendShort0Entry(out, 3, 0, nameLen)
	return out
}
