package codegen

import (
	"errors"
	"testing"

	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/symtable"
)

func TestGenerateNilModuleErrs(t *testing.T) {
	scope, err := symtable.Build(&ast.Module{})
	if err != nil {
		t.Fatalf("symtable.Build: %v", err)
	}
	_, err = Generate(nil, scope, GenerateOptions{Filename: "x.py", Name: "<module>", QualName: "<module>", FirstLineNo: 1})
	if err == nil {
		t.Fatal("Generate(nil, ...) returned nil error, want non-nil")
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Generate(nil, ...) returned ErrNotImplemented, want a different error")
	}
}

func TestGenerateNilScopeErrs(t *testing.T) {
	_, err := Generate(&ast.Module{}, nil, GenerateOptions{Filename: "x.py", Name: "<module>", QualName: "<module>", FirstLineNo: 1})
	if err == nil {
		t.Fatal("Generate(mod, nil, ...) returned nil error, want non-nil")
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Generate(mod, nil, ...) returned ErrNotImplemented, want a different error")
	}
}

func TestGenerateEmptyModuleNotImplemented(t *testing.T) {
	mod := &ast.Module{}
	scope, err := symtable.Build(mod)
	if err != nil {
		t.Fatalf("symtable.Build: %v", err)
	}
	_, err = Generate(mod, scope, GenerateOptions{Filename: "x.py", Name: "<module>", QualName: "<module>", FirstLineNo: 1})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Generate empty module: err = %v, want ErrNotImplemented", err)
	}
}

func TestNewCompileUnitFields(t *testing.T) {
	scope, err := symtable.Build(&ast.Module{})
	if err != nil {
		t.Fatalf("symtable.Build: %v", err)
	}
	u := newCompileUnit(scope, "<module>", "<module>", 1, nil)
	if u.Scope != scope {
		t.Fatalf("Scope = %p, want %p", u.Scope, scope)
	}
	if u.Seq == nil {
		t.Fatal("Seq is nil; expected fresh InstrSeq")
	}
	if u.Name != "<module>" || u.QualName != "<module>" {
		t.Fatalf("Name/QualName = %q/%q, want <module>/<module>", u.Name, u.QualName)
	}
	if u.FirstLineNo != 1 {
		t.Fatalf("FirstLineNo = %d, want 1", u.FirstLineNo)
	}
	if u.Parent != nil {
		t.Fatalf("Parent = %v, want nil", u.Parent)
	}
}
