package bytecode

// Exported linetable helpers for the general expression compiler in the
// compiler package, which cannot call unexported bytecode functions.

// GenExprProlog returns the 5-byte LONG(1) prologue linetable entry that
// covers the RESUME instruction in a module-level code object:
//
//	f0 03 01 01 01  (code=LONG, length=1, delta=-1, end_delta=1, cols=[0,0))
func GenExprProlog() []byte {
	return []byte{0xf0, 0x03, 0x01, 0x01, 0x01}
}

// GenExprFirstEntry appends a ONE_LINE0/1/2 or LONG entry for the first
// real instruction after RESUME (lineDelta is relative to the synthetic
// line 0, so for line 1 pass lineDelta=1).
func GenExprFirstEntry(out []byte, lineDelta, cuCount int, sc, ec byte) []byte {
	return appendFirstLineEntry(out, lineDelta, cuCount, sc, ec)
}

// GenExprSameLine appends a SHORTn or ONE_LINE0 entry for an instruction
// on the same source line as the previous entry.
func GenExprSameLine(out []byte, cuCount int, sc, ec byte) []byte {
	return appendSameLine(out, cuCount, sc, ec)
}
