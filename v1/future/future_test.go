package future_test

import (
	"strings"
	"testing"

	"github.com/tamnd/gocopy/v1/future"
	"github.com/tamnd/gocopy/v1/lower"
	parser "github.com/tamnd/gopapy/parser"
)

func mod(t *testing.T, src string) *parser.Module {
	t.Helper()
	m, err := parser.ParseFile("<test>", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return m
}

func collect(t *testing.T, src string) (future.Flags, error) {
	t.Helper()
	pmod := mod(t, src)
	amod, err := lower.Lower(pmod)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	return future.Collect(amod)
}

func TestCollectEmpty(t *testing.T) {
	got, err := collect(t, "x = 1\n")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got != 0 {
		t.Fatalf("flags = %#x, want 0", got)
	}
}

func TestCollectNilModule(t *testing.T) {
	got, err := future.Collect(nil)
	if err != nil || got != 0 {
		t.Fatalf("Collect(nil) = (%#x, %v)", got, err)
	}
}

func TestCollectAnnotations(t *testing.T) {
	got, err := collect(t, "from __future__ import annotations\nx = 1\n")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got&future.Annotations == 0 {
		t.Fatalf("flags = %#x, missing Annotations", got)
	}
}

func TestCollectMultipleNames(t *testing.T) {
	got, err := collect(t, "from __future__ import annotations, division\n")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got&future.Annotations == 0 || got&future.Division == 0 {
		t.Fatalf("flags = %#x, missing Annotations|Division", got)
	}
}

func TestCollectAfterDocstring(t *testing.T) {
	src := "\"hello\"\nfrom __future__ import annotations\n"
	got, err := collect(t, src)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got&future.Annotations == 0 {
		t.Fatalf("flags = %#x, missing Annotations", got)
	}
}

func TestCollectAfterStmtRejected(t *testing.T) {
	src := "x = 1\nfrom __future__ import annotations\n"
	_, err := collect(t, src)
	if err == nil {
		t.Fatal("expected error for misplaced future import")
	}
	if !strings.Contains(err.Error(), "beginning of the file") {
		t.Fatalf("err = %v, want 'beginning of the file'", err)
	}
}

func TestCollectStarRejected(t *testing.T) {
	_, err := collect(t, "from __future__ import *\n")
	if err == nil {
		t.Fatal("expected error for star import")
	}
	if !strings.Contains(err.Error(), "future feature *") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectUnknownNameRejected(t *testing.T) {
	_, err := collect(t, "from __future__ import imaginary_feature\n")
	if err == nil {
		t.Fatal("expected error for unknown future feature")
	}
	if !strings.Contains(err.Error(), "imaginary_feature") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectPlainImportFutureNotAFlag(t *testing.T) {
	// `import __future__` is a normal import, not a flag directive.
	// It should not error and should not set any flag.
	got, err := collect(t, "import __future__\n")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got != 0 {
		t.Fatalf("flags = %#x, want 0", got)
	}
}
