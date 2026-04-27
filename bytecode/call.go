package bytecode

// CallArg describes one positional argument in a function call assignment.
type CallArg struct {
	Name    string
	Col     byte
	NameLen byte
}

// CallAssignBytecode returns the instruction stream for `x = f(args...)`
// where f and all args are names and there are no keyword arguments.
//
// Bytecode pattern (N args):
//
//	RESUME 0
//	LOAD_NAME 0 (f)
//	PUSH_NULL 0
//	LOAD_NAME 1 (arg0)   // repeated N times
//	  ...
//	CALL N + 3 cache words
//	STORE_NAME N+1 (x)
//	LOAD_CONST 0 (None)
//	RETURN_VALUE 0
//
// co_names: [f, arg0, ..., argN-1, x]
// co_consts: [None]
// co_stacksize: 2+N (LOAD_NAME f + PUSH_NULL = 2 stack slots, each arg adds 1)
func CallAssignBytecode(n int) []byte {
	size := 2 + 2 + 2*n + 2 + 6 + 2 + 2 + 2 // RESUME+LOAD_NAME+PUSH_NULL+N*LOAD_NAME+CALL+3caches+STORE+LC+RV
	out := make([]byte, 0, size)
	out = append(out, byte(RESUME), 0)
	out = append(out, byte(LOAD_NAME), 0) // f
	out = append(out, byte(PUSH_NULL), 0)
	for i := range n {
		out = append(out, byte(LOAD_NAME), byte(i+1))
	}
	out = append(out, byte(CALL), byte(n))
	out = append(out, 0, 0, 0, 0, 0, 0) // 3 cache words
	out = append(out, byte(STORE_NAME), byte(n+1))
	out = append(out, byte(LOAD_CONST), 0)
	out = append(out, byte(RETURN_VALUE), 0)
	return out
}

// CallAssignLineTable returns the PEP 626 line table for `x = f(args...)`.
//
// funcCol/funcEnd: column range of the function name (f).
// args: positions of each argument name.
// closeEnd: exclusive end column of the closing `)` (= lineEndCol for a simple call).
// targetLen: length of the target name x.
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. LOAD_NAME f + PUSH_NULL: 2 code units at (funcCol, funcEnd)
//  3. LOAD_NAME arg0..argN-1: 1 code unit each
//  4. CALL N + 3 caches: 4 code units at (funcCol, closeEnd)
//  5. STORE_NAME + LOAD_CONST + RETURN_VALUE: 3 code units at (0, targetLen)
func CallAssignLineTable(line int, funcCol, funcEnd byte, args []CallArg, closeEnd, targetLen byte) []byte {
	n := len(args)
	out := make([]byte, 0, 5+3+2*n+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)                     // prologue
	out = appendFirstLineEntry(out, line, 2, funcCol, funcEnd)            // LOAD_NAME f + PUSH_NULL
	for _, a := range args {
		out = appendSameLine(out, 1, a.Col, a.Col+a.NameLen)             // LOAD_NAME argI
	}
	out = appendSameLine(out, 4, funcCol, closeEnd)                      // CALL + 3 caches
	out = appendSameLine(out, 3, 0, targetLen)                           // STORE + LC + RV
	return out
}
