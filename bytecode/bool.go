package bytecode

// BoolAndBytecode returns the instruction stream for `x = a and b`
// (module-level boolean-and assignment, both operands are names).
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME 0 (a)
//	COPY 1
//	TO_BOOL 0 + 3 cache words
//	POP_JUMP_IF_FALSE 3 + 1 cache word
//	NOT_TAKEN 0
//	POP_TOP 0
//	LOAD_NAME 1 (b)
//	STORE_NAME 2 (x)
//	LOAD_CONST 0 (None)
//	RETURN_VALUE 0
//
// Jump offset 3: when a is falsy, STORE_NAME x (keeping a on stack from COPY).
// co_names: [a, b, x]  co_consts: [None]  co_stacksize: 2
func BoolAndBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), 0,
		byte(COPY), 1,
		byte(TO_BOOL), 0, 0, 0, 0, 0, 0, 0, // TO_BOOL + 3 cache words
		byte(POP_JUMP_IF_FALSE), 3, 0, 0,   // POP_JUMP_IF_FALSE + 1 cache word
		byte(NOT_TAKEN), 0,
		byte(POP_TOP), 0,
		byte(LOAD_NAME), 1,
		byte(STORE_NAME), 2,
		byte(LOAD_CONST), 0,
		byte(RETURN_VALUE), 0,
	}
}

// BoolOrBytecode returns the instruction stream for `x = a or b`
// (module-level boolean-or assignment, both operands are names).
//
// Identical to BoolAndBytecode except POP_JUMP_IF_TRUE replaces
// POP_JUMP_IF_FALSE: when a is truthy, keep a; when falsy, use b.
//
// co_names: [a, b, x]  co_consts: [None]  co_stacksize: 2
func BoolOrBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), 0,
		byte(COPY), 1,
		byte(TO_BOOL), 0, 0, 0, 0, 0, 0, 0, // TO_BOOL + 3 cache words
		byte(POP_JUMP_IF_TRUE), 3, 0, 0,    // POP_JUMP_IF_TRUE + 1 cache word
		byte(NOT_TAKEN), 0,
		byte(POP_TOP), 0,
		byte(LOAD_NAME), 1,
		byte(STORE_NAME), 2,
		byte(LOAD_CONST), 0,
		byte(RETURN_VALUE), 0,
	}
}

// BoolAndOrLineTable returns the PEP 626 line table for:
//
//	x = a and b  or  x = a or b  (both operands are names)
//
// leftCol/leftLen: column and length of the left name (a).
// rightCol/rightLen: column and length of the right name (b).
// targetLen: length of the target name (x, always at column 0).
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. LOAD_NAME a: 1 code unit
//  3. COPY + TO_BOOL+3cache + POP_JUMP_IF_FALSE+1cache + NOT_TAKEN: 8 code units
//  4. POP_TOP: 1 code unit
//  5. LOAD_NAME b: 1 code unit
//  6. STORE_NAME + LOAD_CONST + RETURN_VALUE: 3 code units
func BoolAndOrLineTable(line int, leftCol, leftLen, rightCol, rightLen, targetLen byte) []byte {
	rightEnd := rightCol + rightLen
	out := make([]byte, 0, 5+3+2+2+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)               // prologue (RESUME)
	out = appendValueEntry(out, line, leftCol, leftCol+leftLen)     // LOAD_NAME a
	out = appendSameLine(out, 8, leftCol, rightEnd)                 // COPY+TO_BOOL+caches+POP_JUMP+cache+NOT_TAKEN
	out = appendSameLine(out, 1, leftCol, rightEnd)                 // POP_TOP
	out = appendSameLine(out, 1, rightCol, rightEnd)                // LOAD_NAME b
	out = appendSameLine(out, 3, 0, targetLen)                      // STORE_NAME+LOAD_CONST+RETURN_VALUE
	return out
}

