package flowgraph

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// instrFlag is the bitset CPython 3.14 uses on each LOAD_FAST(_LOAD_FAST)
// instruction during optimize_load_fast's per-block scan.
//
// MIRRORS: Python/flowgraph.c:2696 LoadFastInstrFlag.
type instrFlag uint8

const (
	// supportKilled — a STORE_FAST killed the local while the
	// borrowed ref was still on the operand stack.
	supportKilled instrFlag = 1 << iota
	// storedAsLocal — the loaded reference is itself stored into a
	// local frame slot (STORE_FAST), which would extend the
	// borrowed lifetime beyond the supporting reference.
	storedAsLocal
	// refUnconsumed — the loaded reference is still on the operand
	// stack at the end of the basic block, escaping the per-block
	// borrow lifetime.
	refUnconsumed
)

// notLocal is the sentinel ref.local for a value that did not
// originate from a LOAD_FAST(_LOAD_FAST). Mirrors CPython's
// NOT_LOCAL (-1).
const notLocal = -1

// dummyInstr is the sentinel ref.instr for a synthetic ref pushed
// to fill in the operand stack at block entry. Mirrors CPython's
// DUMMY_INSTR (-1).
const dummyInstr = -1

// ref is one entry on the abstract operand stack: it remembers the
// instruction index that produced the value and (for LOAD_FAST
// products) the local slot that supports the borrow.
//
// MIRRORS: Python/flowgraph.c:2691 ref.
type ref struct {
	instr int
	local int
}

