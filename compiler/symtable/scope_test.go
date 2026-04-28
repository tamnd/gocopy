package symtable

import (
	"reflect"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
)

func TestSymbolKindBits(t *testing.T) {
	var s SymbolKind
	s |= SymParam | SymLocal
	if !s.Has(SymParam) {
		t.Fatal("expected SymParam set")
	}
	if !s.Has(SymLocal) {
		t.Fatal("expected SymLocal set")
	}
	if s.Has(SymCell) {
		t.Fatal("did not expect SymCell")
	}
	if !s.HasAny(SymCell | SymLocal) {
		t.Fatal("HasAny should match SymLocal")
	}
}

func TestScopeKindString(t *testing.T) {
	cases := map[ScopeKind]string{
		ScopeModule:        "module",
		ScopeFunction:      "function",
		ScopeClass:         "class",
		ScopeComprehension: "comprehension",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("ScopeKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestNewScopeClassPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for ScopeClass")
		}
	}()
	NewScope(ScopeClass, "C", nil)
}

func TestNewScopeComprehensionPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for ScopeComprehension")
		}
	}()
	NewScope(ScopeComprehension, "<listcomp>", nil)
}

func TestDefineFirstRecordsOrder(t *testing.T) {
	mod := NewScope(ScopeModule, "", nil)
	mod.Define("a", SymAssigned)
	mod.Define("b", SymAssigned)
	mod.Define("a", SymUsed) // re-define ORs flags, no reorder
	mod.Define("c", SymAssigned)

	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(mod.OrderedNames, want) {
		t.Fatalf("OrderedNames = %v, want %v", mod.OrderedNames, want)
	}
	a := mod.Lookup("a")
	if !a.Flags.Has(SymAssigned | SymUsed) {
		t.Fatalf("a flags = %b, want SymAssigned|SymUsed", a.Flags)
	}
}

// TestLocalsPlusModuleFunction covers a plain function: two params,
// two body locals, no closure.
//
//	def f(a, b):
//	    c = a
//	    d = c
//
// Expected slots: a, b, c, d  — all FastLocal|FastArg for params,
// FastLocal for body locals.
func TestLocalsPlusModuleFunction(t *testing.T) {
	mod := NewScope(ScopeModule, "", nil)
	f := NewScope(ScopeFunction, "f", mod)
	f.Params = []string{"a", "b"}
	f.ArgCount = 2
	f.Define("a", SymParam|SymLocal)
	f.Define("b", SymParam|SymLocal)
	f.Define("c", SymLocal|SymAssigned)
	f.Define("d", SymLocal|SymAssigned)

	gotNames := f.LocalsPlusNames()
	wantNames := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("LocalsPlusNames = %v, want %v", gotNames, wantNames)
	}
	gotKinds := f.LocalsPlusKinds()
	wantKinds := []byte{
		bytecode.LocalsKindArg,
		bytecode.LocalsKindArg,
		bytecode.LocalsKindLocal,
		bytecode.LocalsKindLocal,
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("LocalsPlusKinds = %v, want %v", gotKinds, wantKinds)
	}
	if f.Lookup("a").Index != 0 || f.Lookup("d").Index != 3 {
		t.Fatalf("Index stamping wrong: a=%d d=%d", f.Lookup("a").Index, f.Lookup("d").Index)
	}
}

// TestLocalsPlusClosure mirrors the closure shape:
//
//	def outer(x):
//	    def inner():
//	        return x
//	    return inner
//
// In outer: x is a param that is also a cell — slot kind 0x66.
// In inner: x is a free — slot kind 0x80.
func TestLocalsPlusClosure(t *testing.T) {
	mod := NewScope(ScopeModule, "", nil)
	outer := NewScope(ScopeFunction, "outer", mod)
	outer.Params = []string{"x"}
	outer.ArgCount = 1
	outer.Define("x", SymParam|SymLocal|SymCell)
	outer.Define("inner", SymLocal|SymAssigned)
	outer.Cells = []string{"x"}

	inner := NewScope(ScopeFunction, "inner", outer)
	inner.Define("x", SymUsed|SymFree)
	inner.Frees = []string{"x"}

	if got, want := outer.LocalsPlusNames(), []string{"x", "inner"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outer LocalsPlusNames = %v, want %v", got, want)
	}
	if got, want := outer.LocalsPlusKinds(), []byte{
		bytecode.LocalsKindArgCell,
		bytecode.LocalsKindLocal,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outer LocalsPlusKinds = %v, want %v", got, want)
	}

	if got, want := inner.LocalsPlusNames(), []string{"x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inner LocalsPlusNames = %v, want %v", got, want)
	}
	if got, want := inner.LocalsPlusKinds(), []byte{
		bytecode.LocalsKindFree,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("inner LocalsPlusKinds = %v, want %v", got, want)
	}
}

func TestResolveSkipsClassWalksParents(t *testing.T) {
	mod := NewScope(ScopeModule, "", nil)
	mod.Define("g", SymGlobalImplicit)

	f := NewScope(ScopeFunction, "f", mod)
	f.Params = []string{"a"}
	f.Define("a", SymParam|SymLocal|SymCell)

	inner := NewScope(ScopeFunction, "inner", f)

	if sym, owner := inner.Resolve("a"); sym == nil || owner != f {
		t.Fatalf("Resolve(a) owner = %v, want f", owner)
	}
	if sym, _ := inner.Resolve("g"); sym != nil {
		// Module-level global-implicit is not a binding, so Resolve
		// should not stop at the module scope for free-variable
		// promotion. Match CPython: only param/local/cell qualify.
		t.Fatalf("Resolve(g) should not promote module global; got %v", sym)
	}
}

func TestWalkPreOrder(t *testing.T) {
	mod := NewScope(ScopeModule, "", nil)
	a := NewScope(ScopeFunction, "a", mod)
	NewScope(ScopeFunction, "b", mod)
	NewScope(ScopeFunction, "c", a)

	var order []string
	mod.Walk(func(s *Scope) {
		order = append(order, s.Name)
	})
	want := []string{"", "a", "c", "b"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("walk order = %v, want %v", order, want)
	}
}
