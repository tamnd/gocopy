package bytecode

// CmpAssignBytecode returns the instruction stream for a module-level
// assignment of the form `target = left op right` where both operands
// are names and op is a comparison, identity, or containment operator.
//
// op must be one of COMPARE_OP, IS_OP, or CONTAINS_OP.
// oparg is the NB_/CMP_/IS_/CONTAINS_ oparg for the operator.
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME leftIdx   (names[0])
//	LOAD_NAME rightIdx  (names[1])
//	op oparg
//	[cache word if op has cache]
//	STORE_NAME targetIdx (names[2])
//	LOAD_CONST 0 (None)
//	RETURN_VALUE
//
// co_names: [left, right, target]
// co_consts: [None]
// co_stacksize: 2
func CmpAssignBytecode(op Opcode, oparg byte) []byte {
	cacheWords := int(CacheSize[op])
	out := make([]byte, 0, 2+2+2+2+2*cacheWords+2+2+2)
	out = append(out, byte(RESUME), 0)
	out = append(out, byte(LOAD_NAME), 0) // left
	out = append(out, byte(LOAD_NAME), 1) // right
	out = append(out, byte(op), oparg)
	for range cacheWords {
		out = append(out, 0, 0)
	}
	out = append(out, byte(STORE_NAME), 2) // target
	out = append(out, byte(LOAD_CONST), 0)
	out = append(out, byte(RETURN_VALUE), 0)
	return out
}

// CmpAssignLineTable returns the PEP 626 line table for:
//
//	`target = left op right` on the given source line
//
// op must be COMPARE_OP, IS_OP, or CONTAINS_OP.
// leftCol/leftLen: column and byte length of the left-operand name.
// rightCol/rightLen: column and byte length of the right-operand name.
// targetLen: byte length of the target name (at column 0).
func CmpAssignLineTable(op Opcode, line int, leftCol, leftLen, rightCol, rightLen, targetLen byte) []byte {
	cacheWords := int(CacheSize[op])
	opCodeUnits := 1 + cacheWords // 1 instruction + N cache words
	out := make([]byte, 0, 5+3+2+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	out = appendValueEntry(out, line, leftCol, leftCol+leftLen)
	out = appendSameLine(out, 1, rightCol, rightCol+rightLen)
	out = appendSameLine(out, opCodeUnits, leftCol, rightCol+rightLen)
	out = appendSameLine(out, 3, 0, targetLen)
	return out
}
