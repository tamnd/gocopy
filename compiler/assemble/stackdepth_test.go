package assemble

import (
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/flowgraph"
	"github.com/tamnd/gocopy/compiler/ir"
)

func mkCFG(t *testing.T, instrs ...ir.Instr) *flowgraph.CFG {
	t.Helper()
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	b.Instrs = append(b.Instrs, instrs...)
	cfg, err := flowgraph.Build(seq)
	if err != nil {
		t.Fatalf("flowgraph.Build: %v", err)
	}
	return cfg
}

func TestStackDepthEmpty(t *testing.T) {
	cfg := mkCFG(t)
	if got := StackDepth(cfg); got != 0 {
		t.Fatalf("StackDepth(empty) = %d, want 0", got)
	}
}

func TestStackDepthLinear(t *testing.T) {
	// PUSH_NULL +1, LOAD_CONST +1, RETURN_VALUE 0.
	// Peak depth = 2 (after LOAD_CONST).
	cfg := mkCFG(t,
		ir.Instr{Op: bytecode.PUSH_NULL},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	if got := StackDepth(cfg); got != 2 {
		t.Fatalf("StackDepth(linear) = %d, want 2", got)
	}
}

func TestStackDepthCallVar(t *testing.T) {
	// PUSH_NULL +1; LOAD_CONST +1; LOAD_CONST +1; LOAD_CONST +1;
	// CALL 2 → pops 4, pushes 1, peak=4.
	cfg := mkCFG(t,
		ir.Instr{Op: bytecode.PUSH_NULL},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 2},
		ir.Instr{Op: bytecode.CALL, Arg: 2},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	if got := StackDepth(cfg); got != 4 {
		t.Fatalf("StackDepth(call) = %d, want 4", got)
	}
}

func TestStackDepthLoadGlobalPushNull(t *testing.T) {
	// LOAD_GLOBAL with low bit set pushes 2 (NULL + global).
	cfg := mkCFG(t,
		ir.Instr{Op: bytecode.LOAD_GLOBAL, Arg: 1},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	if got := StackDepth(cfg); got != 2 {
		t.Fatalf("StackDepth(load_global+1) = %d, want 2", got)
	}
}

func TestStackDepthBuildList(t *testing.T) {
	// LOAD_CONST x5; BUILD_LIST 5 (1-5 = -4). Peak before build = 5.
	cfg := mkCFG(t,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 2},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 3},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 4},
		ir.Instr{Op: bytecode.BUILD_LIST, Arg: 5},
		ir.Instr{Op: bytecode.RETURN_VALUE},
	)
	if got := StackDepth(cfg); got != 5 {
		t.Fatalf("StackDepth(build_list) = %d, want 5", got)
	}
}
