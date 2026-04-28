package flowgraph_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler"
	"github.com/tamnd/gocopy/compiler/flowgraph"
	"github.com/tamnd/gocopy/compiler/ir"
)

// TestRoundtripCFG extends the v0.6.3 IR round-trip with a CFG
// pass: Decode → Build → Linearize → Encode must reproduce the
// original byte stream and line table for every fixture.
func TestRoundtripCFG(t *testing.T) {
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

func roundtripCode(t *testing.T, path string, co *bytecode.CodeObject) {
	t.Helper()
	seq, err := ir.Decode(co)
	if err != nil {
		t.Fatalf("%s: ir.Decode: %v", path, err)
	}
	cfg, err := flowgraph.Build(seq)
	if err != nil {
		t.Fatalf("%s: flowgraph.Build: %v", path, err)
	}
	linear := flowgraph.Linearize(cfg)
	bcode, ltab, err := ir.Encode(linear)
	if err != nil {
		t.Fatalf("%s: ir.Encode: %v", path, err)
	}
	if !bytes.Equal(bcode, co.Bytecode) {
		t.Fatalf("%s: CFG roundtrip diverges (decode→build→linearize→encode produced %d bytes, original %d)",
			path, len(bcode), len(co.Bytecode))
	}
	if !bytes.Equal(ltab, co.LineTable) {
		t.Fatalf("%s: linetable diverges through CFG roundtrip", path)
	}
	for _, c := range co.Consts {
		if nested, ok := c.(*bytecode.CodeObject); ok {
			roundtripCode(t, path+"::"+nested.Name, nested)
		}
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
