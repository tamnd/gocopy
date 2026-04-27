package bytecode

// FuncDefModuleBytecode returns the module-level instruction stream for
// a `def funcName(...): ...` where funcNameIdx is the index of the function
// name in co_names, the function code object is co_consts[0], and None is
// co_consts[1].
//
// Pattern:
//
//	RESUME 0
//	LOAD_CONST 0        (function code object)
//	MAKE_FUNCTION 0
//	STORE_NAME funcNameIdx
//	LOAD_CONST 1        (None)
//	RETURN_VALUE 0
func FuncDefModuleBytecode(funcNameIdx byte) []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_CONST), 0,
		byte(MAKE_FUNCTION), 0,
		byte(STORE_NAME), funcNameIdx,
		byte(LOAD_CONST), 1,
		byte(RETURN_VALUE), 0,
	}
}

// FuncDefModuleLineTable returns the PEP 626 line table for a module-level
// def spanning defLine through bodyEndLine, with the last body statement
// ending at bodyEndCol (0-indexed exclusive).
//
// Table entries:
//  1. Prologue: RESUME at synthetic line (1 CU)
//  2. LONG: 5 CUs (LOAD_CONST+MAKE_FUNCTION+STORE_NAME+LOAD_CONST+RETURN_VALUE)
//     attributed to defLine..bodyEndLine at columns [0, bodyEndCol)
func FuncDefModuleLineTable(defLine, bodyEndLine int, bodyEndCol byte) []byte {
	out := make([]byte, 0, 10)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	out = append(out, entryHeader(codeLong, 5))
	out = appendSignedVarint(out, defLine)
	out = appendVarint(out, uint(bodyEndLine-defLine))
	out = appendVarint(out, 1) // start_col+1=1 → start_col=0
	out = appendVarint(out, uint(bodyEndCol)+1)
	return out
}

// FuncReturnArgBytecode returns the instruction stream for a function whose
// single body statement is `return arg` where arg is at argIdx in
// co_localsplusnames.
//
// Pattern:
//
//	RESUME 0
//	LOAD_FAST_BORROW argIdx
//	RETURN_VALUE 0
func FuncReturnArgBytecode(argIdx byte) []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_FAST_BORROW), argIdx,
		byte(RETURN_VALUE), 0,
	}
}

// FuncReturnArgLineTable returns the PEP 626 line table for a function body
// `return arg` where the return keyword is at retKwCol and the argument
// expression spans [argCol, argEnd).
//
// Table entries:
//  1. SHORT0: RESUME at line firstlineno (1 CU, columns [0,0))
//  2. ONE_LINE1/2/LONG: LOAD_FAST_BORROW at bodyLine (1 CU, [argCol, argEnd))
//  3. SHORT0: RETURN_VALUE at same line (1 CU, [retKwCol, argEnd))
func FuncReturnArgLineTable(firstlineno, bodyLine int, argCol, argEnd, retKwCol byte) []byte {
	out := make([]byte, 0, 7)
	out = appendSameLine(out, 1, 0, 0)
	out = appendFirstLineEntry(out, bodyLine-firstlineno, 1, argCol, argEnd)
	out = appendSameLine(out, 1, retKwCol, argEnd)
	return out
}
