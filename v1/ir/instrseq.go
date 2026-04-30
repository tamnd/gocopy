package ir

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/symtable"
)

// InstrSeq is the IR's top-level container. Each module-level
// CodeObject and each function body lowers to one InstrSeq.
//
// Names and Consts are pointers because v0.6.5+ assemblers may
// share them across nested code objects (a function body's
// constants pool is independent of the module's, but names tables
// are de-duplicated by id, not value).
//
// Sym links the IR to the owning symbol-table scope so codegen
// can answer "what is the slot for this name?" in O(1). nil for
// the module top level until v0.6.6 wires it.
//
// OrigLineTable and OrigExcTable are passthrough fields the
// decoder fills in. The encoder skeleton emits them verbatim.
// v0.6.5 generalizes these: the encoder will materialize them
// from per-instruction Loc data and a separate exception-region
// list. Today they keep the IR honest about what bytes the
// CodeObject carried in.
type InstrSeq struct {
	Blocks []*Block

	Names  *bytecode.AtomTable
	Consts *bytecode.ConstPool
	Sym    *symtable.Scope

	FirstLineNo int32

	OrigLineTable []byte
	OrigExcTable  []byte

	nextBlockID uint32
	nextLabel   LabelID
}

// NewInstrSeq returns an empty sequence ready for the builder API.
func NewInstrSeq() *InstrSeq {
	return &InstrSeq{}
}

// AddBlock appends a fresh empty block and returns it. The first
// AddBlock call yields block 0; subsequent calls increment.
func (s *InstrSeq) AddBlock() *Block {
	b := &Block{ID: s.nextBlockID}
	s.nextBlockID++
	s.Blocks = append(s.Blocks, b)
	return b
}

// AllocLabel returns a fresh LabelID. IDs start at 1; LabelID 0
// is reserved for NoLabel.
func (s *InstrSeq) AllocLabel() LabelID {
	s.nextLabel++
	return s.nextLabel
}

// BindLabel marks a block as the destination of a label. Blocks
// without an explicit label keep LabelID(0).
func (s *InstrSeq) BindLabel(id LabelID, b *Block) {
	b.Label = id
}

// AddJump appends a control-transfer instruction. At v0.6.3 the
// target is recorded as the raw oparg (the decoder sees jumps as
// already-resolved offsets), and Patch is a no-op. v0.6.4
// introduces real label resolution.
func (b *Block) AddJump(op bytecode.Opcode, target LabelID, loc bytecode.Loc) {
	b.Instrs = append(b.Instrs, Instr{Op: op, Arg: uint32(target), Loc: loc})
}

// Patch is a placeholder that resolves any pending jump targets
// once block layout is known. v0.6.3 always layouts in one block
// with raw decoded oparg values; nothing to patch. v0.6.4 makes
// this real.
func (s *InstrSeq) Patch() error {
	return nil
}
