package codegen

import (
	"bytes"
	"errors"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
)

// TestBuildEmptyModule covers the only shape v0.6.6 hands to the
// codegen path. The output must byte-equal the canonical empty
// module CodeObject the classifier emits today.
func TestBuildEmptyModule(t *testing.T) {
	mod := &ast.Module{}
	co, err := Build(mod, nil, Options{
		Filename:    "x.py",
		Name:        "<module>",
		QualName:    "<module>",
		FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(empty): %v", err)
	}
	if !bytes.Equal(co.Bytecode, bytecode.NoOpBytecode(1)) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, bytecode.NoOpBytecode(1))
	}
	if !bytes.Equal(co.LineTable, bytecode.LineTableEmpty()) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, bytecode.LineTableEmpty())
	}
	if len(co.ExcTable) != 0 {
		t.Fatalf("exctable = %x, want empty", co.ExcTable)
	}
	if co.StackSize != 1 {
		t.Fatalf("stacksize = %d, want 1", co.StackSize)
	}
	if co.FirstLineNo != 1 {
		t.Fatalf("firstlineno = %d, want 1", co.FirstLineNo)
	}
	if co.Filename != "x.py" || co.Name != "<module>" || co.QualName != "<module>" {
		t.Fatalf("metadata = %q/%q/%q, want x.py/<module>/<module>",
			co.Filename, co.Name, co.QualName)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 0 {
		t.Fatalf("names = %v, want []", co.Names)
	}
}

// TestBuildUnsupportedReturnsErrUnsupported guarantees the driver's
// fallback contract: any non-empty module returns ErrUnsupported so
// the classifier path takes over.
func TestBuildUnsupportedReturnsErrUnsupported(t *testing.T) {
	mod := &ast.Module{Body: []ast.Stmt{&ast.Pass{}}}
	_, err := Build(mod, nil, Options{Name: "<module>", QualName: "<module>"})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(non-empty) error = %v, want ErrUnsupported", err)
	}
}

func TestBuildNilModule(t *testing.T) {
	_, err := Build(nil, nil, Options{})
	if err == nil {
		t.Fatalf("Build(nil) returned nil error")
	}
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(nil) returned ErrUnsupported, want a distinct error")
	}
}
