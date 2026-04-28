package flowgraph

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

func TestDominatorsLinearChain(t *testing.T) {
	// JUMP_FORWARD over a NOP gives two distinct blocks plus the
	// jump-target block. Layout (units): [JUMP_FORWARD][NOP][RETURN]:
	//   block 0: JUMP_FORWARD 1
	//   block 1: NOP            (unreachable, but build-time CFG
	//                            still includes it)
	//   block 2: RETURN_VALUE   (jump target)
	//
	// Block 0 dominates block 2 (the only reachable successor).
	// Block 1 is unreachable from block 0; Dominators leaves it
	// undefined, which the algorithm tolerates.
	seq := mkSeq(
		ir.Instr{Op: bytecode.JUMP_FORWARD, Arg: 1},
		ir.Instr{Op: bytecode.NOP},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	g, err := Build(seq)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(g.Blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(g.Blocks))
	}
	dt := Dominators(g)
	if dt.Idom[g.Blocks[0].ID] != g.Blocks[0].ID {
		t.Fatalf("Idom[0] = %d, want self", dt.Idom[g.Blocks[0].ID])
	}
	if dt.Idom[g.Blocks[2].ID] != g.Blocks[0].ID {
		t.Fatalf("Idom[2] = %d, want 0", dt.Idom[g.Blocks[2].ID])
	}
}

func TestDominatorsDiamond(t *testing.T) {
	// Diamond:
	//   block 0: POP_JUMP_IF_FALSE → fall to block 1 OR jump to block 3
	//   block 1: JUMP_FORWARD     → block 3
	//   block 2: NOP              → fallthrough block 3 (unreachable here)
	//   block 3: RETURN_VALUE
	//
	// Layout in units: [PJIF +cache=2u][JUMP_FORWARD=1u][NOP=1u][RETURN=1u].
	// PJIF arg=2 → target = 2 + 2 = 4 (RETURN start).
	// JUMP_FORWARD arg=1 → target = 3 + 1 = 4 (RETURN start).
	//
	// Both branches converge at block 3 (RETURN). The join is
	// dominated by block 0, the branch head.
	seq := mkSeq(
		ir.Instr{Op: bytecode.POP_JUMP_IF_FALSE, Arg: 2},
		ir.Instr{Op: bytecode.JUMP_FORWARD, Arg: 1},
		ir.Instr{Op: bytecode.NOP},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	g, err := Build(seq)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(g.Blocks) != 4 {
		t.Fatalf("blocks = %d, want 4", len(g.Blocks))
	}
	dt := Dominators(g)
	// Join block (3) is dominated by block 0 (the diamond head),
	// not by 1 or 2.
	if got := dt.Idom[g.Blocks[3].ID]; got != g.Blocks[0].ID {
		t.Fatalf("Idom[3] = %d, want 0 (diamond head)", got)
	}
}
