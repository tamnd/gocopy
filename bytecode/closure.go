package bytecode

// ClosureOuterBytecode returns the bytecode for the outer function in a
// closure of the form `def f(x): def g(): return x; return g`.
//
// localsplusnames[0] = x (arg, promoted to cell, kind 0x66)
// localsplusnames[1] = g (local, kind 0x20)
// co_consts[0] = g code object (no None)
//
// Instruction sequence:
//
//	MAKE_CELL 0               promote x to cell (before RESUME)
//	RESUME 0
//	LOAD_FAST_BORROW 0        load x cell to build closure tuple
//	BUILD_TUPLE 1             closure = (x,)
//	LOAD_CONST 0              g code object
//	MAKE_FUNCTION 0
//	SET_FUNCTION_ATTRIBUTE 8  set __closure__
//	STORE_FAST 1              g = <function>
//	LOAD_FAST_BORROW 1        load g
//	RETURN_VALUE 0
func ClosureOuterBytecode() []byte {
	return []byte{
		byte(MAKE_CELL), 0,
		byte(RESUME), 0,
		byte(LOAD_FAST_BORROW), 0,
		byte(BUILD_TUPLE), 1,
		byte(LOAD_CONST), 0,
		byte(MAKE_FUNCTION), 0,
		byte(SET_FUNCTION_ATTRIBUTE), 8,
		byte(STORE_FAST), 1,
		byte(LOAD_FAST_BORROW), 1,
		byte(RETURN_VALUE), 0,
	}
}

// ClosureInnerBytecode returns the bytecode for the inner function in a
// closure of the form `def g(): return x` where x is a free variable.
//
// localsplusnames[0] = x (free variable, kind 0x80)
//
// Instruction sequence:
//
//	COPY_FREE_VARS 1   copy x from closure cell
//	RESUME 0
//	LOAD_DEREF 0       load free variable x
//	RETURN_VALUE 0
func ClosureInnerBytecode() []byte {
	return []byte{
		byte(COPY_FREE_VARS), 1,
		byte(RESUME), 0,
		byte(LOAD_DEREF), 0,
		byte(RETURN_VALUE), 0,
	}
}

// ClosureOuterLineTable returns the PEP 626 line table for the outer function.
//
//   - outerDefLine: line of `def f(x):`
//   - innerDefLine: line of `def g():` (start of inner def, used as LONG entry start line)
//   - innerRetLine: line of `return x` in g (innerBodyEnd, used as LONG entry end line)
//   - outerRetLine: line of `return g` in f
//   - innerDefCol: column of the `def` keyword in `def g():`
//   - innerBodyEndCol: exclusive end column of the last token in g's body
//   - outerRetArgCol: column of `g` in `return g`
//   - outerRetArgEnd: exclusive end column of `g` in `return g`
//   - outerRetKwCol: column of the `return` keyword in `return g`
//
// Entries:
//  1. NO_INFO 1 CU — MAKE_CELL (synthetic, no source location)
//  2. SHORT0 1 CU — RESUME at outerDefLine
//  3. LONG 6 CU — LOAD_FAST_BORROW+BUILD_TUPLE+LOAD_CONST+MAKE_FUNCTION+SET_FUNCTION_ATTRIBUTE+STORE_FAST
//     start=innerDefLine, end=innerRetLine, cols=[innerDefCol, innerBodyEndCol)
//  4. ONE_LINE* 1 CU — LOAD_FAST_BORROW g at outerRetLine, cols=[outerRetArgCol, outerRetArgEnd)
//  5. SHORT* 1 CU — RETURN_VALUE at outerRetLine, cols=[outerRetKwCol, outerRetArgEnd)
func ClosureOuterLineTable(outerDefLine, innerDefLine, innerRetLine, outerRetLine int,
	innerDefCol, innerBodyEndCol, outerRetArgCol, outerRetArgEnd, outerRetKwCol byte) []byte {
	out := make([]byte, 0, 14)
	out = append(out, entryHeader(codeNoInfo, 1))
	out = appendSameLine(out, 1, 0, 0)
	out = append(out, entryHeader(codeLong, 6))
	out = appendSignedVarint(out, innerDefLine-outerDefLine)
	out = appendVarint(out, uint(innerRetLine-innerDefLine))
	out = appendVarint(out, uint(innerDefCol)+1)
	out = appendVarint(out, uint(innerBodyEndCol)+1)
	out = appendFirstLineEntry(out, outerRetLine-innerDefLine, 1, outerRetArgCol, outerRetArgEnd)
	out = appendSameLine(out, 1, outerRetKwCol, outerRetArgEnd)
	return out
}

// ClosureInnerLineTable returns the PEP 626 line table for the inner function.
//
//   - innerDefLine: line of `def g():` (= g.firstlineno)
//   - innerRetLine: line of `return x`
//   - innerFreeArgCol: column of x in `return x`
//   - innerFreeArgEnd: exclusive end column of x in `return x`
//   - innerRetKwCol: column of the `return` keyword in `return x`
//
// Entries:
//  1. NO_INFO 1 CU — COPY_FREE_VARS (synthetic)
//  2. SHORT0 1 CU — RESUME at innerDefLine
//  3. ONE_LINE* 1 CU — LOAD_DEREF x at innerRetLine, cols=[innerFreeArgCol, innerFreeArgEnd)
//  4. SHORT* 1 CU — RETURN_VALUE at innerRetLine, cols=[innerRetKwCol, innerFreeArgEnd)
func ClosureInnerLineTable(innerDefLine, innerRetLine int,
	innerFreeArgCol, innerFreeArgEnd, innerRetKwCol byte) []byte {
	out := make([]byte, 0, 8)
	out = append(out, entryHeader(codeNoInfo, 1))
	out = appendSameLine(out, 1, 0, 0)
	out = appendFirstLineEntry(out, innerRetLine-innerDefLine, 1, innerFreeArgCol, innerFreeArgEnd)
	out = appendSameLine(out, 1, innerRetKwCol, innerFreeArgEnd)
	return out
}
