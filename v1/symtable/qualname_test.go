package symtable

import "testing"

func TestQualnameModule(t *testing.T) {
	mod := NewScope(ScopeModule, "", nil)
	finalizeQualnames(mod)
	if mod.QualName != "" {
		t.Fatalf("module qualname = %q, want empty", mod.QualName)
	}
}

func TestQualnameTopLevelFunction(t *testing.T) {
	mod := NewScope(ScopeModule, "", nil)
	f := NewScope(ScopeFunction, "f", mod)
	finalizeQualnames(mod)
	if f.QualName != "f" {
		t.Fatalf("top-level qualname = %q, want %q", f.QualName, "f")
	}
}

func TestQualnameNestedFunction(t *testing.T) {
	mod := NewScope(ScopeModule, "", nil)
	outer := NewScope(ScopeFunction, "outer", mod)
	inner := NewScope(ScopeFunction, "inner", outer)
	deep := NewScope(ScopeFunction, "deep", inner)
	finalizeQualnames(mod)

	if got, want := outer.QualName, "outer"; got != want {
		t.Errorf("outer qualname = %q, want %q", got, want)
	}
	if got, want := inner.QualName, "outer.<locals>.inner"; got != want {
		t.Errorf("inner qualname = %q, want %q", got, want)
	}
	if got, want := deep.QualName, "outer.<locals>.inner.<locals>.deep"; got != want {
		t.Errorf("deep qualname = %q, want %q", got, want)
	}
}
