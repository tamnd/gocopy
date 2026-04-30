package flowgraph

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

// inlineSmallOrNoLinenoBlocks duplicates a small terminator block (one
// ending in RETURN_VALUE, with at most maxInlineExitInstrs
// instructions) into every predecessor that reaches it via an
// unconditional JUMP_FORWARD, replacing the JUMP_FORWARD with the
// inlined tail. Predecessors that reach the candidate via
// fall-through are LEFT alone — CPython's
// basicblock_inline_small_or_no_lineno_blocks only acts on blocks
// that end with an unconditional jump, so the structural depth at
// the merge point matches CPython exactly: a block falling through
// to a small RETURN_VALUE merge keeps its fallthrough edge, and the
// merge block survives.
//
// The merge block is removed from seq.Blocks only when every
// predecessor was inlined (i.e. no fall-through pred remains). When
// a fall-through pred survives, the merge block stays alive and
// resolveJumps later linearises it as the contiguous bytecode tail.
//
// Why fall-through must NOT be inlined:
// optimize_load_fast (CPython 3.14 Python/flowgraph.c:2776) is
// per-block; refs still on the operand stack at end-of-block are
// flagged REF_UNCONSUMED, which suppresses promotion of the
// LOAD_FAST that produced them. Inlining the RETURN_VALUE into a
// fall-through pred would consume the ref within the same block and
// erroneously promote the LOAD_FAST to LOAD_FAST_BORROW — breaking
// byte parity for ternary IfExp shapes whose else branch reaches
// the merge by fall-through.
//
// At v0.7.10.4 this is a narrow specialisation of CPython 3.14
// Python/flowgraph.c::inline_small_or_no_lineno_blocks for the
// shapes the visitor emits (the BoolOp / IfExp merge tail of
// STORE_NAME + LOAD_CONST None + RETURN_VALUE, plus the IfExp
// JUMP_FORWARD → end:RETURN_VALUE shape).
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
// SOURCE: CPython 3.14 Python/flowgraph.c:1211
// basicblock_inline_small_or_no_lineno_blocks +
// Python/flowgraph.c:1245 inline_small_or_no_lineno_blocks.
func inlineSmallOrNoLinenoBlocks(seq *ir.InstrSeq) {
	if seq == nil || len(seq.Blocks) <= 1 {
		return
	}
	for i := 0; i < len(seq.Blocks); {
		b := seq.Blocks[i]
		if !inlineExitCandidate(b) {
			i++
			continue
		}
		jumpPreds, hasFallthrough, ok := collectInlinablePredecessors(seq, i, b)
		if !ok || len(jumpPreds) == 0 {
			i++
			continue
		}
		for _, p := range jumpPreds {
			p.block.Instrs = p.block.Instrs[:len(p.block.Instrs)-1]
			p.block.Instrs = append(p.block.Instrs, cloneInstrs(b.Instrs)...)
		}
		if hasFallthrough {
			i++
			continue
		}
		seq.Blocks = append(seq.Blocks[:i], seq.Blocks[i+1:]...)
	}
}

// maxInlineExitInstrs caps the size of a candidate merge block. The
// v0.7.4 BoolOp / IfExp shapes produce 3-instruction tails
// (STORE_NAME, LOAD_CONST None, RETURN_VALUE); the bound is widened
// in v0.7.5+ as new shapes need it.
const maxInlineExitInstrs = 6

// inlinePred describes one JUMP_FORWARD predecessor of an inline
// candidate. (Fall-through predecessors are reported via the
// hasFallthrough bool from collectInlinablePredecessors and are not
// inlined.)
type inlinePred struct {
	block *ir.Block
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

// collectInlinablePredecessors classifies the predecessors of b at
// seq.Blocks[idx] by how they reach it. JUMP_FORWARD preds are
// returned in jumpPreds and will be inlined; a fall-through pred
// sets hasFallthrough = true. The third return is false when at
// least one predecessor reaches b via a conditional or backward
// jump — in that case the merge block must stay alive AND keep its
// labelled identity (no inlining at all).
func collectInlinablePredecessors(seq *ir.InstrSeq, idx int, b *ir.Block) (jumpPreds []inlinePred, hasFallthrough bool, ok bool) {
	for j, p := range seq.Blocks {
		if p == b {
			continue
		}
		if len(p.Instrs) == 0 {
			if j+1 == idx {
				hasFallthrough = true
			}
			continue
		}
		last := p.Instrs[len(p.Instrs)-1]
		if isJump(last.Op) {
			if last.Arg != uint32(b.Label) {
				continue
			}
			if last.Op == bytecode.JUMP_FORWARD {
				jumpPreds = append(jumpPreds, inlinePred{block: p})
				continue
			}
			return nil, false, false
		}
		if isTerminator(last.Op) {
			continue
		}
		if j+1 == idx {
			hasFallthrough = true
		}
	}
	return jumpPreds, hasFallthrough, true
}

// cloneInstrs returns a deep copy of instrs so duplication into
// multiple predecessors does not alias the source slice.
func cloneInstrs(instrs []ir.Instr) []ir.Instr {
	out := make([]ir.Instr, len(instrs))
	copy(out, instrs)
	return out
}
