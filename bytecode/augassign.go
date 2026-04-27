package bytecode

// AugAssignBytecode returns the instruction stream for a module-level
// augmented assignment of the form `name = initVal\nname += augVal\n`,
// where both values are non-negative integers.
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD initVal                 (LOAD_SMALL_INT if 0..255, else LOAD_CONST 0)
//	STORE_NAME 0
//	LOAD_NAME 0
//	LOAD augVal                  (LOAD_SMALL_INT if 0..255, else LOAD_CONST augConstIdx)
//	BINARY_OP NbInplaceAdd (+=)
//	<5 cache words>
//	STORE_NAME 0
//	<nops>
//	LOAD_CONST noneIdx
//	RETURN_VALUE
//
// consts layout:
//   - consts[0] = initVal (phantom slot when small int, real slot when large)
//   - consts[1] = augVal  (only present when augVal > 255)
//   - consts[?] = None    (last slot)
func AugAssignBytecode(initVal, augVal int64, tailStmts int) []byte {
	if tailStmts < 0 {
		panic("bytecode.AugAssignBytecode: tailStmts must be >= 0")
	}
	nops := 0
	if tailStmts > 1 {
		nops = tailStmts - 1
	}
	initSmall := initVal >= 0 && initVal <= 255
	augSmall := augVal >= 0 && augVal <= 255
	var noneIdx, augConstIdx byte
	if augSmall {
		noneIdx = 1
	} else {
		noneIdx = 2
		augConstIdx = 1
	}
	out := make([]byte, 0, 2+2+2+2+2+2+10+2+2*nops+4)
	out = append(out, byte(RESUME), 0)
	if initSmall {
		out = append(out, byte(LOAD_SMALL_INT), byte(initVal))
	} else {
		out = append(out, byte(LOAD_CONST), 0)
	}
	out = append(out, byte(STORE_NAME), 0)
	out = append(out, byte(LOAD_NAME), 0)
	if augSmall {
		out = append(out, byte(LOAD_SMALL_INT), byte(augVal))
	} else {
		out = append(out, byte(LOAD_CONST), augConstIdx)
	}
	out = append(out, byte(BINARY_OP), NbInplaceAdd)
	out = append(out, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0) // 5 cache words
	out = append(out, byte(STORE_NAME), 0)
	for range nops {
		out = append(out, byte(NOP), 0)
	}
	out = append(out, byte(LOAD_CONST), noneIdx, byte(RETURN_VALUE), 0)
	return out
}

// AugAssignLineTable returns the PEP 626 line table for:
//
//	`name = initVal` on initLine, value at columns initValStart..initValEnd
//	`name += augVal` on augLine, value at columns augValStart..augValEnd
//
// augValEnd is also the end column of the full augmented-assignment statement
// (no trailing text after the value). nameLen is the byte length of the
// variable name (1..15). Optional tail no-op statements follow.
func AugAssignLineTable(
	initLine int, nameLen, initValStart, initValEnd byte,
	augLine int, augValStart, augValEnd byte,
	tail []NoOpStmt,
) []byte {
	out := make([]byte, 0, 5+3+2+3+2+2+2+4*len(tail))
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	// LOAD initVal: 1 code unit on initLine
	out = appendValueEntry(out, initLine, initValStart, initValEnd)
	// STORE_NAME: 1 unit, same line, cols 0..nameLen
	out = appendShort0Entry(out, 1, 0, nameLen)
	// LOAD_NAME: 1 unit on augLine, cols 0..nameLen
	out = appendValueEntry(out, augLine-initLine, 0, nameLen)
	// LOAD augVal: 1 unit, same line, cols augValStart..augValEnd
	out = appendSameLine(out, 1, augValStart, augValEnd)
	// BINARY_OP + 5 cache words: 6 units, same line, cols 0..augValEnd
	out = appendSameLine(out, 6, 0, augValEnd)
	// STORE_NAME: covers tail if present, else absorbs LOAD_CONST None + RV
	storeLen := 3
	if len(tail) > 0 {
		storeLen = 1
	}
	out = appendSameLine(out, storeLen, 0, nameLen)
	prevLine := augLine
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
