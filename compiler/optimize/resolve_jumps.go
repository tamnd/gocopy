package optimize

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

// instructionUnits returns the on-disk size of an instruction in
// code units (one unit = 2 bytes), including any leading
// EXTENDED_ARG words and trailing inline-cache words. Mirrors
// flowgraph.instructionUnits byte-for-byte.
func instructionUnits(instr ir.Instr) int {
	units := 1
	if instr.Arg > 0xFF {
		if instr.Arg > 0xFFFF {
			units++
			if instr.Arg > 0xFFFFFF {
				units++
			}
		}
		units++
	}
	units += int(bytecode.CacheSize[instr.Op])
	return units
}

// isJumpOp reports whether op transfers control to a non-fallthrough
// successor. Mirrors flowgraph.isJump.
func isJumpOp(op bytecode.Opcode) bool {
	switch op {
	case bytecode.JUMP_FORWARD,
		bytecode.JUMP_BACKWARD,
		bytecode.POP_JUMP_IF_FALSE,
		bytecode.POP_JUMP_IF_TRUE,
		bytecode.POP_JUMP_IF_NONE,
		bytecode.POP_JUMP_IF_NOT_NONE,
		bytecode.FOR_ITER:
		return true
	}
	return false
}

// isTerminatorOp reports whether op ends a basic block.
// Conditional jumps are jumps but not terminators here — they have
// fallthrough successors.
func isTerminatorOp(op bytecode.Opcode) bool {
	switch op {
	case bytecode.RETURN_VALUE,
		bytecode.JUMP_FORWARD,
		bytecode.JUMP_BACKWARD:
		return true
	}
	return false
}
