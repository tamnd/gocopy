package ir

// LabelID is a dense integer handed out by InstrSeq.AllocLabel.
// Block 0 is reserved for "no label", so legitimate labels start
// at 1. The encoder skeleton at v0.6.3 does not use labels — the
// decoder produces one block holding every decoded instruction.
// v0.6.4's CFG builder splits that block by label boundaries.
type LabelID uint32

// NoLabel is the sentinel for "this block has no symbolic label".
const NoLabel LabelID = 0