// TernaryBytecode returns the instruction stream for `x = a if c else b`
// (module-level conditional expression, all operands are names).
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME 0 (c)           -- condition
//	TO_BOOL 0 + 3 cache words
//	POP_JUMP_IF_FALSE 5 + 1 cache word
//	NOT_TAKEN 0
//	LOAD_NAME 1 (a)           -- true branch
//	STORE_NAME 3 (x)
//	LOAD_CONST 0 (None)
//	RETURN_VALUE 0
//	LOAD_NAME 2 (b)           -- false branch (jump target)
//	STORE_NAME 3 (x)
//	LOAD_CONST 0 (None)
//	RETURN_VALUE 0
//
// Jump offset 5: false branch is 5 words after the POP_JUMP_IF_FALSE cache.
// co_names: [c, a, b, x]  co_consts: [None]  co_stacksize: 1
func TernaryBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), 0,
		byte(TO_BOOL), 0, 0, 0, 0, 0, 0, 0, // TO_BOOL + 3 cache words
		byte(POP_JUMP_IF_FALSE), 5, 0, 0,   // POP_JUMP_IF_FALSE + 1 cache word
		byte(NOT_TAKEN), 0,
		byte(LOAD_NAME), 1,
		byte(STORE_NAME), 3,
		byte(LOAD_CONST), 0,
		byte(RETURN_VALUE), 0,
		byte(LOAD_NAME), 2,
		byte(STORE_NAME), 3,
		byte(LOAD_CONST), 0,
		byte(RETURN_VALUE), 0,
	}
}

// TernaryLineTable returns the PEP 626 line table for:
//
//	x = a if c else b  (all operands are names, on the given source line)
//
// condCol/condLen: column and length of the condition name (c).
// trueCol/trueLen: column and length of the true-branch name (a).
// falseCol/falseLen: column and length of the false-branch name (b).
// targetLen: length of the target name (x, always at column 0).
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. LOAD_NAME c + TO_BOOL+3cache + POP_JUMP_IF_FALSE+1cache + NOT_TAKEN: 8 code units
//  3. LOAD_NAME a: 1 code unit
//  4. STORE_NAME + LOAD_CONST + RETURN_VALUE: 3 code units
//  5. LOAD_NAME b: 1 code unit
//  6. STORE_NAME + LOAD_CONST + RETURN_VALUE: 3 code units
func TernaryLineTable(line int, condCol, condLen, trueCol, trueLen, falseCol, falseLen, targetLen byte) []byte {
	out := make([]byte, 0, 5+3+2+2+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)                         // prologue (RESUME)
	out = appendFirstLineEntry(out, line, 8, condCol, condCol+condLen)        // LOAD c+TO_BOOL+caches+POP_JUMP+cache+NOT_TAKEN
	out = appendSameLine(out, 1, trueCol, trueCol+trueLen)                    // LOAD_NAME a
	out = appendSameLine(out, 3, 0, targetLen)                                // STORE_NAME+LOAD_CONST+RETURN_VALUE
	out = appendSameLine(out, 1, falseCol, falseCol+falseLen)                 // LOAD_NAME b
	out = appendSameLine(out, 3, 0, targetLen)                                // STORE_NAME+LOAD_CONST+RETURN_VALUE
	return out
}

// appendFirstLineEntry emits one PEP 626 entry for the first instruction(s)
// on a new source line covering `length` code units. It is like
// appendValueEntry but supports arbitrary lengths (not just 1).
func appendFirstLineEntry(out []byte, lineDelta, length int, startCol, endCol byte) []byte {
	switch lineDelta {
	case 0:
		return append(out, entryHeader(codeOneLine0, length), startCol, endCol)
	case 1:
		return append(out, entryHeader(codeOneLine1, length), startCol, endCol)
	case 2:
		return append(out, entryHeader(codeOneLine2, length), startCol, endCol)
	default:
		out = append(out, entryHeader(codeLong, length))
		out = appendSignedVarint(out, lineDelta)
		out = append(out, 0x00)
		out = appendVarint(out, uint(startCol)+1)
		out = appendVarint(out, uint(endCol)+1)
		return out
	}
}
