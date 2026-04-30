package codegen

import (
	"errors"
	"testing"

	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/symtable"
)

func TestGenerateNilModuleErrs(t *testing.T) {
	scope, err := symtable.Build(&ast.Module{})
	if err != nil {
		t.Fatalf("symtable.Build: %v", err)
	}
	_, _, _, err = Generate(nil, scope, GenerateOptions{Filename: "x.py", Name: "<module>", QualName: "<module>", FirstLineNo: 1})
	if err == nil {
		t.Fatal("Generate(nil, ...) returned nil error, want non-nil")
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Generate(nil, ...) returned ErrNotImplemented, want a different error")
	}
}

func TestGenerateNilScopeErrs(t *testing.T) {
	_, _, _, err := Generate(&ast.Module{}, nil, GenerateOptions{Filename: "x.py", Name: "<module>", QualName: "<module>", FirstLineNo: 1})
	if err == nil {
		t.Fatal("Generate(mod, nil, ...) returned nil error, want non-nil")
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Generate(mod, nil, ...) returned ErrNotImplemented, want a different error")
	}
}

func TestGenerateEmptyModule(t *testing.T) {
	mod := &ast.Module{}
	scope, err := symtable.Build(mod)
	if err != nil {
		t.Fatalf("symtable.Build: %v", err)
	}
	seq, consts, names, err := Generate(mod, scope, GenerateOptions{Filename: "x.py", Name: "<module>", QualName: "<module>", FirstLineNo: 1})
	if err != nil {
		t.Fatalf("Generate empty module: err = %v, want nil", err)
	}
	if seq == nil || len(seq.Blocks) != 1 || len(seq.Blocks[0].Instrs) != 3 {
		t.Fatalf("Generate empty module: seq=%v, want 1 block with 3 instrs", seq)
	}
	if len(consts) != 1 || consts[0] != nil {
		t.Fatalf("Generate empty module: consts=%v, want [nil]", consts)
	}
	if len(names) != 0 {
		t.Fatalf("Generate empty module: names=%v, want []", names)
	}
}

func TestAddConstDedupNil(t *testing.T) {
	scope, err := symtable.Build(&ast.Module{})
	if err != nil {
		t.Fatalf("symtable.Build: %v", err)
	}
	u := newCompileUnit(scope, "<module>", "<module>", 1, nil)
	if i := u.addConst(nil); i != 0 {
		t.Fatalf("first addConst(nil) = %d, want 0", i)
	}
	if i := u.addConst(nil); i != 0 {
		t.Fatalf("second addConst(nil) = %d, want 0 (dedup)", i)
	}
	if len(u.Consts) != 1 {
		t.Fatalf("Consts after two addConst(nil) = %v, want length 1", u.Consts)
	}
}

func TestAddConstDistinctValues(t *testing.T) {
	scope, err := symtable.Build(&ast.Module{})
	if err != nil {
		t.Fatalf("symtable.Build: %v", err)
	}
	u := newCompileUnit(scope, "<module>", "<module>", 1, nil)
	if i := u.addConst("hello"); i != 0 {
		t.Fatalf("addConst(\"hello\") = %d, want 0", i)
	}
	if i := u.addConst(nil); i != 1 {
		t.Fatalf("addConst(nil) = %d, want 1", i)
	}
	if i := u.addConst("hello"); i != 0 {
		t.Fatalf("addConst(\"hello\") again = %d, want 0 (dedup)", i)
	}
	if i := u.addConst("world"); i != 2 {
		t.Fatalf("addConst(\"world\") = %d, want 2", i)
	}
}

func TestAddNameDedup(t *testing.T) {
	scope, err := symtable.Build(&ast.Module{})
	if err != nil {
		t.Fatalf("symtable.Build: %v", err)
	}
	u := newCompileUnit(scope, "<module>", "<module>", 1, nil)
	if i := u.addName("__doc__"); i != 0 {
		t.Fatalf("addName(\"__doc__\") = %d, want 0", i)
	}
	if i := u.addName("__doc__"); i != 0 {
		t.Fatalf("addName(\"__doc__\") again = %d, want 0 (dedup)", i)
	}
	if i := u.addName("foo"); i != 1 {
		t.Fatalf("addName(\"foo\") = %d, want 1", i)
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
