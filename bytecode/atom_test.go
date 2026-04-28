package bytecode

import "testing"

func TestAtomInternRoundTrip(t *testing.T) {
	tab := NewAtomTable()
	a := tab.Intern("foo")
	b := tab.Intern("bar")
	c := tab.Intern("foo")
	if a != c {
		t.Errorf("Intern should return same Atom for repeated string: %d vs %d", a, c)
	}
	if a == b {
		t.Errorf("distinct strings must get distinct atoms")
	}
	if got := tab.String(a); got != "foo" {
		t.Errorf("String(a)=%q want foo", got)
	}
	if got := tab.String(b); got != "bar" {
		t.Errorf("String(b)=%q want bar", got)
	}
}

func TestAtomOrderingMatchesInsertion(t *testing.T) {
	tab := NewAtomTable()
	for i, s := range []string{"a", "b", "c", "d"} {
		a := tab.Intern(s)
		if int(a) != i {
			t.Errorf("Intern(%q) atom=%d want %d", s, a, i)
		}
	}
	if tab.Len() != 4 {
		t.Errorf("Len()=%d want 4", tab.Len())
	}
}

func TestAtomReinternKeepsID(t *testing.T) {
	tab := NewAtomTable()
	first := tab.Intern("x")
	tab.Intern("y")
	tab.Intern("z")
	again := tab.Intern("x")
	if first != again {
		t.Errorf("re-Intern of x should return same ID, got %d vs %d", again, first)
	}
}

func TestAtomSlice(t *testing.T) {
	tab := NewAtomTable()
	tab.Intern("one")
	tab.Intern("two")
	tab.Intern("three")
	got := tab.Slice()
	want := []string{"one", "two", "three"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Slice()[%d]=%q want %q", i, got[i], want[i])
		}
	}
	got[0] = "mutated"
	if tab.String(0) != "one" {
		t.Errorf("Slice() must return defensive copy")
	}
}

func TestAtomEmptyString(t *testing.T) {
	tab := NewAtomTable()
	a := tab.Intern("")
	b := tab.Intern("")
	if a != b || tab.Len() != 1 {
		t.Errorf("empty string should intern once: a=%d b=%d len=%d", a, b, tab.Len())
	}
}
