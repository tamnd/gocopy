package bytecode

// BinOpAssignBytecode returns the instruction stream for a module-level
// assignment of the form `target = left op right` where both operands are
// names. oparg is the NB_* enum value for the operator (e.g. NbAdd for +).
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME leftIdx
//	LOAD_NAME rightIdx
//	BINARY_OP oparg
//	<5 cache words (10 zero bytes)>
//	STORE_NAME targetIdx
//	LOAD_CONST 0 (None)
//	RETURN_VALUE
//
// co_names order: [left, right, target] (insertion order during compilation).
// co_consts: [None]
// co_stacksize: 2
func BinOpAssignBytecode(oparg byte) []byte {
	out := make([]byte, 0, 2+2+2+2+10+2+2+2)
	out = append(out, byte(RESUME), 0)
	out = append(out, byte(LOAD_NAME), 0) // left at names[0]
	out = append(out, byte(LOAD_NAME), 1) // right at names[1]
	out = append(out, byte(BINARY_OP), oparg)
	out = append(out, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0) // 5 cache words
	out = append(out, byte(STORE_NAME), 2)            // target at names[2]
	out = append(out, byte(LOAD_CONST), 0)
	out = append(out, byte(RETURN_VALUE), 0)
	return out
}

// BinOpAssignLineTable returns the PEP 626 line table for:
//
//	`target = left op right` on the given source line
//
// leftCol/leftLen: column and byte length of the left-operand name.
// rightCol/rightLen: column and byte length of the right-operand name.
// targetLen: byte length of the target name (at column 0).
func BinOpAssignLineTable(line int, leftCol, leftLen, rightCol, rightLen, targetLen byte) []byte {
	out := make([]byte, 0, 5+3+2+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	// LOAD_NAME left: first real instruction on this line
	out = appendValueEntry(out, line, leftCol, leftCol+leftLen)
	// LOAD_NAME right: same line
	out = appendSameLine(out, 1, rightCol, rightCol+rightLen)
	// BINARY_OP + 5 cache words (6 code units): spans left..rightEnd
	out = appendSameLine(out, 6, leftCol, rightCol+rightLen)
	// STORE_NAME + LOAD_CONST None + RETURN_VALUE (3 code units)
	out = appendSameLine(out, 3, 0, targetLen)
	return out
}

// UnaryNegInvertBytecode returns the instruction stream for:
//
//	target = -operand  (UNARY_NEGATIVE)
//	target = ~operand  (UNARY_INVERT)
//
// opcode must be UNARY_NEGATIVE or UNARY_INVERT.
// co_names order: [operand, target]
// co_consts: [None]
// co_stacksize: 1
func UnaryNegInvertBytecode(opcode Opcode) []byte {
	out := make([]byte, 0, 2+2+2+2+2+2)
	out = append(out, byte(RESUME), 0)
	out = append(out, byte(LOAD_NAME), 0) // operand at names[0]
	out = append(out, byte(opcode), 0)
	out = append(out, byte(STORE_NAME), 1) // target at names[1]
	out = append(out, byte(LOAD_CONST), 0)
	out = append(out, byte(RETURN_VALUE), 0)
	return out
}

// UnaryNegInvertLineTable returns the PEP 626 line table for:
//
//	`target = -operand` or `target = ~operand` on the given source line
//
// opCol: column of the unary operator (- or ~).
// operandCol/operandLen: column and byte length of the operand name.
// targetLen: byte length of the target name (at column 0).
func UnaryNegInvertLineTable(line int, opCol, operandCol, operandLen, targetLen byte) []byte {
	out := make([]byte, 0, 5+3+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	// LOAD_NAME operand
	out = appendValueEntry(out, line, operandCol, operandCol+operandLen)
	// UNARY_NEGATIVE / UNARY_INVERT: spans opCol..operandEnd
	out = appendSameLine(out, 1, opCol, operandCol+operandLen)
	// STORE_NAME + LOAD_CONST None + RETURN_VALUE (3 code units)
	out = appendSameLine(out, 3, 0, targetLen)
	return out
}

// UnaryNotBytecode returns the instruction stream for `target = not operand`.
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME 0 (operand)
//	TO_BOOL
//	<3 cache words (6 zero bytes)>
//	UNARY_NOT
//	STORE_NAME 1 (target)
//	LOAD_CONST 0 (None)
//	RETURN_VALUE
//
// co_names order: [operand, target]
// co_consts: [None]
// co_stacksize: 1
func UnaryNotBytecode() []byte {
	out := make([]byte, 0, 2+2+2+6+2+2+2+2)
	out = append(out, byte(RESUME), 0)
	out = append(out, byte(LOAD_NAME), 0) // operand at names[0]
	out = append(out, byte(TO_BOOL), 0)
	out = append(out, 0, 0, 0, 0, 0, 0) // 3 cache words
	out = append(out, byte(UNARY_NOT), 0)
	out = append(out, byte(STORE_NAME), 1) // target at names[1]
	out = append(out, byte(LOAD_CONST), 0)
	out = append(out, byte(RETURN_VALUE), 0)
	return out
}

// UnaryNotLineTable returns the PEP 626 line table for:
//
//	`target = not operand` on the given source line
//
// notCol: column of 'n' in 'not'.
// operandCol/operandLen: column and byte length of the operand name.
// targetLen: byte length of the target name (at column 0).
func UnaryNotLineTable(line int, notCol, operandCol, operandLen, targetLen byte) []byte {
	out := make([]byte, 0, 5+3+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	// LOAD_NAME operand
	out = appendValueEntry(out, line, operandCol, operandCol+operandLen)
	// TO_BOOL + 3 cache words + UNARY_NOT (5 code units): spans notCol..operandEnd
	out = appendSameLine(out, 5, notCol, operandCol+operandLen)
	// STORE_NAME + LOAD_CONST None + RETURN_VALUE (3 code units)
	out = appendSameLine(out, 3, 0, targetLen)
	return out
}
