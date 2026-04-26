package bytecode

// CodeObject is gocopy's mutable representation of a CPython code object.
// The marshal package walks this struct in field order matching
// Python/marshal.c::w_object's code-object branch and emits the wire format.
//
// Field order is the canonical CPython 3.14 code-object marshal order:
//
//	argcount, posonlyargcount, kwonlyargcount, stacksize, flags,
//	bytecode, consts, names, localsplusnames, localspluskinds,
//	filename, name, qualname, firstlineno, linetable, exceptiontable.
type CodeObject struct {
	ArgCount        int32
	PosOnlyArgCount int32
	KwOnlyArgCount  int32
	StackSize       int32
	Flags           uint32

	Bytecode []byte // raw instruction bytes (opcode, oparg, cache words)

	// Consts holds the value table. Each entry is one of: nil (None),
	// bool, int64, float64, complex128, string, []byte, *CodeObject, or
	// a tuple represented as []any. v0.0.1 only emits a single nil for
	// the implicit `return None`.
	Consts []any

	Names           []string // attribute / global names referenced by LOAD_GLOBAL etc.
	LocalsPlusNames []string // varnames + cellvars + freevars in one ordered slice
	LocalsPlusKinds []byte   // kind flag per name (FastLocal/FastCell/FastFree/FastArg/FastHidden)

	Filename     string
	Name         string
	QualName     string
	FirstLineNo  int32
	LineTable    []byte
	ExcTable     []byte
}
