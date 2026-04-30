package flowgraph

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// InsertSuperinstructions fuses adjacent instruction pairs into the
// fixed set of super-instructions CPython 3.14 emits at this stage:
//
//   - LOAD_FAST + LOAD_FAST   → LOAD_FAST_LOAD_FAST
//   - STORE_FAST + LOAD_FAST  → STORE_FAST_LOAD_FAST
//   - STORE_FAST + STORE_FAST → STORE_FAST_STORE_FAST
//
// SOURCE: CPython 3.14 Python/flowgraph.c:2588 insert_superinstructions.
//
// The pass operates over each block's Instrs slice in place. The
// CPython implementation replaces the second instruction of a fused
// pair with NOP and runs remove_redundant_nops afterwards; the Go
// version splices the pair into a single fused instruction directly,
// which is equivalent for byte parity (no other pass at this point
// in the pipeline introduces NOPs).
//
// The LOAD_FAST → LOAD_FAST_BORROW promotion (and the LFLF →
// LOAD_FAST_BORROW_LOAD_FAST_BORROW promotion that follows from it)
// is owned by OptimizeLoadFast (Python/flowgraph.c:2776), which
// runs immediately after this pass.
func InsertSuperinstructions(seq *ir.InstrSeq) {
	if seq == nil {
		return
	}
	for _, b := range seq.Blocks {
		if b == nil || len(b.Instrs) < 2 {
			continue
		}
		out := make([]ir.Instr, 0, len(b.Instrs))
		i := 0
		for i < len(b.Instrs) {
			if i+1 < len(b.Instrs) {
				if fused, ok := makeSuperInstruction(b.Instrs[i], b.Instrs[i+1]); ok {
					out = append(out, fused)
					i += 2
					continue
				}
			}
			out = append(out, b.Instrs[i])
			i++
		}
		b.Instrs = out
	}
}

// makeSuperInstruction returns the fused instruction for the
// pair (a, b) when it matches one of the recognised forms. The
// per-instruction guards mirror CPython's:
//
//   - both opargs must be < 16 (the super-instruction encodes them
//     in 4 bits each: high nibble = a, low nibble = b);
//   - if both instructions carry a non-zero source line number, the
//     lines must match (CPython skips fusion across line boundaries
//     so PEP 626 line attribution stays correct).
//
// SOURCE: CPython 3.14 Python/flowgraph.c:2572 make_super_instruction.
func makeSuperInstruction(a, b ir.Instr) (ir.Instr, bool) {
	if a.Loc.Line != 0 && b.Loc.Line != 0 && a.Loc.Line != b.Loc.Line {
		return ir.Instr{}, false
	}
	if a.Arg >= 16 || b.Arg >= 16 {
		return ir.Instr{}, false
	}
	var superOp bytecode.Opcode
	switch {
	case a.Op == bytecode.LOAD_FAST && b.Op == bytecode.LOAD_FAST:
		superOp = bytecode.LOAD_FAST_LOAD_FAST
	case a.Op == bytecode.STORE_FAST && b.Op == bytecode.LOAD_FAST:
		superOp = bytecode.STORE_FAST_LOAD_FAST
	case a.Op == bytecode.STORE_FAST && b.Op == bytecode.STORE_FAST:
		superOp = bytecode.STORE_FAST_STORE_FAST
	default:
		return ir.Instr{}, false
	}
	return ir.Instr{
		Op:  superOp,
		Arg: (a.Arg << 4) | b.Arg,
		Loc: a.Loc,
	}, true
}
