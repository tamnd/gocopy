package symtable

import (
	"reflect"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	parser "github.com/tamnd/gopapy/parser"
)

func parse(t *testing.T, src string) *parser.Module {
	t.Helper()
	mod, err := parser.ParseFile("<test>", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return mod
}

func mustBuild(t *testing.T, src string) *Scope {
	t.Helper()
	mod := parse(t, src)
	root, err := Build(mod)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return root
}

func TestBuildEmptyModule(t *testing.T) {
	root := mustBuild(t, "\n")
	if root.Kind != ScopeModule {
		t.Fatalf("root kind = %v, want module", root.Kind)
	}
	if len(root.Children) != 0 {
		t.Fatalf("empty module has children: %v", root.Children)
	}
}

func TestBuildModuleAssign(t *testing.T) {
	root := mustBuild(t, "x = 1\ny = x\n")
	if root.Lookup("x") == nil {
		t.Fatal("x not defined at module")
	}
	if !root.Lookup("y").Flags.Has(SymAssigned) {
		t.Fatal("y not assigned")
	}
	if !root.Lookup("x").Flags.Has(SymUsed) {
		t.Fatal("x not used")
	}
}

func TestBuildSimpleFuncDef(t *testing.T) {
	src := "def f(a):\n    return a\n"
	root := mustBuild(t, src)
	if got := len(root.Children); got != 1 {
		t.Fatalf("module children = %d, want 1", got)
	}
	f := root.Children[0]
	if f.Kind != ScopeFunction || f.Name != "f" {
		t.Fatalf("child = %v / %v", f.Kind, f.Name)
	}
	if !reflect.DeepEqual(f.Params, []string{"a"}) {
		t.Fatalf("params = %v, want [a]", f.Params)
	}
	if got, want := f.LocalsPlusNames(), []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LocalsPlusNames = %v, want %v", got, want)
	}
	if got, want := f.LocalsPlusKinds(), []byte{bytecode.LocalsKindArg}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LocalsPlusKinds = %v, want %v", got, want)
	}
	if root.Lookup("f") == nil {
		t.Fatal("f not bound at module level")
	}
}

func TestBuildFuncWithBodyLocals(t *testing.T) {
	src := "def f(a, b):\n    c = a\n    d = c\n    return d\n"
	root := mustBuild(t, src)
	f := root.Children[0]

	if got, want := f.LocalsPlusNames(), []string{"a", "b", "c", "d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LocalsPlusNames = %v, want %v", got, want)
	}
	if got, want := f.LocalsPlusKinds(), []byte{
		bytecode.LocalsKindArg,
		bytecode.LocalsKindArg,
		bytecode.LocalsKindLocal,
		bytecode.LocalsKindLocal,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LocalsPlusKinds = %v, want %v", got, want)
	}
}

func TestBuildClosureCellPromotion(t *testing.T) {
	src := "def f(x):\n    def g():\n        return x\n    return g\n"
	root := mustBuild(t, src)
	if len(root.Children) != 1 {
		t.Fatalf("module children = %d", len(root.Children))
	}
	f := root.Children[0]
	if len(f.Children) != 1 {
		t.Fatalf("f children = %d", len(f.Children))
	}
	g := f.Children[0]

	// f.x must be SymParam|SymLocal|SymCell.
	x := f.Lookup("x")
	if !x.Flags.Has(SymParam | SymLocal | SymCell) {
		t.Fatalf("f.x flags = %b, want SymParam|SymLocal|SymCell", x.Flags)
	}
	// g.x must be SymUsed|SymFree.
	gx := g.Lookup("x")
	if !gx.Flags.Has(SymFree) || !gx.Flags.Has(SymUsed) {
		t.Fatalf("g.x flags = %b, want SymUsed|SymFree", gx.Flags)
	}
	// Slot tables.
	if got, want := f.LocalsPlusNames(), []string{"x", "g"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("f LocalsPlusNames = %v, want %v", got, want)
	}
	if got, want := f.LocalsPlusKinds(), []byte{
		bytecode.LocalsKindArgCell,
		bytecode.LocalsKindLocal,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("f LocalsPlusKinds = %v, want %v", got, want)
	}
	if got, want := g.LocalsPlusNames(), []string{"x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("g LocalsPlusNames = %v, want %v", got, want)
	}
	if got, want := g.LocalsPlusKinds(), []byte{bytecode.LocalsKindFree}; !reflect.DeepEqual(got, want) {
		t.Fatalf("g LocalsPlusKinds = %v, want %v", got, want)
	}
	// Qualnames.
	if got, want := f.QualName, "f"; got != want {
		t.Errorf("f.QualName = %q, want %q", got, want)
	}
	if got, want := g.QualName, "f.<locals>.g"; got != want {
		t.Errorf("g.QualName = %q, want %q", got, want)
	}
}

func TestBuildModuleStarImport(t *testing.T) {
	src := "from os import *\nx = 1\n"
	root := mustBuild(t, src)
	if !root.Lookup("*").Flags.Has(SymStarImport) {
		t.Fatal("module did not record star import")
	}
}

func TestBuildModuleImport(t *testing.T) {
	src := "import os\nimport collections.abc as cabc\n"
	root := mustBuild(t, src)
	if root.Lookup("os") == nil {
		t.Fatal("os not bound")
	}
	if root.Lookup("cabc") == nil {
		t.Fatal("cabc not bound")
	}
	if root.Lookup("collections") != nil {
		t.Fatal("collections should not be bound when as-alias is used")
	}
}

func TestBuildClassRejected(t *testing.T) {
	src := "class C:\n    pass\n"
	mod, err := parser.ParseFile("<test>", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := Build(mod); err == nil {
		t.Fatal("expected error for class def")
	}
}

func TestBuildLambdaRejected(t *testing.T) {
	src := "f = lambda x: x\n"
	mod, err := parser.ParseFile("<test>", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := Build(mod); err == nil {
		t.Fatal("expected error for lambda")
	}
}

func TestBuildGlobalDeclaration(t *testing.T) {
	src := "g = 1\ndef f():\n    global g\n    g = 2\n"
	root := mustBuild(t, src)
	f := root.Children[0]
	gSym := f.Lookup("g")
	if gSym == nil || !gSym.Flags.Has(SymGlobal) {
		t.Fatalf("f.g should be SymGlobal, got %+v", gSym)
	}
	if !contains(f.Globals, "g") {
		t.Fatalf("f.Globals = %v", f.Globals)
	}
}
