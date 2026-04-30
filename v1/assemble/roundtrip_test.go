package assemble_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1"
	"github.com/tamnd/gocopy/v1/assemble"
	"github.com/tamnd/gocopy/v1/flowgraph"
	"github.com/tamnd/gocopy/v1/ir"
)

// TestRoundtripAssemble extends the v0.6.4 CFG round-trip with the
// assembler: Decode → Build → Linearize → Assemble must reproduce
// the original byte stream, line table, exception table, and stack
// size for every fixture's module CodeObject and every nested
// function CodeObject.
func TestRoundtripAssemble(t *testing.T) {
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
	co2, err := assemble.Assemble(linear, assemble.Options{Source: co})
	if err != nil {
		t.Fatalf("%s: assemble.Assemble: %v", path, err)
	}
	if !bytes.Equal(co2.Bytecode, co.Bytecode) {
		t.Fatalf("%s: bytecode diverges (got %d bytes, want %d)", path, len(co2.Bytecode), len(co.Bytecode))
	}
	if !bytes.Equal(co2.LineTable, co.LineTable) {
		t.Fatalf("%s: linetable diverges\n got  %x\n want %x", path, co2.LineTable, co.LineTable)
	}
	if !bytes.Equal(co2.ExcTable, co.ExcTable) {
		t.Fatalf("%s: exctable diverges (got %d bytes, want %d)", path, len(co2.ExcTable), len(co.ExcTable))
	}
	if co2.StackSize != co.StackSize {
		t.Fatalf("%s: stacksize = %d, want %d", path, co2.StackSize, co.StackSize)
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
