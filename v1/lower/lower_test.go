package lower_test

import (
	"testing"

	"github.com/tamnd/gocopy/v1/lower"
	parser "github.com/tamnd/gopapy/parser"
)

func TestLowerIdentity(t *testing.T) {
	src := "def f(x):\n    return x\n"
	pmod, err := parser.ParseFile("<test>", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	amod, err := lower.Lower(pmod)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	// At v0.6.2 lowering is identity. The pointer must be the
	// same one ParseFile returned.
	if amod != pmod {
		t.Fatalf("Lower produced a new pointer: %p vs %p", amod, pmod)
	}
}

func TestLowerNilModule(t *testing.T) {
	// A nil module is not an error path the parser can produce,
	// but the boundary should not crash.
	if _, err := lower.Lower(nil); err != nil {
		t.Fatalf("Lower(nil) err = %v, want nil", err)
	}
}
