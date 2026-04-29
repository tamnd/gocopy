package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunShWalksFuncbody asserts tests/run.sh discovers fixtures
// under tests/fixtures/funcbody/. This guards against accidental
// regressions where a future refactor of run.sh drops the funcbody
// pass introduced in v0.7.10.1 (spec 1559).
func TestRunShWalksFuncbody(t *testing.T) {
	root := findRepoRoot(t)
	runSh := filepath.Join(root, "tests", "run.sh")
	body, err := os.ReadFile(runSh)
	if err != nil {
		t.Fatalf("read %s: %v", runSh, err)
	}
	src := string(body)
	if !strings.Contains(src, "tests/fixtures/funcbody/*.py") {
		t.Errorf("tests/run.sh does not contain a glob over tests/fixtures/funcbody/*.py")
	}
	if !strings.Contains(src, "tests/fixtures/*.py") {
		t.Errorf("tests/run.sh dropped its top-level tests/fixtures/*.py glob")
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cur := cwd
	for {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			t.Fatalf("could not locate repo root from %s", cwd)
		}
		cur = parent
	}
}
