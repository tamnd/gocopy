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

// AssignsThenFuncDefLineTable returns the PEP 626 line table for a module body
// consisting of N ≥ 1 constant-folded assignments followed by a function definition.
// Each assign contributes 2 CUs (LOAD_CONST + STORE_NAME); the funcdef contributes 5.
func AssignsThenFuncDefLineTable(asgns []AssignInfo, defLine, bodyEndLine int, bodyEndCol byte) []byte {
	out := make([]byte, 0, 5+3*len(asgns)+6)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	prevLine := 0
	for _, a := range asgns {
		out = appendValueEntry(out, a.Line-prevLine, a.ValStart, a.ValEnd)
		out = appendShort0Entry(out, 1, 0, a.NameLen)
		prevLine = a.Line
	}
	out = append(out, entryHeader(codeLong, 5))
	out = appendSignedVarint(out, defLine-prevLine)
	out = appendVarint(out, uint(bodyEndLine-defLine))
	out = appendVarint(out, 1)
	out = appendVarint(out, uint(bodyEndCol)+1)
	return out
}

// MultiFuncDefEntry describes one function definition within a modMultiFuncDef module.
type MultiFuncDefEntry struct {
	DefLine     int
	BodyEndLine int
	BodyEndCol  byte
}

// MultiFuncDefLineTable returns the PEP 626 line table for a module body of
// N >= 2 function definitions with no other top-level statements.
//
// Layout:
//   - Prologue: 1 CU (RESUME, no-source)
//   - For each funcdef except the last: 3 CUs (LOAD_CONST + MAKE_FUNCTION + STORE_NAME)
//   - For the last funcdef: 5 CUs (LOAD_CONST + MAKE_FUNCTION + STORE_NAME + LOAD_CONST None + RETURN_VALUE)
func MultiFuncDefLineTable(entries []MultiFuncDefEntry) []byte {
	n := len(entries)
	out := make([]byte, 0, 5+5*n)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	prevLine := 0
	for i, e := range entries {
		cuCount := 3
		if i == n-1 {
			cuCount = 5
		}
		out = append(out, entryHeader(codeLong, cuCount))
		out = appendSignedVarint(out, e.DefLine-prevLine)
		out = appendVarint(out, uint(e.BodyEndLine-e.DefLine))
		out = appendVarint(out, 1) // start_col+1=1 → col 0
		out = appendVarint(out, uint(e.BodyEndCol)+1)
		prevLine = e.DefLine
	}
	return out
}

// MixedModuleInfo describes a mixed module: optional docstring, optional
// constLitColl (__all__), folded BinOp assignments, and function definitions.
type MixedModuleInfo struct {
	HasDocstring bool
	DocLine      int
	DocEndLine   int
	DocEndCol    byte

	HasCLC       bool
	CLCLine      int
	CLCCloseLine int
	CLCOpenCol   byte
	CLCCloseEnd  byte
	CLCTargetLen byte

	Assigns []AssignInfo
	Funcs   []MultiFuncDefEntry
}

// MixedModuleLineTable generates the PEP 626 module-level line table for a
// mixed module with the structure:
//
//	[docstring?] [constLitColl?] [foldedBinOp assigns*] [funcBodyExprs+]
func MixedModuleLineTable(info MixedModuleInfo) []byte {
	out := []byte{0xf0, 0x03, 0x01, 0x01, 0x01} // prologue
	prevLine := 0

	if info.HasDocstring {
		// LOAD_CONST docstring + STORE_NAME __doc__ = 2 CU, multi-line span.
		out = appendListSpanEntry(out, 2, info.DocLine-prevLine, info.DocEndLine-info.DocLine, 0, info.DocEndCol)
		prevLine = info.DocLine
	}

	if info.HasCLC {
		// BUILD_LIST 0 + LOAD_CONST tuple + LIST_EXTEND 1 = 3 CU
		out = appendListSpanEntry(out, 3, info.CLCLine-prevLine, info.CLCCloseLine-info.CLCLine, info.CLCOpenCol, info.CLCCloseEnd)
		// STORE_NAME = 1 CU, same line, cols [0, targetLen)
		out = appendShort0Entry(out, 1, 0, info.CLCTargetLen)
		prevLine = info.CLCLine
	}

	for _, a := range info.Assigns {
		out = appendValueEntry(out, a.Line-prevLine, a.ValStart, a.ValEnd)
		out = appendShort0Entry(out, 1, 0, a.NameLen)
		prevLine = a.Line
	}

	n := len(info.Funcs)
	for i, f := range info.Funcs {
		cuCount := 3
		if i == n-1 {
			cuCount = 5
		}
		out = append(out, entryHeader(codeLong, cuCount))
		out = appendSignedVarint(out, f.DefLine-prevLine)
		out = appendVarint(out, uint(f.BodyEndLine-f.DefLine))
		out = appendVarint(out, 1)
		out = appendVarint(out, uint(f.BodyEndCol)+1)
		prevLine = f.DefLine
	}
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
