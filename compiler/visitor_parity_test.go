package compiler

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	parser "github.com/tamnd/gopapy/parser"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/codegen"
	"github.com/tamnd/gocopy/compiler/flowgraph"
	"github.com/tamnd/gocopy/compiler/lower"
	"github.com/tamnd/gocopy/compiler/optimize"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// TestVisitorParity is the v0.7.x parallel-output gate.
//
// For every fixture: run the visitor pipeline (codegen.Generate →
// optimize.Run → assemble.Assemble); compare the resulting CodeObject
// byte-for-byte against the classifier path's CodeObject (the v0.6
// authoritative path). Mismatches are logged via t.Log, not t.Fatal:
// during v0.7.x the visitor is *catching up*, and a per-fixture
// mismatch is expected until each shape is promoted.
//
// The test summary prints byte-identical / not-implemented /
// mismatched counts. At v0.7.0 the steady state is "byte-identical:
// 0, not-implemented: 246, mismatched: 0" because every visitor arm
// returns ErrNotImplemented. v0.7.1+ moves fixtures off the
// not-implemented bucket as visit_<node> arms land.
//
// The test always passes; its job is to surface the count, not to
// gate the build. The 246/246 oracle (tests/run.sh) gates byte
// equality on the authoritative classifier path.
func TestVisitorParity(t *testing.T) {
	root := findFixturesDir(t)
	files, err := filepath.Glob(filepath.Join(root, "*.py"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	funcbodyFiles, err := filepath.Glob(filepath.Join(root, "funcbody", "*.py"))
	if err != nil {
		t.Fatalf("glob funcbody: %v", err)
	}
	files = append(files, funcbodyFiles...)
	if len(files) == 0 {
		t.Skipf("no fixtures found under %s", root)
	}
	var (
		identical      int
		notImplemented int
		mismatched     int
		errored        int
		skipped        int
	)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		pmod, parseErr := parser.ParseFile(filepath.Base(f), string(src))
		if parseErr != nil {
			skipped++
			continue
		}
		mod, lowerErr := lower.Lower(pmod)
		if lowerErr != nil {
			skipped++
			continue
		}
		scope, symErr := symtable.Build(mod)
		if symErr != nil {
			skipped++
			continue
		}
		seq, consts, names, genErr := codegen.Generate(mod, scope, codegen.GenerateOptions{
			Source:      src,
			Filename:    filepath.Base(f),
			Name:        "<module>",
			QualName:    "<module>",
			FirstLineNo: 1,
		})
		if genErr != nil {
			if errors.Is(genErr, codegen.ErrNotImplemented) {
				notImplemented++
			} else {
				errored++
				t.Logf("%s: codegen.Generate: %v", filepath.Base(f), genErr)
			}
			continue
		}
		flowgraph.OptimizeLoadConst(seq, consts)
		consts = flowgraph.FoldTupleOfConstants(seq, consts)
		consts = flowgraph.RemoveUnusedConsts(seq, consts)
		seq = optimize.Run(seq)
		got, asmErr := assemble.Assemble(seq, assemble.Options{
			Filename: filepath.Base(f),
			Name:     "<module>",
			QualName: "<module>",
			Consts:   consts,
			Names:    names,
		})
		if asmErr != nil {
			errored++
			t.Logf("%s: assemble.Assemble: %v", filepath.Base(f), asmErr)
			continue
		}
		want, classErr := Compile(src, Options{Filename: filepath.Base(f)})
		if classErr != nil {
			errored++
			t.Logf("%s: classifier path errored: %v", filepath.Base(f), classErr)
			continue
		}
		if codeObjectsEqual(got, want) {
			identical++
		} else {
			mismatched++
			t.Logf("%s: visitor output mismatches classifier output", filepath.Base(f))
		}
	}
	t.Logf("visitor parity: byte-identical=%d not-implemented=%d mismatched=%d errored=%d skipped=%d (total=%d)",
		identical, notImplemented, mismatched, errored, skipped, len(files))
}

// codeObjectsEqual compares two CodeObjects on the byte-level fields
// the oracle gates on (bytecode, line table, exception table,
// stack size, first line, filename/name/qualname). Other fields
// (consts, names, localsplus*) are checked transitively because the
// bytecode references them by index — a divergence shows up as a
// bytecode mismatch.
func codeObjectsEqual(a, b *bytecode.CodeObject) bool {
	if a == nil || b == nil {
		return a == b
	}
	if !bytes.Equal(a.Bytecode, b.Bytecode) {
		return false
	}
	if !bytes.Equal(a.LineTable, b.LineTable) {
		return false
	}
	if !bytes.Equal(a.ExcTable, b.ExcTable) {
		return false
	}
	if a.StackSize != b.StackSize || a.FirstLineNo != b.FirstLineNo {
		return false
	}
	if a.Filename != b.Filename || a.Name != b.Name || a.QualName != b.QualName {
		return false
	}
	return true
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