// OptimizeLoadFast strength-reduces LOAD_FAST and LOAD_FAST_LOAD_FAST
// into the borrowed-reference variants LOAD_FAST_BORROW and
// LOAD_FAST_BORROW_LOAD_FAST_BORROW where the per-block lifetime
// analysis proves the borrow is safe.
//
// MIRRORS: Python/flowgraph.c:2776 optimize_load_fast.
//
// The pass walks every basic block once via DFS from the entry. For
// each block:
//
//  1. The instr_flags array is reset to zero.
//  2. The ref stack is reset and primed with dummy refs equal to the
//     block's startDepth so abstract pops don't underflow.
//  3. Each instruction is simulated against the ref stack. STORE_FAST
//     kills locals; opcodes that consume references pop them; opcodes
//     that produce values push NOT_LOCAL refs.
//  4. At end of block, any refs still on the stack mark their source
//     instruction with refUnconsumed.
//  5. Every LOAD_FAST whose flag mask is zero is promoted to
//     LOAD_FAST_BORROW; every LOAD_FAST_LOAD_FAST whose flag mask is
//     zero is promoted to LOAD_FAST_BORROW_LOAD_FAST_BORROW.
//
// The pass operates over the visitor's labelled multi-block IR
// directly. Successor edges are recovered locally from each block's
// last instruction (jump target via Label, fallthrough via the next
// block in source order).
func OptimizeLoadFast(seq *ir.InstrSeq) {
	if seq == nil || len(seq.Blocks) == 0 {
		return
	}

	labelToBlock := map[ir.LabelID]*ir.Block{}
	for _, b := range seq.Blocks {
		if b == nil {
			continue
		}
		if b.Label != 0 {
			labelToBlock[b.Label] = b
		}
	}

	startDepth := map[uint32]int{}
	visited := map[uint32]bool{}

	var stack []*ir.Block
	entry := seq.Blocks[0]
	startDepth[entry.ID] = 0
	visited[entry.ID] = true
	stack = append(stack, entry)

	pushBlock := func(target *ir.Block, depth int) {
		if target == nil {
			return
		}
		if visited[target.ID] {
			return
		}
		visited[target.ID] = true
		startDepth[target.ID] = depth
		stack = append(stack, target)
	}

	for len(stack) > 0 {
		block := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		flags := make([]instrFlag, len(block.Instrs))
		var refs []ref
		for i := 0; i < startDepth[block.ID]; i++ {
			refs = append(refs, ref{instr: dummyInstr, local: notLocal})
		}

		for i, instr := range block.Instrs {
			switch instr.Op {
			case bytecode.LOAD_FAST:
				refs = append(refs, ref{instr: i, local: int(instr.Arg)})

			case bytecode.LOAD_FAST_LOAD_FAST:
				hi := int(instr.Arg >> 4)
				lo := int(instr.Arg & 15)
				refs = append(refs, ref{instr: i, local: hi})
				refs = append(refs, ref{instr: i, local: lo})

			case bytecode.LOAD_FAST_BORROW:
				// Already promoted (e.g. v0.7.10.3 carry-over input).
				// Track on the stack but no further promotion.
				refs = append(refs, ref{instr: i, local: int(instr.Arg)})

			case bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW:
				hi := int(instr.Arg >> 4)
				lo := int(instr.Arg & 15)
				refs = append(refs, ref{instr: i, local: hi})
				refs = append(refs, ref{instr: i, local: lo})

			case bytecode.STORE_FAST:
				r, ok := popRef(&refs)
				if !ok {
					break
				}
				storeLocal(flags, refs, int(instr.Arg), r)

			case bytecode.STORE_FAST_LOAD_FAST:
				// STORE half.
				if r, ok := popRef(&refs); ok {
					storeLocal(flags, refs, int(instr.Arg>>4), r)
				}
				// LOAD half.
				refs = append(refs, ref{instr: i, local: int(instr.Arg & 15)})

			case bytecode.STORE_FAST_STORE_FAST:
				if r, ok := popRef(&refs); ok {
					storeLocal(flags, refs, int(instr.Arg>>4), r)
				}
				if r, ok := popRef(&refs); ok {
					storeLocal(flags, refs, int(instr.Arg&15), r)
				}

			case bytecode.COPY:
				idx := len(refs) - int(instr.Arg)
				if idx >= 0 && idx < len(refs) {
					r := refs[idx]
					refs = append(refs, r)
				} else {
					refs = append(refs, ref{instr: i, local: notLocal})
				}

			case bytecode.FOR_ITER:
				if target, ok := jumpTarget(instr, labelToBlock); ok {
					pushBlock(target, len(refs)+1)
				}
				refs = append(refs, ref{instr: i, local: notLocal})

			case bytecode.LOAD_ATTR:
				// pop self; push attr; if method bit set, also re-push self.
				self, _ := popRef(&refs)
				refs = append(refs, ref{instr: i, local: notLocal})
				if instr.Arg&1 != 0 {
					refs = append(refs, ref{instr: self.instr, local: self.local})
				}

			case bytecode.SET_FUNCTION_ATTRIBUTE:
				// pop attr-value; pop func; push func (TOS-2 passes through).
				_, _ = popRef(&refs)
				tos, _ := popRef(&refs)
				refs = append(refs, tos)

			default:
				num_popped, num_pushed, ok := numPoppedPushed(instr.Op, instr.Arg)
				if !ok {
					// Unknown opcode — bail out conservatively for this
					// block: clear all flags so nothing in this block
					// gets promoted. Mirrors CPython's behaviour when
					// an unsupported opcode would invalidate the
					// abstract trace.
					for k := range flags {
						flags[k] = supportKilled | storedAsLocal | refUnconsumed
					}
					goto blockDone
				}
				if hasJumpTarget(instr.Op) {
					if target, ok := jumpTarget(instr, labelToBlock); ok {
						pushBlock(target, len(refs)-num_popped+num_pushed)
					}
				}
				for range num_popped {
					popRef(&refs)
				}
				for range num_pushed {
					refs = append(refs, ref{instr: i, local: notLocal})
				}
			}
		}

		// Push fallthrough successor.
		if len(block.Instrs) > 0 {
			last := block.Instrs[len(block.Instrs)-1]
			if !isUnconditionalJump(last.Op) && !isScopeExit(last.Op) {
				blockIdx := -1
				for k, b := range seq.Blocks {
					if b == block {
						blockIdx = k
						break
					}
				}
				if blockIdx >= 0 && blockIdx+1 < len(seq.Blocks) {
					pushBlock(seq.Blocks[blockIdx+1], len(refs))
				}
			}
		} else if len(seq.Blocks) > 0 {
			blockIdx := -1
			for k, b := range seq.Blocks {
				if b == block {
					blockIdx = k
					break
				}
			}
			if blockIdx >= 0 && blockIdx+1 < len(seq.Blocks) {
				pushBlock(seq.Blocks[blockIdx+1], len(refs))
			}
		}

		// Mark refs still on the operand stack as escaping the block.
		for _, r := range refs {
			if r.instr >= 0 && r.instr < len(flags) {
				flags[r.instr] |= refUnconsumed
			}
		}

	blockDone:
		// Promote unflagged LOAD_FAST(_LOAD_FAST) to the borrow form.
		for k := range block.Instrs {
			if flags[k] != 0 {
				continue
			}
			switch block.Instrs[k].Op {
			case bytecode.LOAD_FAST:
				block.Instrs[k].Op = bytecode.LOAD_FAST_BORROW
			case bytecode.LOAD_FAST_LOAD_FAST:
				block.Instrs[k].Op = bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW
			}
		}
	}
}

// killLocal sets supportKilled on every load instruction whose ref
// is currently on the operand stack and whose local matches.
//
// MIRRORS: Python/flowgraph.c:2707 kill_local.
func killLocal(flags []instrFlag, refs []ref, local int) {
	for _, r := range refs {
		if r.local == local && r.instr >= 0 && r.instr < len(flags) {
			flags[r.instr] |= supportKilled
		}
	}
}

// storeLocal kills any ref with the same local slot, then marks
// the popped ref's source instruction as storedAsLocal (unless the
// ref was a synthetic dummy from block-entry priming).
//
// MIRRORS: Python/flowgraph.c:2718 store_local.
func storeLocal(flags []instrFlag, refs []ref, local int, r ref) {
	killLocal(flags, refs, local)
	if r.instr != dummyInstr && r.instr >= 0 && r.instr < len(flags) {
		flags[r.instr] |= storedAsLocal
	}
}

