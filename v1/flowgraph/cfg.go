// Package flowgraph builds a control-flow graph over the v0.6.3
// instruction-sequence IR. CFG construction is read-only at v0.6.4:
// blocks are split at jump-target boundaries, successors are
// recorded, but no instruction is rewritten and Linearize emits the
// original byte stream.
//
// Mutation (block reordering, jump rewriting, peephole) belongs to
// the optimizer — v0.6.8.
//
// SOURCE: CPython 3.14 Python/flowgraph.c (cfg-construction half:
// _PyCfgBuilder, cfg_builder_use_label, cfg_builder_check).
package flowgraph

import (
	"errors"
	"sort"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

// EdgeKind classifies a CFG edge.
type EdgeKind uint8

const (
	// EdgeFallthrough is the implicit edge from one instruction to
	// the next. Conditional jumps have one fallthrough edge plus
	// one jump edge.
	EdgeFallthrough EdgeKind = iota
	// EdgeJump is a control-transfer edge produced by a JUMP_*,
	// POP_JUMP_IF_*, or FOR_ITER instruction.
	EdgeJump
)

// Edge is one CFG edge.
type Edge struct {
	Kind EdgeKind
	Dest *ir.Block
}

// CFG is the read-only control-flow graph for one InstrSeq.
type CFG struct {
	Seq    *ir.InstrSeq
	Blocks []*ir.Block
	Entry  *ir.Block
	Exit   *ir.Block
	Edges  map[uint32][]Edge

	// startCodeUnit maps each block ID to its starting code-unit
	// offset in the original byte stream. Linearize uses this to
	// re-concatenate blocks in original order; Dominators uses it
	// for deterministic traversal.
	startCodeUnit map[uint32]int
}

// Build splits seq.Blocks[0] into basic blocks at every jump-target
// and after every terminator. v0.6.3 always produces a single-block
// InstrSeq, so Build operates on Blocks[0].
//
// Returns an error if the sequence is empty or malformed.
func Build(seq *ir.InstrSeq) (*CFG, error) {
	if seq == nil {
		return nil, errors.New("flowgraph.Build: nil InstrSeq")
	}
	if len(seq.Blocks) == 0 {
		// Empty sequence — one empty block keeps the invariants.
		empty := &ir.Block{}
		return &CFG{
			Seq:           seq,
			Blocks:        []*ir.Block{empty},
			Entry:         empty,
			Exit:          empty,
			Edges:         map[uint32][]Edge{},
			startCodeUnit: map[uint32]int{0: 0},
		}, nil
	}
	flat := seq.Blocks[0]

	// Pass 1: assign each instruction a code-unit offset (the same
	// offset Decode reads from). The offset is the start of the
	// instruction in the original byte stream divided by 2.
	starts := make([]int, len(flat.Instrs))
	cursor := 0
	for i, instr := range flat.Instrs {
		starts[i] = cursor
		cursor += instructionUnits(instr)
	}
	totalUnits := cursor

	// Pass 2: collect target code-unit offsets for every jump.
	targets := map[int]bool{0: true}
	for i, instr := range flat.Instrs {
		if !isJump(instr.Op) {
			continue
		}
		jumpStart := starts[i]
		jumpUnits := instructionUnits(instr)
		next := jumpStart + jumpUnits
		var target int
		switch instr.Op {
		case bytecode.JUMP_BACKWARD:
			target = next - int(instr.Arg)
		default:
			target = next + int(instr.Arg)
		}
		if target < 0 || target > totalUnits {
			return nil, errors.New("flowgraph.Build: jump target out of range")
		}
		targets[target] = true
		// A new block also starts immediately after the jump if the
		// instruction can fall through (conditional jumps and
		// FOR_ITER) or if the instruction is unconditional (so
		// successor analysis is uniform).
		targets[next] = true
	}
	// Block boundaries also follow non-jump terminators
	// (RETURN_VALUE, RAISE_*).
	for i, instr := range flat.Instrs {
		if isTerminator(instr.Op) && !isJump(instr.Op) {
			targets[starts[i]+instructionUnits(instr)] = true
		}
	}

	boundaries := make([]int, 0, len(targets))
	for u := range targets {
		boundaries = append(boundaries, u)
	}
	sort.Ints(boundaries)
	// Map code-unit offset to instruction index in flat.Instrs for
	// quick lookup.
	unitToInstr := map[int]int{}
	for i, off := range starts {
		unitToInstr[off] = i
	}
	unitToInstr[totalUnits] = len(flat.Instrs)

	// Pass 3: emit blocks in boundary order.
	g := &CFG{
		Seq:           seq,
		Edges:         map[uint32][]Edge{},
		startCodeUnit: map[uint32]int{},
	}
	blockByStart := map[int]*ir.Block{}
	var nextID uint32
	for bi := 0; bi < len(boundaries); bi++ {
		start := boundaries[bi]
		var end int
		if bi+1 < len(boundaries) {
			end = boundaries[bi+1]
		} else {
			end = totalUnits
		}
		startIdx, ok := unitToInstr[start]
		if !ok {
			return nil, errors.New("flowgraph.Build: jump target lands inside an instruction")
		}
		endIdx, ok := unitToInstr[end]
		if !ok {
			return nil, errors.New("flowgraph.Build: block end lands inside an instruction")
		}
		if startIdx == endIdx {
			continue
		}
		block := &ir.Block{
			ID:     nextID,
			Instrs: flat.Instrs[startIdx:endIdx],
		}
		nextID++
		g.Blocks = append(g.Blocks, block)
		blockByStart[start] = block
		g.startCodeUnit[block.ID] = start
	}
	if len(g.Blocks) == 0 {
		// All-empty input: a single empty block keeps the graph
		// well-formed.
		empty := &ir.Block{}
		g.Blocks = []*ir.Block{empty}
		blockByStart[0] = empty
		g.startCodeUnit[0] = 0
	}
	g.Entry = g.Blocks[0]
	g.Exit = g.Blocks[len(g.Blocks)-1]

	// Pass 4: edges. For each block, look at its last instruction.
	for bi, block := range g.Blocks {
		if len(block.Instrs) == 0 {
			continue
		}
		last := block.Instrs[len(block.Instrs)-1]
		blockStart := g.startCodeUnit[block.ID]
		// Compute the code-unit offset of the instruction following
		// the last one in the block.
		nextUnit := blockStart
		for _, instr := range block.Instrs {
			nextUnit += instructionUnits(instr)
		}
		var edges []Edge
		fallthrough_ := !isTerminator(last.Op) || isConditionalJump(last.Op) || last.Op == bytecode.FOR_ITER
		if isJump(last.Op) {
			lastUnits := instructionUnits(last)
			lastStart := nextUnit - lastUnits
			var target int
			if last.Op == bytecode.JUMP_BACKWARD {
				target = lastStart + lastUnits - int(last.Arg)
			} else {
				target = lastStart + lastUnits + int(last.Arg)
			}
			if dest, ok := blockByStart[target]; ok {
				edges = append(edges, Edge{Kind: EdgeJump, Dest: dest})
			}
		}
		if fallthrough_ {
			if bi+1 < len(g.Blocks) {
				edges = append(edges, Edge{Kind: EdgeFallthrough, Dest: g.Blocks[bi+1]})
			}
		}
		if len(edges) > 0 {
			g.Edges[block.ID] = edges
		}
	}

	g.Seq.Blocks = g.Blocks
	return g, nil
}

// Linearize concatenates the CFG's blocks back into a single-block
// InstrSeq. Block order is preserved from Build, so the resulting
// sequence reconstructs the original byte stream byte-for-byte when
// passed back through ir.Encode.
func Linearize(g *CFG) *ir.InstrSeq {
	out := ir.NewInstrSeq()
	out.Names = g.Seq.Names
	out.Consts = g.Seq.Consts
	out.Sym = g.Seq.Sym
	out.FirstLineNo = g.Seq.FirstLineNo
	out.OrigLineTable = g.Seq.OrigLineTable
	out.OrigExcTable = g.Seq.OrigExcTable
	flat := out.AddBlock()
	for _, b := range g.Blocks {
		flat.Instrs = append(flat.Instrs, b.Instrs...)
	}
	return out
}

// instructionUnits returns the on-disk size of an instruction in
// code units (one unit = 2 bytes), including any leading
// EXTENDED_ARG words and trailing inline-cache words.
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

// isJump reports whether the opcode has a jump target oparg —
// gocopy's analogue of CPython 3.14's OPCODE_HAS_JUMP. The set is the
// union of IS_UNCONDITIONAL_JUMP_OPCODE (JUMP_FORWARD / JUMP_BACKWARD)
// and IS_CONDITIONAL_JUMP_OPCODE (POP_JUMP_IF_*), plus FOR_ITER which
// flowgraph.c marks HAS_JUMP_FLAG. gocopy does not yet emit JUMP /
// JUMP_NO_INTERRUPT / JUMP_BACKWARD_NO_INTERRUPT, so they are absent
// here; the visitor lowers them to JUMP_FORWARD / JUMP_BACKWARD before
// IR construction.
//
// SOURCE: CPython 3.14 Include/internal/pycore_opcode_utils.h:39
// IS_UNCONDITIONAL_JUMP_OPCODE + line 46 IS_CONDITIONAL_JUMP_OPCODE
// (FOR_ITER's HAS_JUMP_FLAG comes from
// Include/internal/pycore_opcode_metadata.h).
func isJump(op bytecode.Opcode) bool {
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

// isConditionalJump reports whether the opcode is a jump with a
// fallthrough successor — POP_JUMP_IF_* plus FOR_ITER (whose
// fallthrough is the loop body and whose jump target is the exit).
//
// SOURCE: CPython 3.14 Include/internal/pycore_opcode_utils.h:46
// IS_CONDITIONAL_JUMP_OPCODE (FOR_ITER added because flowgraph.c
// treats it as a conditional branch — see Python/flowgraph.c:516
// fallthrough computation).
func isConditionalJump(op bytecode.Opcode) bool {
	switch op {
	case bytecode.POP_JUMP_IF_FALSE,
		bytecode.POP_JUMP_IF_TRUE,
		bytecode.POP_JUMP_IF_NONE,
		bytecode.POP_JUMP_IF_NOT_NONE,
		bytecode.FOR_ITER:
		return true
	}
	return false
}

// isTerminator reports whether the opcode ends a basic block with no
// fallthrough successor — i.e. CPython's
// `IS_SCOPE_EXIT_OPCODE(op) || IS_UNCONDITIONAL_JUMP_OPCODE(op)`,
// which is the predicate `basicblock_nofallthrough` evaluates per
// block. gocopy does not yet emit RERAISE / JUMP / JUMP_NO_INTERRUPT
// / JUMP_BACKWARD_NO_INTERRUPT, so they are absent here.
//
// SOURCE: CPython 3.14 Include/internal/pycore_opcode_utils.h:52
// IS_SCOPE_EXIT_OPCODE + line 39 IS_UNCONDITIONAL_JUMP_OPCODE,
// combined per Python/flowgraph.c:239 basicblock_nofallthrough.
func isTerminator(op bytecode.Opcode) bool {
	switch op {
	case bytecode.RETURN_VALUE,
		bytecode.RAISE_VARARGS,
		bytecode.JUMP_FORWARD,
		bytecode.JUMP_BACKWARD:
		return true
	}
	return false
}
