package bytecode

// WhileAssignBytecode returns the instruction stream for a simple
// `while cond: name = val` loop with no break/continue/else.
//
// Bytecode pattern:
//
//	RESUME 0
//	[loop top L1:]
//	  LOAD_NAME condIdx
//	  TO_BOOL 0 + 3 cache words
//	  POP_JUMP_IF_FALSE 5 + 1 cache word
//	  NOT_TAKEN 0
//	  LOAD_SMALL_INT bodyVal
//	  STORE_NAME varIdx
//	  JUMP_BACKWARD 12 + 1 cache word
//	[loop exit L2:]
//	  LOAD_CONST noneIdx (None)
//	  RETURN_VALUE 0
//
// Jump offsets (in code units):
//   - POP_JUMP_IF_FALSE oparg = 5: NOT_TAKEN+LOAD_SMALL_INT+STORE_NAME+JUMP_BACKWARD+cache
//   - JUMP_BACKWARD oparg = 12: L2_cu - L1_cu (back from LOAD_CONST to LOAD_NAME)
func WhileAssignBytecode(condIdx, bodyVal, varIdx, noneIdx byte) []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), condIdx,
		byte(TO_BOOL), 0, 0, 0, 0, 0, 0, 0, // TO_BOOL + 3 cache words
		byte(POP_JUMP_IF_FALSE), 5, 0, 0,    // offset=5, 1 cache
		byte(NOT_TAKEN), 0,
		byte(LOAD_SMALL_INT), bodyVal,
		byte(STORE_NAME), varIdx,
		byte(JUMP_BACKWARD), 12, 0, 0, // offset=12, 1 cache
		byte(LOAD_CONST), noneIdx,
		byte(RETURN_VALUE), 0,
	}
}

// WhileAssignLineTable returns the PEP 626 line table for a simple
// `while cond: name = val` loop on two lines.
//
// Line table entries:
//  1. Condition block (8 code units): LOAD_NAME+TO_BOOL+3cache+POP_JUMP+cache+NOT_TAKEN
//  2. LOAD_SMALL_INT (1 code unit): integer literal in body
//  3. STORE_NAME + JUMP_BACKWARD + cache (3 code units): variable name in body
//  4. LOAD_CONST + RETURN_VALUE (2 code units): LONG entry attributed back to
//     the condition line (loop exits when condition is false).
func WhileAssignLineTable(condLine, bodyLine int, condCol, condEnd, valCol, valEnd, varCol, varEnd byte) []byte {
	out := make([]byte, 0, 20)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)

	out = appendFirstLineEntry(out, condLine, 8, condCol, condEnd)
	out = appendFirstLineEntry(out, bodyLine-condLine, 1, valCol, valEnd)
	out = appendSameLine(out, 3, varCol, varEnd)

	// LOAD_CONST + RETURN_VALUE: attributed back to condition line.
	lineDelta := condLine - bodyLine
	out = append(out, entryHeader(codeLong, 2))
	out = appendSignedVarint(out, lineDelta)
	out = append(out, 0x00) // end_line_delta=0
	out = appendVarint(out, uint(condCol)+1)
	out = appendVarint(out, uint(condEnd)+1)
	return out
}
