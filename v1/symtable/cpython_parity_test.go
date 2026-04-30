package symtable

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	parser "github.com/tamnd/gopapy/parser"
)

// cpyScope is the JSON shape produced by the helper Python script.
// Annotation scopes (PEP 649) are stripped on the Python side so
// the gocopy tree, which does not yet model them, can be compared
// directly.
type cpyScope struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Children []*cpyScope `json:"children"`
}

// cpyDumpScript prints a JSON dump of the symtable for stdin
// source, dropping the annotation scopes CPython 3.14 inserts on
// every block. Only function / module / class / comprehension
// scopes survive.
const cpyDumpScript = `
import sys, json, symtable

def dump(t):
    kind = t.get_type()
    if hasattr(kind, 'value'):
        kind = kind.value
    if kind == 'annotation':
        return None
    return {
        'name': t.get_name(),
        'type': str(kind),
        'children': [c for c in (dump(x) for x in t.get_children()) if c is not None],
    }

src = sys.stdin.read()
t = symtable.symtable(src, '<test>', 'exec')
print(json.dumps(dump(t)))
`

// TestCPythonScopeTreeParity validates that the gocopy scope tree
// for each fixture matches python3.14's symtable.symtable result,
// modulo the synthetic __annotate__ scopes CPython 3.14 inserts
// for PEP 649. Skips when python3.14 is not on PATH so the basic
// CI test job stays green; the oracle CI job has Python.
func TestCPythonScopeTreeParity(t *testing.T) {
	if _, err := exec.LookPath("python3.14"); err != nil {
		t.Skip("python3.14 not on PATH")
	}
	root := findFixturesDir(t)
	if root == "" {
		return
	}
	files, err := filepath.Glob(filepath.Join(root, "*.py"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			cmd := exec.Command("python3.14", "-c", cpyDumpScript)
			cmd.Stdin = strings.NewReader(string(src))
			out, err := cmd.Output()
			if err != nil {
				t.Skipf("python3.14 symtable error: %v", err)
			}
			var want cpyScope
			if err := json.Unmarshal(out, &want); err != nil {
				t.Fatalf("decode python output: %v", err)
			}

			mod, err := parser.ParseFile(f, string(src))
			if err != nil {
				t.Skipf("gocopy parse error: %v", err)
			}
			got, err := Build(mod)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			compareScopes(t, &want, got, "")
		})
	}
}

// compareScopes walks the two trees in parallel and reports the
// first mismatch with a path prefix that names the offending scope.
func compareScopes(t *testing.T, want *cpyScope, got *Scope, path string) {
	t.Helper()
	gotName := got.Name
	if got.Kind == ScopeModule {
		gotName = "top"
	}
	wantName := want.Name
	if want.Type == "module" {
		wantName = "top"
	}
	pathHere := path + "/" + gotName

	if want.Type == "module" && got.Kind != ScopeModule {
		t.Fatalf("%s: kind mismatch: want module, got %s", pathHere, got.Kind)
	}
	if want.Type == "function" && got.Kind != ScopeFunction {
		t.Fatalf("%s: kind mismatch: want function, got %s", pathHere, got.Kind)
	}
	if wantName != gotName && !(want.Type == "module" && gotName == "top") {
		// Names should match for non-module scopes; modules use
		// the implicit "top" label both sides.
		if got.Kind != ScopeModule {
			t.Fatalf("%s: name mismatch: want %q, got %q", pathHere, wantName, gotName)
		}
	}
	if len(want.Children) != len(got.Children) {
		t.Fatalf("%s: child count mismatch: want %d (%v), got %d (%v)",
			pathHere, len(want.Children), childNames(want.Children),
			len(got.Children), gotChildNames(got.Children))
	}
	for i := range want.Children {
		compareScopes(t, want.Children[i], got.Children[i], pathHere)
	}
}

func childNames(cs []*cpyScope) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func gotChildNames(cs []*Scope) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

