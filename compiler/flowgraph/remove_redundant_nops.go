package flowgraph

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// removeRedundantNops drops NOP instructions that contribute no
// line-table data. Mirrors CPython 3.14
// Python/flowgraph.c:1044 basicblock_remove_redundant_nops verbatim.
//
// Per-NOP elision rules (any of these makes the NOP redundant):
//
//  1. NOP at NO_LOCATION (line < 0 in CPython, line == 0 here).
//  2. NOP whose line matches the previous emitted instruction's line
//     (same-line consecutive instructions don't need a NOP marker).
//  3. NOP whose line matches the next instruction's line in the
//     same block.
//  4. NOP whose line matches the first non-NO_LOCATION instruction
//     of the next non-empty block.
//
// When the NOP is the last instruction in its block AND the next
// instruction is NO_LOCATION, the next instruction inherits the
// NOP's loc before the NOP is dropped (CPython preserves this so the
// PEP 626 line event still fires).
//
// SOURCE: CPython 3.14 Python/flowgraph.c:1044
// basicblock_remove_redundant_nops.
func removeRedundantNops(seq *ir.InstrSeq) {
	if seq == nil {
		return
	}
	for i, b := range seq.Blocks {
		basicblockRemoveRedundantNops(seq, i, b)
	}
}

func basicblockRemoveRedundantNops(seq *ir.InstrSeq, blockIdx int, b *ir.Block) {
	dest := 0
	prevLine := uint32(0)
	havePrev := false
	for src := 0; src < len(b.Instrs); src++ {
		instr := b.Instrs[src]
		line := instr.Loc.Line
		if instr.Op == bytecode.NOP {
			// Rule 1: NOP at NO_LOCATION.
			if line == 0 {
				continue
			}
			// Rule 2: NOP whose line matches the prior emitted
			// instr's line.
			if havePrev && prevLine == line {
				continue
			}
			// Rule 3 / 4: line matches a successor.
			if src < len(b.Instrs)-1 {
				nextLine := b.Instrs[src+1].Loc.Line
				if nextLine == line {
					continue
				}
				if nextLine == 0 {
					// Pull this NOP's loc into the next instr,
					// then drop the NOP.
					b.Instrs[src+1].Loc = instr.Loc
					continue
				}
			} else {
				next := nextNonEmptyBlockSeq(seq, blockIdx)
				if next != nil {
					nextLine := firstSignificantLine(next)
					if nextLine == line {
						continue
					}
				}
			}
		}
		if dest != src {
			b.Instrs[dest] = b.Instrs[src]
		}
		dest++
		prevLine = line
		havePrev = true
	}
	b.Instrs = b.Instrs[:dest]
}

// nextNonEmptyBlockSeq returns the first block at index > idx whose
// Instrs slice is non-empty, or nil. Mirrors CPython's
// next_nonempty_block walk over b_next.
func nextNonEmptyBlockSeq(seq *ir.InstrSeq, idx int) *ir.Block {
	for j := idx + 1; j < len(seq.Blocks); j++ {
		if len(seq.Blocks[j].Instrs) > 0 {
			return seq.Blocks[j]
		}
	}
	return nil
}

// firstSignificantLine returns the line of the first instruction in
// b that is not a NO_LOCATION NOP (since those are about to be
// elided). Returns 0 when the block has nothing significant.
func firstSignificantLine(b *ir.Block) uint32 {
	for _, instr := range b.Instrs {
		if instr.Op == bytecode.NOP && instr.Loc.Line == 0 {
			continue
		}
		return instr.Loc.Line
	}
	return 0
}
