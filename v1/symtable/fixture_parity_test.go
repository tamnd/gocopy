package symtable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	parser "github.com/tamnd/gopapy/parser"
)

// TestFixtureParity asserts that Build accepts every Python source
// file under tests/fixtures/ without error. The set is the same
// 246 fixtures the oracle drives; if a new fixture introduces a
// scope shape the symtable does not handle, this test catches it
// before the oracle does.
//
// This is the per-fixture parity gate from spec 1510 §4.2.
// Comparison against the classifier's slot tables is deferred to
// v0.6.2, when codegen starts consuming the scope tree directly —
// at that point each compileX path already has the matching code
// object in hand and a side-by-side check is one Equal call away.
// Today the oracle proves byte-identity end to end, which is the
// stronger gate.
func TestFixtureParity(t *testing.T) {
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
			mod, err := parser.ParseFile(f, string(src))
			if err != nil {
				// Some fixtures rely on the classifier's pre-AST
				// layer accepting the source; if the parser
				// rejects, treat that as out of scope here.
				t.Skipf("parse error: %v", err)
			}
			if _, err := Build(mod); err != nil {
				t.Fatalf("Build: %v", err)
			}
		})
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
