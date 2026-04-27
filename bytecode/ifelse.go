package bytecode

// IfBranch describes one condition+body pair in an if/elif/else chain.
// Used by IfElseBytecode.
type IfBranch struct {
	CondIdx byte // index of condition name in co_names
	BodyVal byte // small-int value for LOAD_SMALL_INT in the true body
	VarIdx  byte // index of variable name in co_names
}

// IfElseBytecode returns the instruction stream for an if/elif/else chain
// where each branch body is a single `name = small_int` assignment.
//
// All branch bodies use LOAD_SMALL_INT (values 0-255). noneIdx is the
// co_consts index of None (always 1 for our pattern). If hasElse is false
// the last code unit group is just LOAD_CONST noneIdx / RETURN_VALUE.
//
// Bytecode pattern for N conditions with else:
//
//	RESUME 0
//	[for each branch (if/elif):]
//	  LOAD_NAME condIdx
//	  TO_BOOL 0 + 3 cache words
//	  POP_JUMP_IF_FALSE 5 + 1 cache word
//	  NOT_TAKEN 0
//	  LOAD_SMALL_INT bodyVal
//	  STORE_NAME varIdx
//	  LOAD_CONST noneIdx (None)
//	  RETURN_VALUE 0
//	[else body (if hasElse):]
//	  LOAD_SMALL_INT elseVal
//	  STORE_NAME elseVarIdx
//	  LOAD_CONST noneIdx
//	  RETURN_VALUE 0
//	[no-else implicit return:]
//	  LOAD_CONST noneIdx
//	  RETURN_VALUE 0
//
// Jump offset 5: after the cache word, skip NOT_TAKEN + LOAD_SMALL_INT +
// STORE_NAME + LOAD_CONST + RETURN_VALUE (5 code units) to reach the
// next condition or else body.
func IfElseBytecode(branches []IfBranch, hasElse bool, elseVal, elseVarIdx, noneIdx byte) []byte {
	// Size per branch: RESUME(1) + LOAD_NAME(1) + TO_BOOL(1)+3cache(3) + POP_JUMP(1)+1cache(1) + NOT_TAKEN(1) + LOAD_SMALL_INT(1) + STORE_NAME(1) + LOAD_CONST(1) + RV(1) = 12 words per branch
	// Else body: 4 words (LOAD_SMALL_INT + STORE + LOAD_CONST + RV) or 2 words (LOAD_CONST + RV)
	n := len(branches)
	tailWords := 2
	if hasElse {
		tailWords = 4
	}
	size := 2 + 12*n + 2*tailWords // in bytes (each word = 2 bytes)
	out := make([]byte, 0, size)
	out = append(out, byte(RESUME), 0)
	for _, br := range branches {
		out = append(out, byte(LOAD_NAME), br.CondIdx)
		out = append(out, byte(TO_BOOL), 0, 0, 0, 0, 0, 0, 0) // TO_BOOL + 3 caches
		out = append(out, byte(POP_JUMP_IF_FALSE), 5, 0, 0)    // offset=5, 1 cache
		out = append(out, byte(NOT_TAKEN), 0)
		out = append(out, byte(LOAD_SMALL_INT), br.BodyVal)
		out = append(out, byte(STORE_NAME), br.VarIdx)
		out = append(out, byte(LOAD_CONST), noneIdx)
		out = append(out, byte(RETURN_VALUE), 0)
	}
	if hasElse {
		out = append(out, byte(LOAD_SMALL_INT), elseVal)
		out = append(out, byte(STORE_NAME), elseVarIdx)
	}
	out = append(out, byte(LOAD_CONST), noneIdx)
	out = append(out, byte(RETURN_VALUE), 0)
	return out
}

// IfBranchLT describes one condition+body pair's source positions for
// line-table generation.
type IfBranchLT struct {
	CondLine int
	CondCol  byte
	CondEnd  byte
	BodyLine int
	ValCol   byte // column of the integer literal in the body
	ValEnd   byte // column after the integer literal
	VarCol   byte // column of the variable name in the body
	VarEnd   byte // column after the variable name
}

// IfElseLineTable returns the PEP 626 line table for an if/elif/else chain.
//
// branches: condition+body positions for each if/elif arm.
// hasElse: whether there is an else body.
// elseLine/elseValCol/elseValEnd/elseVarCol/elseVarEnd: positions in the else body.
//
// Line table entries per branch:
//  1. Condition block (8 code units): LOAD_NAME + TO_BOOL+3cache + POP_JUMP+cache + NOT_TAKEN
//  2. LOAD_SMALL_INT (1 code unit): value position in body
//  3. STORE_NAME + LOAD_CONST + RETURN_VALUE (3 code units): variable position in body
//
// For no-else: appends a LONG entry (2 code units) at the first condition's
// column going back to the condition line (implicit "return None" when false).
// For else: appends the else body entries (same structure as a true body).
func IfElseLineTable(branches []IfBranchLT, hasElse bool, elseLine int, elseValCol, elseValEnd, elseVarCol, elseVarEnd byte) []byte {
	out := make([]byte, 0, 5+10*len(branches)+10)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)

	prevLine := 0
	for i, br := range branches {
		// Condition block: 8 code units
		lineDelta := br.CondLine - prevLine
		out = appendFirstLineEntry(out, lineDelta, 8, br.CondCol, br.CondEnd)
		prevLine = br.CondLine

		// True body: LOAD_SMALL_INT (1 cu)
		lineDelta = br.BodyLine - prevLine
		out = appendFirstLineEntry(out, lineDelta, 1, br.ValCol, br.ValEnd)
		prevLine = br.BodyLine

		// True body: STORE_NAME + LOAD_CONST + RETURN_VALUE (3 cu)
		out = appendSameLine(out, 3, br.VarCol, br.VarEnd)

		if !hasElse && i == len(branches)-1 {
			// Implicit return None when condition is false: 2 code units,
			// attributed to the first condition's source position.
			first := branches[0]
			lineDelta = first.CondLine - prevLine
			// Use LONG entry for arbitrary line delta.
			out = append(out, entryHeader(codeLong, 2))
			out = appendSignedVarint(out, lineDelta)
			out = append(out, 0x00) // end_line_delta=0
			out = appendVarint(out, uint(first.CondCol)+1)
			out = appendVarint(out, uint(first.CondEnd)+1)
			return out
		}
	}

	// Else body.
	lineDelta := elseLine - prevLine
	out = appendFirstLineEntry(out, lineDelta, 1, elseValCol, elseValEnd)
	out = appendSameLine(out, 3, elseVarCol, elseVarEnd)
	return out
}
