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
	NOP            Opcode = 27
	RETURN_VALUE   Opcode = 35
	BINARY_OP      Opcode = 44
	COPY           Opcode = 59
	LOAD_CONST     Opcode = 82
	LOAD_NAME      Opcode = 93
	LOAD_SMALL_INT Opcode = 94
	STORE_NAME     Opcode = 116
	RESUME         Opcode = 128
)

// CacheSize maps each opcode to the number of inline cache entries that
// follow it in the bytecode stream. Each entry is two bytes (one
// instruction word). Values are zero-initialized for opcodes we do not
// list; v0.0.1 does not emit any cached opcode, so the empty defaults are
// correct.
//
// SOURCE: github.com/tamnd/goipy/op/opcodes.go::Cache (CPython 3.14
// _PyOpcode_Caches).
// NbInplaceAdd is the BINARY_OP oparg for the `+=` operator (NB_INPLACE_ADD).
const NbInplaceAdd = 13

var CacheSize = [256]uint8{
	44: 5, // BINARY_OP: 5 inline-cache words (10 bytes)
}
