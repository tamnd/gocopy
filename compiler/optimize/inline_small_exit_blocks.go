package optimize

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// inlineSmallExitBlocks duplicates a small terminator block (one
// ending in RETURN_VALUE, with at most maxInlineExitInstrs
// instructions) into every predecessor that reaches it via
// fall-through or unconditional JUMP_FORWARD, then removes the merge
// block from seq.Blocks (and drops any consumed JUMP_FORWARD).
//
// At v0.7.6 this is a narrow specialisation of CPython 3.14
// Python/flowgraph.c::inline_small_or_no_lineno_blocks for the
// shapes the visitor emits (the BoolOp / IfExp merge tail of
// STORE_NAME + LOAD_CONST None + RETURN_VALUE).
//
// Skip rules:
//   - seq nil or len(seq.Blocks) <= 1 — nothing to inline.
//   - candidate has no Label — no jump can name it; the visitor
//     never emits an unlabelled merge block.
//   - candidate has a conditional or backward-jump predecessor —
//     the block must remain a labelled jump target. The pass
//     leaves the seq alone in this case (this is what keeps the
//     BoolOp shape multi-block until resolveJumps linearises it).
//
// The pass is idempotent: a second run after the fixed point is a
// no-op. v0.7.6 calls it once; v0.7.13's full optimize_cfg loop
// runs it together with the rest of the CFG passes.
//
// SOURCE: CPython 3.14 Python/flowgraph.c::inline_small_or_no_lineno_blocks.
func inlineSmallExitBlocks(seq *ir.InstrSeq) {
	if seq == nil || len(seq.Blocks) <= 1 {
		return
	}
	for i := 0; i < len(seq.Blocks); {
		b := seq.Blocks[i]
		if !inlineExitCandidate(b) {
			i++
			continue
		}
		preds, ok := collectInlinablePredecessors(seq, i, b)
		if !ok || len(preds) == 0 {
			i++
			continue
		}
		for _, p := range preds {
			if p.viaJump {
				p.block.Instrs = p.block.Instrs[:len(p.block.Instrs)-1]
			}
			p.block.Instrs = append(p.block.Instrs, cloneInstrs(b.Instrs)...)
		}
		seq.Blocks = append(seq.Blocks[:i], seq.Blocks[i+1:]...)
	}
}

// maxInlineExitInstrs caps the size of a candidate merge block. The
// v0.7.4 BoolOp / IfExp shapes produce 3-instruction tails
// (STORE_NAME, LOAD_CONST None, RETURN_VALUE); the bound is widened
// in v0.7.5+ as new shapes need it.
const maxInlineExitInstrs = 6

// inlinePred describes one predecessor of an inline candidate.
// viaJump records whether the predecessor reaches the candidate via
// JUMP_FORWARD (true) or fall-through (false).
type inlinePred struct {
	block   *ir.Block
	viaJump bool
}

// inlineExitCandidate reports whether b is eligible to be duplicated
// into its predecessors and removed.
func inlineExitCandidate(b *ir.Block) bool {
	if b.Label == ir.NoLabel {
		return false
	}
	n := len(b.Instrs)
	if n == 0 || n > maxInlineExitInstrs {
		return false
	}
	return b.Instrs[n-1].Op == bytecode.RETURN_VALUE
}

// collectInlinablePredecessors returns the predecessors of b at
// seq.Blocks[idx] together with how they reach it. The second return
// is false when at least one predecessor reaches b via a conditional
// or backward jump — in that case the merge block must stay alive.
func collectInlinablePredecessors(seq *ir.InstrSeq, idx int, b *ir.Block) ([]inlinePred, bool) {
	var preds []inlinePred
	for j, p := range seq.Blocks {
		if p == b {
			continue
		}
		if len(p.Instrs) == 0 {
			if j+1 == idx {
				preds = append(preds, inlinePred{block: p})
			}
			continue
		}
		last := p.Instrs[len(p.Instrs)-1]
		if isJumpOp(last.Op) {
			if last.Arg != uint32(b.Label) {
				continue
			}
			if last.Op == bytecode.JUMP_FORWARD {
				preds = append(preds, inlinePred{block: p, viaJump: true})
				continue
			}
			return nil, false
		}
		if isTerminatorOp(last.Op) {
			continue
		}
		if j+1 == idx {
			preds = append(preds, inlinePred{block: p})
		}
	}
	return preds, true
}

// cloneInstrs returns a deep copy of instrs so duplication into
// multiple predecessors does not alias the source slice.
func cloneInstrs(instrs []ir.Instr) []ir.Instr {
	out := make([]ir.Instr, len(instrs))
	copy(out, instrs)
	return out
}
