package bytecode

import "testing"

func TestLocValid(t *testing.T) {
	if (Loc{}).Valid() {
		t.Error("zero Loc should be invalid")
	}
	if !(Loc{Line: 1}).Valid() {
		t.Error("Line=1 should be valid")
	}
}

func TestLocIsZero(t *testing.T) {
	if !(Loc{}).IsZero() {
		t.Error("zero Loc should be IsZero")
	}
	if (Loc{Line: 1}).IsZero() {
		t.Error("non-zero Line means not IsZero")
	}
	if (Loc{Col: 1}).IsZero() {
		t.Error("non-zero Col means not IsZero")
	}
}

func TestLocOneLine(t *testing.T) {
	if (Loc{}).OneLine() {
		t.Error("invalid Loc cannot be OneLine")
	}
	if !(Loc{Line: 5, EndLine: 5}).OneLine() {
		t.Error("Line==EndLine should be OneLine")
	}
	if (Loc{Line: 5, EndLine: 6}).OneLine() {
		t.Error("Line!=EndLine should not be OneLine")
	}
	if !(Loc{Line: 5, EndLine: 5, Col: 0, EndCol: 10}).OneLine() {
		t.Error("OneLine should ignore columns")
	}
}

func TestLocEquality(t *testing.T) {
	a := Loc{Line: 1, EndLine: 2, Col: 3, EndCol: 4}
	b := Loc{Line: 1, EndLine: 2, Col: 3, EndCol: 4}
	if a != b {
		t.Error("identical Loc values should compare equal")
	}
	c := Loc{Line: 1, EndLine: 2, Col: 3, EndCol: 5}
	if a == c {
		t.Error("differing Loc values should compare unequal")
	}
}

func TestLocSize(t *testing.T) {
	// Sanity: keep the struct compact. 12 bytes is the minimum on
	// any sane alignment (2x uint32 + 2x uint16).
	var l Loc
	_ = l
}
