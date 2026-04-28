package ir_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler"
	"github.com/tamnd/gocopy/compiler/ir"
)

// TestRoundtripFixtures is the substantive parity gate for v0.6.3:
// for every fixture, take the bytecode + linetable our compiler
// produces, run them through ir.Decode then ir.Encode, and demand
// byte equality.
//
// The test consumes the in-memory CodeObject from compiler.Compile
// directly, so no marshal-reader is required. It iterates every
// CodeObject in the Consts pool too — function bodies live there
// — to keep the IR honest about nested code objects.
func TestRoundtripFixtures(t *testing.T) {
	root := findFixturesDir(t)
	files, err := filepath.Glob(filepath.Join(root, "*.py"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Skipf("no fixtures found under %s", root)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			co, err := compiler.Compile(src, compiler.Options{Filename: filepath.Base(f)})
			if err != nil {
				t.Skipf("compile: %v", err)
			}
			roundtripCode(t, f, co)
		})
	}
}

// roundtripCode checks one CodeObject and recurses into nested
// CodeObjects in its Consts pool.
func roundtripCode(t *testing.T, path string, co *bytecode.CodeObject) {
	t.Helper()
	seq, err := ir.Decode(co)
	if err != nil {
		t.Fatalf("%s: ir.Decode: %v", path, err)
	}
	bcode, ltab, err := ir.Encode(seq)
	if err != nil {
		t.Fatalf("%s: ir.Encode: %v", path, err)
	}
	if !bytes.Equal(bcode, co.Bytecode) {
		t.Fatalf("%s: bytecode roundtrip diverges (decode/encode produced %d bytes, original %d)",
			path, len(bcode), len(co.Bytecode))
	}
	if !bytes.Equal(ltab, co.LineTable) {
		t.Fatalf("%s: linetable roundtrip diverges (decode/encode produced %d bytes, original %d)",
			path, len(ltab), len(co.LineTable))
	}
	for _, c := range co.Consts {
		nested, ok := c.(*bytecode.CodeObject)
		if !ok {
			continue
		}
		roundtripCode(t, path+"::"+nested.Name, nested)
	}
}

func findFixturesDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cur := cwd
	for {
		candidate := filepath.Join(cur, "tests", "fixtures")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur || !strings.Contains(parent, "gocopy") {
			t.Skipf("could not locate tests/fixtures from %s", cwd)
			return ""
		}
		cur = parent
	}
}
