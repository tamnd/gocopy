package ir

import "github.com/tamnd/gocopy/bytecode"

// Block is one basic block in the IR. At v0.6.3 the decoder emits
// exactly one block per CodeObject; CFG construction (v0.6.4)
// splits a flat block into labelled basic blocks at every jump
// target.
//
// Succ is empty until v0.6.4 wires control flow.
type Block struct {
	ID     uint32
	Label  LabelID
	Instrs []Instr
	Succ   []*Block
}

// AddOp appends an instruction with no jump target.
func (b *Block) AddOp(op bytecode.Opcode, arg uint32, loc bytecode.Loc) {
	b.Instrs = append(b.Instrs, Instr{Op: op, Arg: arg, Loc: loc})
}
