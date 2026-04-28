// Package ir is the instruction-sequence IR that gocopy's compiler
// passes consume. It mirrors CPython 3.14's
// Python/instruction_sequence.c: a flat sequence of (opcode, oparg,
// location) triples organized into blocks, plus the inline-cache
// machinery and the line-table linkage.
//
// At v0.6.3 the IR is dormant infrastructure. The decoder
// (CodeObject → InstrSeq) and the encoder skeleton (InstrSeq →
// bytecode + linetable) are wired and round-trip tested across all
// 246 fixtures. The classifier still owns codegen — v0.6.6 starts
// emitting InstrSeq directly.
//
// SOURCE: CPython 3.14 Python/instruction_sequence.c and
// Include/internal/pycore_instruction_sequence.h.
package ir

import "github.com/tamnd/gocopy/bytecode"

// Instr is one IR instruction: an opcode, a 32-bit oparg, and the
// source location the instruction came from.
//
// CPython 3.14 encodes oparg as one byte per instruction word and
// uses leading EXTENDED_ARG instructions to widen the effective
// oparg up to 32 bits. The IR collapses the chain into a single
// uint32; emitting EXTENDED_ARG words is the encoder's job.
type Instr struct {
	Op  bytecode.Opcode
	Arg uint32
	Loc bytecode.Loc
}

// extendedArg is the CPython 3.14 EXTENDED_ARG opcode. We intern
// the constant here so neither the decoder nor the encoder has to
// reach into bytecode/opcode.go for an opcode the rest of gocopy
// does not emit.
//
// SOURCE: github.com/tamnd/goipy/op/opcodes.go (EXTENDED_ARG = 69).
const extendedArg bytecode.Opcode = 69

// Bytes returns the on-disk byte sequence for this instruction:
// zero or more EXTENDED_ARG words encoding the high bytes of Arg,
// then one (Op, low byte of Arg) word, then 2 * CacheSize[Op]
// zero bytes for inline caches.
//
// CPython emits zero-filled cache words for fresh code objects
// produced by py_compile — the runtime fills them in on first
// execution. gocopy mirrors that.
func (i Instr) Bytes() []byte {
	hi3 := byte(i.Arg >> 24)
	hi2 := byte(i.Arg >> 16)
	hi1 := byte(i.Arg >> 8)
	lo := byte(i.Arg)
	cache := bytecode.CacheSize[i.Op]
	out := make([]byte, 0, 2+int(cache)*2+6)
	if hi3 != 0 {
		out = append(out, byte(extendedArg), hi3)
		out = append(out, byte(extendedArg), hi2)
		out = append(out, byte(extendedArg), hi1)
	} else if hi2 != 0 {
		out = append(out, byte(extendedArg), hi2)
		out = append(out, byte(extendedArg), hi1)
	} else if hi1 != 0 {
		out = append(out, byte(extendedArg), hi1)
	}
	out = append(out, byte(i.Op), lo)
	for range cache {
		out = append(out, 0, 0)
	}
	return out
}
