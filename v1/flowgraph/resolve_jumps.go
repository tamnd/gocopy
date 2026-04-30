package flowgraph

import (
	"fmt"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

// resolveJumps rewrites every jump instruction's Arg from a
// LabelID into the byte-distance form ir.Encode and
// flowgraph.Build consume, then linearises the multi-block IR
// into a single flat block.
//
// Mirrors the label-resolution + offset-computation half of
// CPython's assembly pipeline:
//   - Python/instruction_sequence.c:86 _PyInstructionSequence_ApplyLabelMap
//     replaces each jump's i_oparg label-id with the absolute
//     instruction index of the labelled target.
//   - Python/assemble.c:674 resolve_jump_offsets then converts
//     those absolute indices into byte distances relative to the
//     instruction *after* the jump (offset += isize before the
//     fixup), iterating until EXTENDED_ARG sizing converges.
//
// gocopy collapses both into one pass because the flowgraph IR
// keeps Block boundaries instead of CPython's flat instr_sequence:
// we walk seq.Blocks twice — once to record each block's start
// offset (the equivalent of ApplyLabelMap building s_labelmap),
// once to rewrite each jump's Arg from LabelID into the signed
// byte distance (the equivalent of resolve_jump_offsets' offset
// fixup loop). gocopy does not need the recompile loop yet because
// no opcode emits EXTENDED_ARG today.
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
// The pass linearises every multi-block sequence, even when no block
// carries a label. CPython's Python/assemble.c:778
// _PyAssemble_MakeCodeObject walks the resolved instr_sequence and
// writes a single contiguous byte stream — there is no
// "skip-when-no-jumps" fast path. Multi-block sequences without
// labels arise when codegen plants a fresh trailing block (e.g.
// Python/codegen.c:6473 _PyCodegen_AddReturnAtEnd's implicit-return
// tail), and downstream passes like ir.Encode walk seq.Blocks;
// without flattening here the trailing block survives as a separate
// block but flowgraph.Build — which operates on seq.Blocks[0] alone —
// would discard its instructions.
//
// SOURCE: CPython 3.14 Python/instruction_sequence.c:86
// _PyInstructionSequence_ApplyLabelMap +
// Python/assemble.c:674 resolve_jump_offsets.
func resolveJumps(seq *ir.InstrSeq) {
	if seq == nil || len(seq.Blocks) <= 1 {
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
			if !isJump(instr.Op) {
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

