package assemble

import (
	"bytes"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// TestAssembleSourceNilUsesOptionsMetadata covers the v0.6.6 codegen
// path: callers without a fixture CodeObject hand the metadata in
// directly via Options. The byte-level fields still come from the
// IR + assembler.
func TestAssembleSourceNilUsesOptionsMetadata(t *testing.T) {
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	seq := ir.NewInstrSeq()
	seq.FirstLineNo = 1
	b := seq.AddBlock()
	b.Instrs = []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: syntheticLoc},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: syntheticLoc},
	}

	co, err := Assemble(seq, Options{
		Filename: "x.py",
		Name:     "<module>",
		QualName: "<module>",
		Consts:   []any{nil},
		Names:    []string{},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if !bytes.Equal(co.Bytecode, bytecode.NoOpBytecode(1)) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, bytecode.NoOpBytecode(1))
	}
	if !bytes.Equal(co.LineTable, bytecode.LineTableEmpty()) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, bytecode.LineTableEmpty())
	}
	if co.StackSize != 1 {
		t.Fatalf("stacksize = %d, want 1", co.StackSize)
	}
	if co.Filename != "x.py" || co.Name != "<module>" {
		t.Fatalf("metadata = %q/%q, want x.py/<module>", co.Filename, co.Name)
	}
	if co.LocalsPlusNames == nil || co.LocalsPlusKinds == nil {
		t.Fatalf("nil-defaulted localsplus slices not coerced to empty: %v / %v",
			co.LocalsPlusNames, co.LocalsPlusKinds)
	}
}
