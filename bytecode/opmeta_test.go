package bytecode

import "testing"

// TestOpMetaCacheSizeMirrorsArray asserts that OpMetaTable is in
// sync with the existing CacheSize array. CacheSize stays as the
// runtime hot-path lookup; OpMetaTable is the richer descriptor.
// Drift between the two would be a silent correctness bug.
func TestOpMetaCacheSizeMirrorsArray(t *testing.T) {
	for op := range 256 {
		m := OpMetaTable[op]
		if m.Name == "" {
			continue
		}
		if m.CacheSize != CacheSize[op] {
			t.Errorf("op %d (%s) CacheSize=%d but CacheSize[%d]=%d",
				op, m.Name, m.CacheSize, op, CacheSize[op])
		}
	}
}

// TestOpMetaJumpsAreFlagged asserts the OpJump flag is set on every
// known jump opcode and not on others.
func TestOpMetaJumpsAreFlagged(t *testing.T) {
	jumps := map[Opcode]bool{
		JUMP_FORWARD:         true,
		JUMP_BACKWARD:        true,
		FOR_ITER:             true,
		POP_JUMP_IF_FALSE:    true,
		POP_JUMP_IF_TRUE:     true,
		POP_JUMP_IF_NONE:     true,
		POP_JUMP_IF_NOT_NONE: true,
	}
	for op, want := range jumps {
		got := OpMetaTable[op].Flags&OpJump != 0
		if got != want {
			t.Errorf("op %s: OpJump set=%v want %v", OpMetaTable[op].Name, got, want)
		}
	}
	// Negative: a non-jump opcode should not have OpJump set.
	if OpMetaTable[LOAD_CONST].Flags&OpJump != 0 {
		t.Errorf("LOAD_CONST should not have OpJump")
	}
}

// TestOpMetaTerminators asserts terminator flag.
func TestOpMetaTerminators(t *testing.T) {
	terms := []Opcode{RETURN_VALUE, JUMP_FORWARD, JUMP_BACKWARD}
	for _, op := range terms {
		if OpMetaTable[op].Flags&OpTerminator == 0 {
			t.Errorf("%s should be terminator", OpMetaTable[op].Name)
		}
	}
	if OpMetaTable[POP_TOP].Flags&OpTerminator != 0 {
		t.Errorf("POP_TOP should not be terminator")
	}
}

// TestOpMetaStackVarOpsAreMarked asserts that opcodes with
// arg-dependent stack effects have StackVar set and a zero StackEff.
func TestOpMetaStackVarOpsAreMarked(t *testing.T) {
	stackVar := []Opcode{CALL, BUILD_TUPLE, BUILD_LIST, BUILD_MAP, BUILD_SET, LOAD_GLOBAL, LOAD_ATTR}
	for _, op := range stackVar {
		m := OpMetaTable[op]
		if !m.StackVar {
			t.Errorf("%s should have StackVar=true", m.Name)
		}
		if m.StackEff != 0 {
			t.Errorf("%s StackVar=true should have StackEff=0, got %d", m.Name, m.StackEff)
		}
	}
}

// TestOpMetaNamesUnique asserts no two populated entries share a Name.
func TestOpMetaNamesUnique(t *testing.T) {
	seen := map[string]int{}
	for i, m := range OpMetaTable {
		if m.Name == "" {
			continue
		}
		if prev, ok := seen[m.Name]; ok {
			t.Errorf("name %s used by op %d and op %d", m.Name, prev, i)
		}
		seen[m.Name] = i
	}
}

// TestMetaOf round-trips through MetaOf.
func TestMetaOf(t *testing.T) {
	if MetaOf(LOAD_CONST).Name != "LOAD_CONST" {
		t.Errorf("MetaOf(LOAD_CONST).Name=%q", MetaOf(LOAD_CONST).Name)
	}
	if MetaOf(255).Name != "" {
		t.Errorf("unknown opcode should have empty name")
	}
}
