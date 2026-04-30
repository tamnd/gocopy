package flowgraph

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// propagateLineNumbers fills NO_LOCATION slots in the line table by
// inheriting locations from predecessors. Mirrors CPython 3.14
// Python/flowgraph.c:3616 propagate_line_numbers verbatim:
//
//	"If an instruction has no line number, but it's predecessor in
//	 the BB does, then copy the line number. If a successor block
//	 has no line number, and only one predecessor, then inherit the
//	 line number. This ensures that all exit blocks (with one
//	 predecessor) receive a line number. Also reduces the size of
//	 the line number table, but has no impact on the generated line
//	 number events."
//
// Three propagations:
//
//  1. Within a block: the loc of each NO_LOCATION instruction
//     becomes the loc of the prior non-NO_LOCATION instruction.
//  2. Block fall-through: the trailing loc propagates into the
//     successor block's first instruction when that successor has
//     exactly one predecessor and its first instruction is
//     NO_LOCATION.
//  3. Block jump: same propagation into a jump target with exactly
//     one predecessor.
//
// CPython's NO_LOCATION sentinel is line == -1; gocopy's
// equivalent is bytecode.Loc{} (zero-valued — Line == 0). The
// codegen-level NO_LOCATION emits (e.g. addReturnAtEnd, MAKE_CELL
// prologue) all carry Line == 0, which this pass backfills.
//
// SOURCE: CPython 3.14 Python/flowgraph.c:3616 propagate_line_numbers.
func propagateLineNumbers(seq *ir.InstrSeq) {
	if seq == nil || len(seq.Blocks) == 0 {
		return
	}
	preds := blockPredecessorCounts(seq)
	for bi, b := range seq.Blocks {
		if len(b.Instrs) == 0 {
			continue
		}
		var prev bytecode.Loc
		havePrev := false
		for k := range b.Instrs {
			loc := b.Instrs[k].Loc
			if isNoLocation(loc) {
				if havePrev {
					b.Instrs[k].Loc = prev
				}
				continue
			}
			prev = loc
			havePrev = true
		}
		if !havePrev {
			continue
		}
		last := b.Instrs[len(b.Instrs)-1]
		// Fall-through successor.
		if !isTerminatorOp(last.Op) {
			if bi+1 < len(seq.Blocks) {
				succ := seq.Blocks[bi+1]
				if preds[succ.ID] == 1 && len(succ.Instrs) > 0 && isNoLocation(succ.Instrs[0].Loc) {
					succ.Instrs[0].Loc = prev
				}
			}
		}
		// Jump target.
		if isJumpOp(last.Op) {
			if target := jumpTargetBlock(seq, ir.LabelID(last.Arg)); target != nil {
				if preds[target.ID] == 1 && len(target.Instrs) > 0 && isNoLocation(target.Instrs[0].Loc) {
					target.Instrs[0].Loc = prev
				}
			}
		}
	}
}

// isNoLocation reports whether loc is gocopy's NO_LOCATION sentinel
// (a zero-valued Loc — Line == 0). codegen emits this for synthetic
// instructions that should inherit their line number from a
// predecessor (the CPython convention is line == -1, but gocopy's
// integral types use 0 as the sentinel because the line table never
// names line 0 anyway).
func isNoLocation(loc bytecode.Loc) bool {
	return loc.Line == 0
}

// blockPredecessorCounts builds a map of block ID → predecessor
// count for seq. Predecessors are the prior block (when its last
// instr falls through) plus every block whose terminating jump
// targets this block's label.
func blockPredecessorCounts(seq *ir.InstrSeq) map[uint32]int {
	preds := make(map[uint32]int, len(seq.Blocks))
	// Entry block has an implicit predecessor (the frame entry) so
	// propagation never overwrites its first loc.
	if len(seq.Blocks) > 0 {
		preds[seq.Blocks[0].ID] = 1
	}
	for i, b := range seq.Blocks {
		if len(b.Instrs) == 0 {
			// Empty block falls through unconditionally.
			if i+1 < len(seq.Blocks) {
				preds[seq.Blocks[i+1].ID]++
			}
			continue
		}
		last := b.Instrs[len(b.Instrs)-1]
		if !isTerminatorOp(last.Op) {
			if i+1 < len(seq.Blocks) {
				preds[seq.Blocks[i+1].ID]++
			}
		}
		if isJumpOp(last.Op) {
			if target := jumpTargetBlock(seq, ir.LabelID(last.Arg)); target != nil {
				preds[target.ID]++
			}
		}
	}
	return preds
}

// jumpTargetBlock returns the block in seq labelled with id, or nil
// when no such block exists. Jumps point at LabelIDs at this stage
// of the pipeline (resolveJumps hasn't run).
func jumpTargetBlock(seq *ir.InstrSeq, id ir.LabelID) *ir.Block {
	if id == ir.NoLabel {
		return nil
	}
	for _, b := range seq.Blocks {
		if b.Label == id {
			return b
		}
	}
	return nil
}
