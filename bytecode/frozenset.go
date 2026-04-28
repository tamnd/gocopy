package bytecode

// Bytecode and linetable helpers for `target = frozenset(arg).__contains__`
// module-level assignments.
//
// CPython emits:
//   RESUME 0
//   LOAD_NAME 0      (frozenset)
//   PUSH_NULL
//   LOAD_NAME 1      (arg)
//   CALL 1           + 3 inline-cache words (6 bytes)
//   LOAD_ATTR 4      (__contains__, non-method: name_idx=2 × 2 = 4)
//                    + 9 inline-cache words (18 bytes)
//   STORE_NAME 3     (target)
//   LOAD_CONST 0     (None)
//   RETURN_VALUE
//
// co_consts: (None,)
// co_names:  ('frozenset', arg, '__contains__', target)
// stacksize: 3

// FrozenSetContainsBytecode returns the fixed 42-byte instruction stream for
// `target = frozenset(arg).__contains__`.
func FrozenSetContainsBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), 0,                                                   // frozenset
		byte(PUSH_NULL), 0,
		byte(LOAD_NAME), 1,                                                   // arg
		byte(CALL), 1,                                                        // CALL(1 positional arg)
		0, 0, 0, 0, 0, 0,                                                     // 3 inline-cache words
		byte(LOAD_ATTR), 4,                                                   // __contains__ (name_idx=2, ×2=4)
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,               // 9 inline-cache words
		byte(STORE_NAME), 3,                                                  // target
		byte(LOAD_CONST), 0,                                                  // None
		byte(RETURN_VALUE), 0,
	}
}

// FrozenSetContainsLineTable returns the PEP 626 line table for
// `target = frozenset(arg).__contains__` on source line `line`.
//
// Column arguments (all 0-indexed, exclusive end):
//   targetLen    – byte length of the assignment target name
//   frozensetCol – column of the 'f' in 'frozenset'
//   argCol       – column of the first character of arg
//   argLen       – byte length of arg
func FrozenSetContainsLineTable(line int, targetLen, frozensetCol, argCol, argLen byte) []byte {
	frozensetEnd := frozensetCol + 9       // len("frozenset") = 9
	argEnd := argCol + argLen
	callEnd := argEnd + 1                  // exclusive end of ')'
	attrEnd := callEnd + 13               // len(".__contains__") = 13

	out := make([]byte, 0, 5+3+2+3+3+3+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // RESUME prologue

	// LOAD_NAME frozenset + PUSH_NULL: 2 CUs at [frozensetCol, frozensetEnd).
	out = appendValueEntryN(out, 2, line, frozensetCol, frozensetEnd)

	// LOAD_NAME arg: 1 CU at [argCol, argEnd).
	out = appendSameLine(out, 1, argCol, argEnd)

	// CALL + 3 inline-cache words: 4 CUs at [frozensetCol, callEnd).
	out = appendSameLine(out, 4, frozensetCol, callEnd)

	// LOAD_ATTR + 9 inline-cache words: 10 CUs; split as 8+2.
	out = appendSameLine(out, 8, frozensetCol, attrEnd)
	out = appendSameLine(out, 2, frozensetCol, attrEnd)

	// STORE_NAME + LOAD_CONST None + RETURN_VALUE: 3 CUs at [0, targetLen).
	out = appendShort0Entry(out, 3, 0, targetLen)

	return out
}
