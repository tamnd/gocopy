package ast_test

import (
	"testing"

	"github.com/tamnd/gocopy/compiler/ast"
	parser "github.com/tamnd/gopapy/parser"
)

// TestAliasIdentity is a compile-time-style assertion that the
// gocopy ast types are identical (not merely assignable) to the
// parser types they alias. If a future release replaces an alias
// with a concrete type, this test breaks intentionally — at which
// point compiler/lower/lower.go grows a real translation step.
func TestAliasIdentity(t *testing.T) {
	src := "x = 1\n"
	pmod, err := parser.ParseFile("<test>", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Direct assignment without conversion proves the alias.
	var amod *ast.Module = pmod
	if amod != pmod {
		t.Fatalf("alias identity: %p vs %p", amod, pmod)
	}
	if len(amod.Body) != 1 {
		t.Fatalf("body len = %d, want 1", len(amod.Body))
	}
	if _, ok := amod.Body[0].(*ast.Assign); !ok {
		t.Fatalf("first stmt is %T, want *ast.Assign", amod.Body[0])
	}
}
