package flowgraph

import (
	"github.com/tamnd/gocopy/v1/ir"
)

// removeUnreachable zeroes out the instruction list of every block
// not reachable from seq.Blocks[0] via fallthrough or jump edges.
// Mirrors CPython 3.14 Python/flowgraph.c:996 remove_unreachable
// verbatim:
//
//	"Mark every block reachable from the entry block. Then delete
//	 the instructions of any unreachable block."
//
// Algorithm: BFS from the entry block via two edge kinds —
// fallthrough (when the block's last instr is not a terminator) and
// jump (when the block's last instr is a jump op, target resolved
// via the LabelID embedded in instr.Arg).
//
// Unreachable blocks are NOT removed from seq.Blocks here — that is
// the job of resolveJumps (which compacts empty blocks during
// linearisation). Zeroing b.Instrs preserves the invariant that
// later passes can iterate seq.Blocks safely. CPython does the
// equivalent: it zeroes b_iused and lets later passes (compact_basic_block)
// reclaim storage.
//
// SOURCE: CPython 3.14 Python/flowgraph.c:996 remove_unreachable.
func removeUnreachable(seq *ir.InstrSeq) {
	if seq == nil || len(seq.Blocks) == 0 {
		return
	}
	visited := make(map[uint32]bool, len(seq.Blocks))
	stack := []*ir.Block{seq.Blocks[0]}
	visited[seq.Blocks[0].ID] = true
	for len(stack) > 0 {
		b := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		// Fallthrough successor.
		if blockFallsThrough(b) {
			if next := nextBlockBySliceOrder(seq, b); next != nil && !visited[next.ID] {
				visited[next.ID] = true
				stack = append(stack, next)
			}
		}
		// Jump target.
		if len(b.Instrs) > 0 {
			last := b.Instrs[len(b.Instrs)-1]
			if isJump(last.Op) {
				if target := jumpTargetBlock(seq, ir.LabelID(last.Arg)); target != nil && !visited[target.ID] {
					visited[target.ID] = true
					stack = append(stack, target)
				}
			}
		}
	}
	for _, b := range seq.Blocks {
		if !visited[b.ID] {
			b.Instrs = nil
		}
	}
}

// blockFallsThrough reports whether b's last instruction allows
// control to flow into the next block. An empty block falls through
// unconditionally; a block ending in a non-terminator op
// (including conditional jumps and FOR_ITER, which DO have a
// fallthrough) falls through; a block ending in an unconditional
// jump or RETURN_VALUE/RAISE_VARARGS does not.
func blockFallsThrough(b *ir.Block) bool {
	if len(b.Instrs) == 0 {
		return true
	}
	last := b.Instrs[len(b.Instrs)-1]
	return !isTerminator(last.Op)
}

// nextBlockBySliceOrder returns the block at seq.Blocks[i+1] where
// seq.Blocks[i] == b, or nil when b is the last block.
func nextBlockBySliceOrder(seq *ir.InstrSeq, b *ir.Block) *ir.Block {
	for i, s := range seq.Blocks {
		if s == b {
			if i+1 < len(seq.Blocks) {
				return seq.Blocks[i+1]
			}
			return nil
		}
	}
	return nil
}
