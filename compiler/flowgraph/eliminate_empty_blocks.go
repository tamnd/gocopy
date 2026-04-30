package flowgraph

import "github.com/tamnd/gocopy/compiler/ir"

// eliminateEmptyBlocks drops empty labelled blocks from seq by
// retargeting jumps that pointed at them to the block's first
// non-empty successor. Mirrors CPython 3.14
// Python/flowgraph.c::eliminate_empty_basic_blocks.
//
// The v0.7.6 visit_If shapes emit no empty bridging blocks (each
// branch terminates with its own LOAD_CONST None + RETURN_VALUE
// tail; the with-orelse case never adds an end block at all). The
// pass is therefore a no-op for v0.7.6 fixtures; it lands now so
// future shapes that DO produce empty bridges have it ready.
//
// Algorithm:
//
//   - For each empty labelled block b in seq.Blocks (forward
//     order):
//   - Find the first non-empty successor n. If n has no label,
//     transfer b.Label to n. If n is also labelled, retarget every
//     jump in seq whose Arg == uint32(b.Label) to point at
//     n.Label. Then drop b from seq.Blocks.
//
// The pass is idempotent: a second run after the fixed point is a
// no-op. v0.7.6 calls it once before inlineSmallExitBlocks.
//
// SOURCE: CPython 3.14 Python/flowgraph.c::eliminate_empty_basic_blocks.
func eliminateEmptyBlocks(seq *ir.InstrSeq) {
	if seq == nil || len(seq.Blocks) <= 1 {
		return
	}
	for i := 0; i < len(seq.Blocks); {
		b := seq.Blocks[i]
		if len(b.Instrs) != 0 || b.Label == ir.NoLabel {
			i++
			continue
		}
		n := nextNonEmptyBlock(seq, i)
		if n == nil {
			i++
			continue
		}
		if n.Label == ir.NoLabel {
			n.Label = b.Label
		} else {
			retargetJumps(seq, b.Label, n.Label)
		}
		seq.Blocks = append(seq.Blocks[:i], seq.Blocks[i+1:]...)
	}
}

// nextNonEmptyBlock returns the first block at index > idx whose
// Instrs is non-empty, or nil if no such block exists.
func nextNonEmptyBlock(seq *ir.InstrSeq, idx int) *ir.Block {
	for j := idx + 1; j < len(seq.Blocks); j++ {
		if len(seq.Blocks[j].Instrs) > 0 {
			return seq.Blocks[j]
		}
	}
	return nil
}

// retargetJumps rewrites every jump instruction in seq whose Arg
// equals uint32(from) to uint32(to). At eliminateEmptyBlocks call
// time, jump Args are still LabelIDs (resolveJumps hasn't run).
func retargetJumps(seq *ir.InstrSeq, from, to ir.LabelID) {
	for _, b := range seq.Blocks {
		for k := range b.Instrs {
			instr := &b.Instrs[k]
			if !isJumpOp(instr.Op) {
				continue
			}
			if instr.Arg == uint32(from) {
				instr.Arg = uint32(to)
			}
		}
	}
}
