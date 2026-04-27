package bytecode

// ForAssignBytecode returns the instruction stream for a simple
// `for loopVar in iter: bodyVar = bodyVal` loop with no break/continue/else.
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME iterIdx
//	GET_ITER
//	[loop top L1:]
//	  FOR_ITER 5 + 1 cache word
//	  STORE_NAME loopVarIdx
//	  LOAD_SMALL_INT bodyVal
//	  STORE_NAME bodyVarIdx
//	  JUMP_BACKWARD 7 + 1 cache word
//	[loop exit L2:]
//	  END_FOR
//	  POP_ITER
//	  LOAD_CONST noneIdx (None)
//	  RETURN_VALUE 0
//
// Jump offsets (in code units):
//   - FOR_ITER oparg = 5: STORE_NAME+LOAD_SMALL_INT+STORE_NAME+JUMP_BACKWARD+cache
//   - JUMP_BACKWARD oparg = 7: L2_cu - L1_cu (from END_FOR back to FOR_ITER)
func ForAssignBytecode(iterIdx, loopVarIdx, bodyVal, bodyVarIdx, noneIdx byte) []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), iterIdx,
		byte(GET_ITER), 0,
		byte(FOR_ITER), 5, 0, 0, // offset=5, 1 cache
		byte(STORE_NAME), loopVarIdx,
		byte(LOAD_SMALL_INT), bodyVal,
		byte(STORE_NAME), bodyVarIdx,
		byte(JUMP_BACKWARD), 7, 0, 0, // offset=7, 1 cache
		byte(END_FOR), 0,
		byte(POP_ITER), 0,
		byte(LOAD_CONST), noneIdx,
		byte(RETURN_VALUE), 0,
	}
}

// ForAssignLineTable returns the PEP 626 line table for a simple
// `for loopVar in iter: bodyVar = bodyVal` loop on two lines.
//
// Line table entries:
//  1. Setup block (4 code units): LOAD_NAME iter+GET_ITER+FOR_ITER+cache
//  2. STORE_NAME loopVar (1 code unit): same line as for
//  3. LOAD_SMALL_INT bodyVal (1 code unit): body line
//  4. STORE_NAME bodyVar + JUMP_BACKWARD + cache (3 code units): same as body
//  5. END_FOR+POP_ITER+LOAD_CONST+RETURN_VALUE (4 code units): LONG entry
//     attributed back to the for line at the iterator's column range.
func ForAssignLineTable(forLine, bodyLine int, iterCol, iterEnd, loopVarCol, loopVarEnd, valCol, valEnd, bodyVarCol, bodyVarEnd byte) []byte {
	out := make([]byte, 0, 25)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)

	out = appendFirstLineEntry(out, forLine, 4, iterCol, iterEnd)
	out = appendSameLine(out, 1, loopVarCol, loopVarEnd)
	out = appendFirstLineEntry(out, bodyLine-forLine, 1, valCol, valEnd)
	out = appendSameLine(out, 3, bodyVarCol, bodyVarEnd)

	// END_FOR + POP_ITER + LOAD_CONST + RETURN_VALUE: attributed back to for line.
	lineDelta := forLine - bodyLine
	out = append(out, entryHeader(codeLong, 4))
	out = appendSignedVarint(out, lineDelta)
	out = append(out, 0x00) // end_line_delta=0
	out = appendVarint(out, uint(iterCol)+1)
	out = appendVarint(out, uint(iterEnd)+1)
	return out
}