// loadFastPushBlock primes target's startDepth and marks it visited
// so the DFS visits it exactly once.
//
// MIRRORS: Python/flowgraph.c:2728 load_fast_push_block.
//
// (Implemented inline as the closure pushBlock inside OptimizeLoadFast
// because Go closures over the local visited / startDepth maps and
// stack are clearer than a free-standing function with five
// parameters.)

// popRef removes and returns the top of the abstract operand stack.
// Returns false on underflow (which can happen when the visitor's
// emitted IR is structurally incomplete; the caller treats it as a
// no-op for that pop).
func popRef(refs *[]ref) (ref, bool) {
	if len(*refs) == 0 {
		return ref{}, false
	}
	r := (*refs)[len(*refs)-1]
	*refs = (*refs)[:len(*refs)-1]
	return r, true
}

// jumpTarget resolves a jump instruction's Arg (LabelID at this
// pipeline stage) into the target block.
func jumpTarget(instr ir.Instr, m map[ir.LabelID]*ir.Block) (*ir.Block, bool) {
	t, ok := m[ir.LabelID(instr.Arg)]
	return t, ok
}

// hasJumpTarget reports whether the opcode carries a jump-target
// oparg at this pipeline stage. Matches the OpJump bit in opmeta.
func hasJumpTarget(op bytecode.Opcode) bool {
	return bytecode.MetaOf(op).Flags&bytecode.OpJump != 0
}

// isUnconditionalJump reports whether the opcode unconditionally
// transfers control. Mirrors CPython's IS_UNCONDITIONAL_JUMP_OPCODE.
func isUnconditionalJump(op bytecode.Opcode) bool {
	switch op {
	case bytecode.JUMP_FORWARD, bytecode.JUMP_BACKWARD:
		return true
	}
	return false
}

// isScopeExit reports whether the opcode unwinds the current frame.
// Mirrors CPython's IS_SCOPE_EXIT_OPCODE.
func isScopeExit(op bytecode.Opcode) bool {
	switch op {
	case bytecode.RETURN_VALUE:
		return true
	}
	return false
}

// numPoppedPushed returns the (popped, pushed) pair for the opcodes
// the gocopy visitor currently emits at this pipeline stage. The
// counts mirror CPython's _PyOpcode_num_popped / _PyOpcode_num_pushed
// for each opcode; opcodes the visitor never produces return ok=false.
//
// MIRRORS: Python/opcode_metadata.h _PyOpcode_num_popped /
// _PyOpcode_num_pushed (subset).
func numPoppedPushed(op bytecode.Opcode, oparg uint32) (popped, pushed int, ok bool) {
	switch op {
	case bytecode.NOP, bytecode.NOT_TAKEN, bytecode.RESUME, bytecode.MAKE_CELL,
		bytecode.COPY_FREE_VARS:
		return 0, 0, true

	case bytecode.LOAD_CONST, bytecode.LOAD_SMALL_INT, bytecode.LOAD_NAME,
		bytecode.LOAD_DEREF, bytecode.PUSH_NULL:
		return 0, 1, true

	case bytecode.LOAD_GLOBAL:
		return 0, 1 + int(oparg&1), true

	case bytecode.POP_TOP, bytecode.STORE_NAME, bytecode.STORE_GLOBAL,
		bytecode.STORE_DEREF, bytecode.END_FOR, bytecode.POP_ITER,
		bytecode.RETURN_VALUE,
		bytecode.POP_JUMP_IF_FALSE, bytecode.POP_JUMP_IF_TRUE,
		bytecode.POP_JUMP_IF_NONE, bytecode.POP_JUMP_IF_NOT_NONE:
		return 1, 0, true

	case bytecode.TO_BOOL, bytecode.UNARY_INVERT, bytecode.UNARY_NEGATIVE,
		bytecode.UNARY_NOT, bytecode.GET_ITER, bytecode.MAKE_FUNCTION:
		return 1, 1, true

	case bytecode.BINARY_OP, bytecode.COMPARE_OP, bytecode.CONTAINS_OP,
		bytecode.IS_OP:
		return 2, 1, true

	case bytecode.STORE_ATTR:
		return 2, 0, true

	case bytecode.STORE_SUBSCR:
		return 3, 0, true

	case bytecode.CALL:
		return int(oparg) + 2, 1, true

	case bytecode.BUILD_TUPLE, bytecode.BUILD_LIST, bytecode.BUILD_SET:
		return int(oparg), 1, true

	case bytecode.BUILD_MAP:
		return int(oparg) * 2, 1, true

	case bytecode.JUMP_FORWARD, bytecode.JUMP_BACKWARD:
		return 0, 0, true
	}
	return 0, 0, false
}
