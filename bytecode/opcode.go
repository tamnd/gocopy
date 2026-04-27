// Package bytecode holds CPython 3.14 opcode constants, the inline cache
// size table, the CodeObject builder, and the line/exception table encoders.
//
// Opcode numbers and the cache size table are sourced from
// github.com/tamnd/goipy/op (the canonical opcode source in this ecosystem),
// which itself is generated from python3.14 dis.opname/opcode.
package bytecode

// Opcode is a single CPython 3.14 opcode byte.
type Opcode uint8

// CPython 3.14 opcode numbers. Only the opcodes the current gocopy version
// actually emits are listed here; subsequent versions add the rest as needed.
//
// SOURCE: github.com/tamnd/goipy/op/opcodes.go (run `go generate ./op` in
// goipy to regenerate from upstream).
const (
	NOP               Opcode = 27
	NOT_TAKEN         Opcode = 28
	POP_TOP           Opcode = 31
	PUSH_NULL         Opcode = 33
	RETURN_VALUE      Opcode = 35
	STORE_SUBSCR      Opcode = 38
	TO_BOOL           Opcode = 39
	UNARY_INVERT      Opcode = 40
	UNARY_NEGATIVE    Opcode = 41
	UNARY_NOT         Opcode = 42
	BINARY_OP         Opcode = 44
	BUILD_LIST        Opcode = 46
	BUILD_MAP         Opcode = 47
	BUILD_SET         Opcode = 48
	BUILD_TUPLE       Opcode = 51
	CALL              Opcode = 52
	CALL_INTRINSIC_1  Opcode = 53
	COMPARE_OP        Opcode = 56
	CONTAINS_OP       Opcode = 57
	COPY              Opcode = 59
	IS_OP             Opcode = 74
	JUMP_BACKWARD     Opcode = 75
	JUMP_FORWARD      Opcode = 77
	LOAD_ATTR         Opcode = 80
	LOAD_CONST        Opcode = 82
	LOAD_NAME         Opcode = 93
	LOAD_SMALL_INT    Opcode = 94
	POP_JUMP_IF_FALSE Opcode = 100
	POP_JUMP_IF_TRUE  Opcode = 103
	STORE_NAME        Opcode = 116
	STORE_ATTR        Opcode = 110
	RESUME            Opcode = 128
)

// CacheSize maps each opcode to the number of inline cache entries that
// follow it in the bytecode stream. Each entry is two bytes (one
// instruction word). Values are zero-initialized for opcodes we do not
// list; v0.0.1 does not emit any cached opcode, so the empty defaults are
// correct.
//
// SOURCE: github.com/tamnd/goipy/op/opcodes.go::Cache (CPython 3.14
// _PyOpcode_Caches).
var CacheSize = [256]uint8{
	38:  1, // STORE_SUBSCR: 1 inline-cache word (2 bytes)
	39:  3, // TO_BOOL: 3 inline-cache words (6 bytes)
	52:  3, // CALL: 3 inline-cache words (6 bytes)
	44:  5, // BINARY_OP: 5 inline-cache words (10 bytes)
	56:  1, // COMPARE_OP: 1 inline-cache word (2 bytes)
	57:  1, // CONTAINS_OP: 1 inline-cache word (2 bytes)
	80:  9, // LOAD_ATTR: 9 inline-cache words (18 bytes)
	75:  1, // JUMP_BACKWARD: 1 inline-cache word (2 bytes)
	100: 1, // POP_JUMP_IF_FALSE: 1 inline-cache word (2 bytes)
	103: 1, // POP_JUMP_IF_TRUE: 1 inline-cache word (2 bytes)
	110: 4, // STORE_ATTR: 4 inline-cache words (8 bytes)
}

// COMPARE_OP oparg values for non-conditional (value) context.
// In conditional-jump context, add 16 to these values.
// SOURCE: empirically verified against CPython 3.14 `python3.14 -m py_compile`.
const (
	CmpLt   = 2   // <
	CmpLtE  = 42  // <=
	CmpEq   = 72  // ==
	CmpNotEq = 103 // !=
	CmpGt   = 132 // >
	CmpGtE  = 172 // >=
)

// NbGetItem is the BINARY_OP oparg for subscript reads `a[b]`.
// It is the NB_GET_ITEM slot in CPython's binary-operation dispatch table.
const NbGetItem = 26

// BINARY_OP opargs for non-inplace binary operators (NB_* enum).
// SOURCE: github.com/tamnd/goipy/op/opcodes.go (NB_* constants).
const (
	NbAdd            = 0
	NbAnd            = 1
	NbFloorDivide    = 2
	NbLshift         = 3
	NbMatrixMultiply = 4
	NbMultiply       = 5
	NbRemainder      = 6
	NbOr             = 7
	NbPower          = 8
	NbRshift         = 9
	NbSubtract       = 10
	NbTrueDivide     = 11
	NbXor            = 12
)

// BINARY_OP opargs for augmented assignment operators (NB_INPLACE_* enum).
// SOURCE: github.com/tamnd/goipy/op/opcodes.go (NB_INPLACE_* constants).
const (
	NbInplaceAdd         = 13
	NbInplaceAnd         = 14
	NbInplaceFloorDivide = 15
	NbInplaceLshift      = 16
	NbInplaceMultiply    = 18
	NbInplaceRemainder   = 19
	NbInplaceOr          = 20
	NbInplacePower       = 21
	NbInplaceRshift      = 22
	NbInplaceSubtract    = 23
	NbInplaceTrueDivide  = 24
	NbInplaceXor         = 25
)
