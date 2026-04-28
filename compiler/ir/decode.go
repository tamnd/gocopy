package ir

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
)

// Decode walks a CodeObject and produces an InstrSeq. The output
// is one block holding every decoded instruction in source order.
// CFG construction (block splitting at jump targets) is deferred
// to v0.6.4.
//
// EXTENDED_ARG instructions are folded into the following real
// instruction's Arg field. Inline cache words (per
// bytecode.CacheSize) are skipped during decode and re-emitted
// during encode as zero bytes.
//
// The line table is decoded into a per-code-unit Loc slice; each
// instruction's Loc is taken from the unit at the instruction's
// starting offset.
//
// OrigLineTable and OrigExcTable are preserved verbatim so the
// encoder skeleton can emit them byte-identically. v0.6.5 replaces
// the passthrough with a generic encoder over Loc runs.
func Decode(co *bytecode.CodeObject) (*InstrSeq, error) {
	if co == nil {
		return nil, errors.New("ir.Decode: nil CodeObject")
	}
	codeUnits := len(co.Bytecode) / 2
	locs, err := decodeLineTable(co.LineTable, co.FirstLineNo, codeUnits)
	if err != nil {
		return nil, err
	}
	seq := NewInstrSeq()
	seq.FirstLineNo = co.FirstLineNo
	seq.OrigLineTable = co.LineTable
	seq.OrigExcTable = co.ExcTable
	block := seq.AddBlock()

	var pendingArg uint32
	pendingShift := 0
	bc := co.Bytecode
	for p := 0; p < len(bc); {
		if p+1 >= len(bc) {
			return nil, errors.New("ir.Decode: truncated instruction")
		}
		op := bytecode.Opcode(bc[p])
		arg := uint32(bc[p+1])
		unit := p / 2
		var loc bytecode.Loc
		if unit < len(locs) {
			loc = locs[unit]
		}
		if op == extendedArg {
			pendingArg = (pendingArg << 8) | arg
			pendingShift++
			if pendingShift > 3 {
				return nil, errors.New("ir.Decode: EXTENDED_ARG chain too long")
			}
			p += 2
			continue
		}
		fullArg := (pendingArg << 8) | arg
		if pendingShift == 0 {
			fullArg = arg
		}
		block.Instrs = append(block.Instrs, Instr{
			Op:  op,
			Arg: fullArg,
			Loc: loc,
		})
		pendingArg = 0
		pendingShift = 0
		p += 2
		cache := int(bytecode.CacheSize[op]) * 2
		p += cache
	}
	if pendingShift != 0 {
		return nil, errors.New("ir.Decode: dangling EXTENDED_ARG at end of bytecode")
	}
	return seq, nil
}
