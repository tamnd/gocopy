package assemble

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/flowgraph"
	"github.com/tamnd/gocopy/v1/ir"
)

// StackDepth computes the maximum stack depth the CFG can reach
// using BFS abstract interpretation, mirroring CPython 3.14
// Python/assemble.c::stackdepth_walk.
//
// For each block we track an incoming stack depth. We start the
// entry block at depth 0 and walk forward: each instruction's
// effect (from OpMeta) advances the depth; conditional branches
// propagate depth to both successors; jumps propagate to the
// jump target. A successor whose recorded incoming depth would
// grow gets re-enqueued.
//
// Returns 0 for an empty CFG.
func StackDepth(g *flowgraph.CFG) int32 {
	if g == nil || len(g.Blocks) == 0 || g.Entry == nil {
		return 0
	}
	startDepth := map[uint32]int32{g.Entry.ID: 0}
	maxDepth := int32(0)
	queue := []*ir.Block{g.Entry}
	for len(queue) > 0 {
		b := queue[0]
		queue = queue[1:]
		depth, ok := startDepth[b.ID]
		if !ok {
			continue
		}
		blockMax := depth
		cur := depth
		for _, instr := range b.Instrs {
			eff := stackEffect(instr, false)
			// For conditional branches, the "branch" effect can
			// differ from the fall-through effect; record the max
			// depth seen at any branch target.
			branchEff := stackEffect(instr, true)
			peakAfterBranch := cur + int32(branchEff)
			if peakAfterBranch > blockMax {
				blockMax = peakAfterBranch
			}
			cur += int32(eff)
			if cur > blockMax {
				blockMax = cur
			}
			// Propagate branch-arm depth to jump targets.
			if isJump(instr.Op) {
				for _, edge := range g.Edges[b.ID] {
					if edge.Kind == flowgraph.EdgeJump && edge.Dest != nil {
						propagate(startDepth, edge.Dest.ID, cur+int32(branchEff)-int32(eff), &queue, edge.Dest)
					}
				}
			}
		}
		if blockMax > maxDepth {
			maxDepth = blockMax
		}
		// Propagate fallthrough.
		for _, edge := range g.Edges[b.ID] {
			if edge.Kind == flowgraph.EdgeFallthrough && edge.Dest != nil {
				propagate(startDepth, edge.Dest.ID, cur, &queue, edge.Dest)
			}
		}
	}
	return maxDepth
}

func propagate(startDepth map[uint32]int32, id uint32, depth int32, queue *[]*ir.Block, b *ir.Block) {
	cur, ok := startDepth[id]
	if !ok || depth > cur {
		startDepth[id] = depth
		*queue = append(*queue, b)
	}
}

// stackEffect returns the stack delta of one instruction.
// `branch` is the CPython sense: true asks for the effect along
// the branch-taken arm; false asks for the fallthrough arm. The
// two arms differ only for FOR_ITER (push iter-next on
// fallthrough, leave the iterator on the stack on branch).
//
// For fixed-effect ops it returns OpMeta.StackEff (same on both
// arms except FOR_ITER). For variable-effect ops it dispatches
// by opcode and oparg, mirroring CPython's stack_effect rules.
func stackEffect(instr ir.Instr, branch bool) int8 {
	meta := bytecode.MetaOf(instr.Op)
	if !meta.StackVar {
		if instr.Op == bytecode.FOR_ITER && branch {
			return 0
		}
		return meta.StackEff
	}
	arg := int(instr.Arg)
	switch instr.Op {
	case bytecode.BUILD_LIST,
		bytecode.BUILD_TUPLE,
		bytecode.BUILD_SET:
		return int8(1 - arg)
	case bytecode.BUILD_MAP:
		return int8(1 - 2*arg)
	case bytecode.CALL:
		// CALL n: pops n + 2 (callable, self/null, n args), pushes 1.
		return int8(-(arg + 1))
	case bytecode.LOAD_GLOBAL:
		// Low bit of oparg pushes an extra NULL.
		if arg&1 != 0 {
			return 2
		}
		return 1
	case bytecode.LOAD_ATTR:
		// Low bit pushes a self for METHOD-style.
		if arg&1 != 0 {
			return 1
		}
		return 0
	}
	return meta.StackEff
}

// isJump matches compiler/flowgraph/cfg.go's isJump.
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
