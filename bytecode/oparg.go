package bytecode

import "fmt"

// LoadGlobalArg encodes the LOAD_GLOBAL oparg as CPython does:
// the high seven bits are the index into co_names, the low bit
// signals whether to push NULL ahead of the loaded value (used by
// CPython's calling convention for `LOAD_GLOBAL` immediately
// followed by `CALL`).
//
// SOURCE: CPython 3.14 Python/compile.c, codegen_load_global.
func LoadGlobalArg(nameIdx byte, pushNull bool) byte {
	if nameIdx >= 128 {
		panic(fmt.Sprintf("LoadGlobalArg: nameIdx=%d out of range [0,128)", nameIdx))
	}
	b := nameIdx << 1
	if pushNull {
		b |= 1
	}
	return b
}

// LoadAttrArg encodes the LOAD_ATTR oparg. The high seven bits are
// the index into co_names; the low bit signals method-load form
// (LOAD_METHOD-style: pushes NULL+method instead of the bare
// attribute).
//
// SOURCE: CPython 3.14 Python/compile.c, codegen_load_attr.
func LoadAttrArg(nameIdx byte, asMethod bool) byte {
	if nameIdx >= 128 {
		panic(fmt.Sprintf("LoadAttrArg: nameIdx=%d out of range [0,128)", nameIdx))
	}
	b := nameIdx << 1
	if asMethod {
		b |= 1
	}
	return b
}

// LflblflbArg encodes the oparg for the
// LOAD_FAST_BORROW_LOAD_FAST_BORROW super-instruction: high nibble
// is the left slot, low nibble is the right slot. Both slots must
// fit in 4 bits.
func LflblflbArg(slotL, slotR byte) byte {
	if slotL >= 16 || slotR >= 16 {
		panic(fmt.Sprintf("LflblflbArg: slots %d,%d out of range [0,16)", slotL, slotR))
	}
	return (slotL << 4) | slotR
}

// CompareCondArg adds 16 to a value-context COMPARE_OP oparg to
// produce the conditional-context oparg. CPython's compiler embeds
// the comparison into the following jump by setting bit 4 of the
// oparg.
//
// SOURCE: CPython 3.14 Python/compile.c, compiler_compare and
// the COMPARISON_BITS layout in Lib/dis.py.
func CompareCondArg(base byte) byte {
	return base + 16
}

// Predefined co_localspluskinds bytes for the slot shapes the
// compiler emits today. Each is an OR-combination of the FastX
// bits in flags.go.
const (
	// LocalsKindLocal: a plain local (variable assigned in body).
	LocalsKindLocal byte = FastLocal
	// LocalsKindArg: a positional/kw argument that stays a plain
	// local (CO_FAST_LOCAL | CO_FAST_ARG).
	LocalsKindArg byte = FastLocal | FastArg
	// LocalsKindArgCell: a cell holding a positional/kw argument
	// captured by a nested closure (CO_FAST_LOCAL | CO_FAST_ARG |
	// CO_FAST_CELL).
	LocalsKindArgCell byte = FastLocal | FastArg | FastCell
	// LocalsKindFree: a free variable referenced from an outer
	// scope.
	LocalsKindFree byte = FastFree
	// LocalsKindCell: a cell that backs a closure-bound name.
	LocalsKindCell byte = FastCell
)
