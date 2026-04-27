package bytecode

// CollKind identifies the collection type for collection-literal assignments.
type CollKind uint8

const (
	CollList  CollKind = iota // x = [...]
	CollTuple                 // x = (...)
	CollSet                   // x = {...}  (set, not dict)
	CollDict                  // x = {k: v, ...}
)

// CollElt describes one element (or key+value pair for dicts) in a
// collection-literal assignment. For dicts, elts alternate key/value.
type CollElt struct {
	Name    string
	Col     byte
	NameLen byte
}

// CollectionEmptyBytecode returns the instruction stream for an empty
// collection literal assignment `x = []`, `x = ()`, or `x = {}`.
//
// For list and dict the pattern is BUILD_LIST/BUILD_MAP 0; for tuple
// CPython constant-folds `()` and emits LOAD_CONST 1 (the () sentinel).
//
// co_names: [x]
// co_consts: [None] for list/dict; [None, ()] for tuple
// co_stacksize: 1
func CollectionEmptyBytecode(kind CollKind) []byte {
	out := make([]byte, 0, 10)
	out = append(out, byte(RESUME), 0)
	switch kind {
	case CollTuple:
		out = append(out, byte(LOAD_CONST), 1) // consts[1] = ()
	case CollList:
		out = append(out, byte(BUILD_LIST), 0)
	case CollDict:
		out = append(out, byte(BUILD_MAP), 0)
	}
	out = append(out, byte(STORE_NAME), 0)
	out = append(out, byte(LOAD_CONST), 0) // None
	out = append(out, byte(RETURN_VALUE), 0)
	return out
}

// CollectionEmptyLineTable returns the PEP 626 line table for an empty
// collection literal `x = []`, `x = ()`, or `x = {}` on the given source
// line.
//
// openCol: column of the opening bracket/paren/brace.
// closeEnd: exclusive end column of the closing bracket (= lineEndCol for
//
//	a single-line statement).
//
// targetLen: byte length of the target name x (always at column 0).
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. BUILD_*/LOAD_CONST: 1 code unit at (openCol, closeEnd)
//  3. STORE_NAME + LOAD_CONST None + RETURN_VALUE: 3 code units at (0, targetLen)
func CollectionEmptyLineTable(line int, openCol, closeEnd, targetLen byte) []byte {
	out := make([]byte, 0, 5+3+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)        // prologue (RESUME)
	out = appendValueEntry(out, line, openCol, closeEnd)    // BUILD_* 0 or LOAD_CONST ()
	out = appendSameLine(out, 3, 0, targetLen)              // STORE_NAME+LOAD_CONST+RETURN_VALUE
	return out
}

// CollectionNamesBytecode returns the instruction stream for a collection
// literal assignment `x = [e0, e1, ...]` (list), `x = (e0, e1, ...)` (tuple),
// `x = {e0, e1, ...}` (set), or `x = {k0: v0, k1: v1, ...}` (dict) where all
// elements are names.
//
// elts holds one entry per LOAD_NAME instruction. For dicts, elts alternates
// key/value so len(elts) must be even; BUILD_MAP oparg = len(elts)/2.
//
// co_names: [elts[0].Name, ..., elts[N-1].Name, target] (insertion order)
// co_consts: [None]
// co_stacksize: len(elts)  (or 1 if len=1, since BUILD_* collapses to 1 stack slot)
func CollectionNamesBytecode(kind CollKind, n int) []byte {
	// n = len(elts)
	size := 2 + 2*n + 2 + 2 + 2 // RESUME + N×LOAD_NAME + BUILD_* + STORE_NAME + LC + RV
	out := make([]byte, 0, size)
	out = append(out, byte(RESUME), 0)
	for i := range n {
		out = append(out, byte(LOAD_NAME), byte(i))
	}
	buildOparg := byte(n)
	if kind == CollDict {
		buildOparg = byte(n / 2)
	}
	switch kind {
	case CollList:
		out = append(out, byte(BUILD_LIST), buildOparg)
	case CollTuple:
		out = append(out, byte(BUILD_TUPLE), buildOparg)
	case CollSet:
		out = append(out, byte(BUILD_SET), buildOparg)
	case CollDict:
		out = append(out, byte(BUILD_MAP), buildOparg)
	}
	out = append(out, byte(STORE_NAME), byte(n))
	out = append(out, byte(LOAD_CONST), 0)
	out = append(out, byte(RETURN_VALUE), 0)
	return out
}

// CollectionNamesLineTable returns the PEP 626 line table for a non-empty
// collection literal assignment where all elements are names, on the given
// source line.
//
// elts: positions of each name element (same order as in CollectionNamesBytecode).
// openCol: column of the opening bracket.
// closeEnd: exclusive end column of the closing bracket.
// targetLen: length of the target name.
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. LOAD_NAME elts[0]: 1 code unit at (elts[0].Col, elts[0].Col+elts[0].NameLen)
//  3. LOAD_NAME elts[1..N-1]: 1 code unit each at their respective columns
//  4. BUILD_* N: 1 code unit at (openCol, closeEnd)
//  5. STORE_NAME + LOAD_CONST + RETURN_VALUE: 3 code units at (0, targetLen)
func CollectionNamesLineTable(line int, elts []CollElt, openCol, closeEnd, targetLen byte) []byte {
	n := len(elts)
	out := make([]byte, 0, 5+3+2*(n-1)+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01) // prologue (RESUME)
	// first element: new line entry
	out = appendValueEntry(out, line, elts[0].Col, elts[0].Col+elts[0].NameLen)
	// subsequent elements: same line
	for _, e := range elts[1:] {
		out = appendSameLine(out, 1, e.Col, e.Col+e.NameLen)
	}
	// BUILD_* N
	out = appendSameLine(out, 1, openCol, closeEnd)
	// STORE_NAME + LOAD_CONST + RETURN_VALUE
	out = appendSameLine(out, 3, 0, targetLen)
	return out
}
