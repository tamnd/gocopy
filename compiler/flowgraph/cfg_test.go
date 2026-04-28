package flowgraph

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

func mkSeq(instrs ...ir.Instr) *ir.InstrSeq {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = append(b.Instrs, instrs...)
	return seq
}

func TestBuildEmpty(t *testing.T) {
	g, err := Build(ir.NewInstrSeq())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(g.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(g.Blocks))
	}
	if g.Entry == nil || g.Exit == nil {
		t.Fatal("Entry/Exit must be non-nil even for empty graph")
	}
}

func TestBuildSingleReturn(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	g, err := Build(seq)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(g.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(g.Blocks))
	}
	if len(g.Edges) != 0 {
		t.Fatalf("edges = %v, want empty (RETURN has no successor)", g.Edges)
	}
}

func TestBuildJumpForward(t *testing.T) {
	// JUMP_FORWARD 1 → skip the next NOP and land at the RETURN_VALUE.
	// Layout in code units: [JUMP_FORWARD][NOP][RETURN] = 3 units.
	seq := mkSeq(
		ir.Instr{Op: bytecode.JUMP_FORWARD, Arg: 1},
		ir.Instr{Op: bytecode.NOP},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	g, err := Build(seq)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Expected: block 0 = [JUMP_FORWARD], block 1 = [NOP],
	// block 2 = [RETURN_VALUE]. JUMP_FORWARD's edge points at block 2.
	if len(g.Blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(g.Blocks))
	}
	edges := g.Edges[g.Blocks[0].ID]
	if len(edges) != 1 || edges[0].Kind != EdgeJump {
		t.Fatalf("block 0 edges = %+v, want one EdgeJump", edges)
	}
	if edges[0].Dest != g.Blocks[2] {
		t.Fatalf("jump target = block %d, want block 2", edges[0].Dest.ID)
	}
}

func TestBuildPopJumpIfFalse(t *testing.T) {
	// POP_JUMP_IF_FALSE has 1 cache word, so it occupies 2 code units.
	// POP_JUMP_IF_FALSE 1 over a NOP (1 unit) lands at the RETURN.
	// Layout: [PJIF + cache][NOP][RETURN] = 4 units.
	seq := mkSeq(
		ir.Instr{Op: bytecode.POP_JUMP_IF_FALSE, Arg: 1},
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
	edges := g.Edges[g.Blocks[0].ID]
	if len(edges) != 2 {
		t.Fatalf("block 0 edges = %d, want 2 (jump + fallthrough)", len(edges))
	}
	var sawJump, sawFall bool
	for _, e := range edges {
		if e.Kind == EdgeJump {
			sawJump = true
		}
		if e.Kind == EdgeFallthrough {
			sawFall = true
		}
	}
	if !sawJump || !sawFall {
		t.Fatalf("block 0 edges = %+v, want both jump and fallthrough", edges)
	}
}

func TestLinearizeIdempotent(t *testing.T) {
	seq := mkSeq(
		ir.Instr{Op: bytecode.JUMP_FORWARD, Arg: 1},
		ir.Instr{Op: bytecode.NOP},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	want, _, err := ir.Encode(seq)
	if err != nil {
		t.Fatalf("Encode pre: %v", err)
	}
	g, err := Build(seq)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out := Linearize(g)
	got, _, err := ir.Encode(out)
	if err != nil {
		t.Fatalf("Encode post: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Linearize roundtrip mismatch:\n got  %v\n want %v", got, want)
	}
}
