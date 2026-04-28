package ir

import "errors"

// Encode walks an InstrSeq and emits the on-disk bytecode plus the
// line-table bytes.
//
// At v0.6.3 the encoder is a skeleton: it emits each instruction's
// bytes (Op + EXTENDED_ARG chain + cache zero-fill) and returns
// OrigLineTable verbatim. v0.6.5's assembler replaces the line
// table passthrough with a generic encoder over per-instruction
// Loc runs.
//
// Returns (bytecode, linetable, err). Err is non-nil only if
// InstrSeq is malformed (no blocks, etc.); the encoder itself
// cannot fail on a well-formed sequence.
func Encode(seq *InstrSeq) ([]byte, []byte, error) {
	if seq == nil {
		return nil, nil, errors.New("ir.Encode: nil InstrSeq")
	}
	var out []byte
	for _, b := range seq.Blocks {
		for _, instr := range b.Instrs {
			out = append(out, instr.Bytes()...)
		}
	}
	return out, seq.OrigLineTable, nil
}
