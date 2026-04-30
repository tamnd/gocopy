package flowgraph

import (
	"fmt"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// resolveJumps rewrites every jump instruction's Arg from a
// LabelID into the byte-distance form ir.Encode and
// flowgraph.Build consume, then linearises the multi-block IR
// into a single flat block.
//
// Mirrors CPython 3.14 Python/flowgraph.c::resolve_unconditional_jumps
// plus the relocation half of _PyAssemble_ResolveJumps.
//
// Pre: every block in seq.Blocks may carry Label > 0 allocated by
// seq.AllocLabel / BindLabel; every jump instruction holds its
// target as Arg = uint32(LabelID). Both invariants hold by
// construction for visitor-emitted IR.
//
// At v0.7.7 both forward jumps and backward jumps are supported.
// JUMP_BACKWARD's Arg becomes (jumpEnd - target); every other jump
// opcode's Arg becomes (target - jumpEnd). A JUMP_BACKWARD whose
// target is at or after jumpEnd, or any other jump opcode whose
// target is before jumpEnd, panics — both encode invariants the
// visitor and CFG construction code never violate. A panic means a
// bug upstream, not a valid CFG.
//
// The pass is a no-op when seq is nil, has at most one block, or
// has no labelled blocks at all (the decoder / assembler
// round-trip path stays unchanged).
//
// SOURCE: CPython 3.14 Python/flowgraph.c::resolve_unconditional_jumps,
// Python/assemble.c::_PyAssemble_ResolveJumps.
func resolveJumps(seq *ir.InstrSeq) {
	if seq == nil || len(seq.Blocks) <= 1 {
		return
	}
	if !hasLabelledBlock(seq) {
		return
	}

	starts := make([]int, len(seq.Blocks))
	cursor := 0
	for i, b := range seq.Blocks {
		starts[i] = cursor
		for _, instr := range b.Instrs {
			cursor += instructionUnits(instr)
		}
	}

	labelStart := map[ir.LabelID]int{}
	for i, b := range seq.Blocks {
		if b.Label != ir.NoLabel {
			labelStart[b.Label] = starts[i]
		}
	}

	for i, b := range seq.Blocks {
		off := starts[i]
		for k := range b.Instrs {
			instr := &b.Instrs[k]
			units := instructionUnits(*instr)
			if !isJumpOp(instr.Op) {
				off += units
				continue
			}
			target, ok := labelStart[ir.LabelID(instr.Arg)]
			if !ok {
				off += units
				continue
			}
			jumpEnd := off + units
			if instr.Op == bytecode.JUMP_BACKWARD {
				if target >= jumpEnd {
					panic(fmt.Sprintf("optimize.resolveJumps: JUMP_BACKWARD with non-backward target at block %d instr %d (target=%d jumpEnd=%d)", i, k, target, jumpEnd))
				}
				instr.Arg = uint32(jumpEnd - target)
			} else {
				if target < jumpEnd {
					panic(fmt.Sprintf("optimize.resolveJumps: forward-jump opcode %d with backward target at block %d instr %d (target=%d jumpEnd=%d)", instr.Op, i, k, target, jumpEnd))
				}
				instr.Arg = uint32(target - jumpEnd)
			}
			off += units
		}
	}

	flat := &ir.Block{ID: 0}
	for _, b := range seq.Blocks {
		flat.Instrs = append(flat.Instrs, b.Instrs...)
	}
	seq.Blocks = []*ir.Block{flat}
}

// hasLabelledBlock reports whether any block carries a Label.
// Without one, no jump in the seq can be a LabelID — the input is
// already in flat / decoded shape.
func hasLabelledBlock(seq *ir.InstrSeq) bool {
	for _, b := range seq.Blocks {
		if b.Label != ir.NoLabel {
			return true
		}
	}
	return false
}

// isJumpOp / isTerminatorOp are local aliases for the merged-package
// equivalents in cfg.go (isJump / isTerminator). The semantics differ
// in one place: this *Op variant of isTerminator excludes
// RAISE_VARARGS (resolve_jumps inlines RETURN_VALUE-tail blocks only,
// not raise-tail blocks), so we can't drop in cfg.go's directly. Once
// spec 1573 Phase D's optimize_basic_block lands the inliner gains
// RAISE_VARARGS coverage and these aliases collapse.
func isJumpOp(op bytecode.Opcode) bool { return isJump(op) }

func isTerminatorOp(op bytecode.Opcode) bool {
	switch op {
	case bytecode.RETURN_VALUE,
		bytecode.JUMP_FORWARD,
		bytecode.JUMP_BACKWARD:
		return true
	}
	return false
}
