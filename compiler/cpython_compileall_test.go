package compiler

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	parser "github.com/tamnd/gopapy/parser"

	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/codegen"
	"github.com/tamnd/gocopy/compiler/lower"
	"github.com/tamnd/gocopy/compiler/optimize"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// TestCompileallLib is the v0.7.x stdlib-coverage tracker. Skipped
// unless GOCOPY_CPYTHON_LIB points at a CPython 3.14 Lib/ directory.
//
// When set: walk *.py files under that directory, run them through
// the visitor pipeline (codegen.Generate → optimize.Run →
// assemble.Assemble), and report counts for byte-identical / not-
// implemented / errored. The byte-identical comparison vs.
// `python3.14 -m py_compile` is added in v0.7.13 once the optimizer
// passes give the visitor a chance to match. At v0.7.0 the harness
// only counts pipeline outcomes — every fixture lands in the
// not-implemented bucket because Generate is a stub.
//
// Lib/test/ and Lib/lib2to3/tests/ are skipped — those subtrees are
// intentionally fragile (broken-by-design Python files used for
// CPython's own self-tests) and not productive parity targets.
//
// The test always passes; the count is the metric.
func TestCompileallLib(t *testing.T) {
	root := os.Getenv("GOCOPY_CPYTHON_LIB")
	if root == "" {
		t.Skip("GOCOPY_CPYTHON_LIB not set; skipping stdlib compileall harness")
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Skipf("GOCOPY_CPYTHON_LIB=%q is not a directory", root)
	}
	var (
		total          int
		notImplemented int
		errored        int
		generated      int
		skipped        int
	)
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if rel == "test" || strings.HasPrefix(rel, "test"+string(filepath.Separator)) {
				return filepath.SkipDir
			}
			if strings.Contains(rel, filepath.Join("lib2to3", "tests")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".py" {
			return nil
		}
		total++
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			errored++
			return nil
		}
		pmod, parseErr := parser.ParseFile(filepath.Base(path), string(src))
		if parseErr != nil {
			skipped++
			return nil
		}
		mod, lowerErr := lower.Lower(pmod)
		if lowerErr != nil {
			skipped++
			return nil
		}
		scope, symErr := symtable.Build(mod)
		if symErr != nil {
			skipped++
			return nil
		}
		seq, consts, names, genErr := codegen.Generate(mod, scope, codegen.GenerateOptions{
			Source:      src,
			Filename:    filepath.Base(path),
			Name:        "<module>",
			QualName:    "<module>",
			FirstLineNo: 1,
		})
		if genErr != nil {
			if errors.Is(genErr, codegen.ErrNotImplemented) {
				notImplemented++
			} else {
				errored++
			}
			return nil
		}
		seq = optimize.Run(seq)
		if _, asmErr := assemble.Assemble(seq, assemble.Options{
			Filename: filepath.Base(path),
			Name:     "<module>",
			QualName: "<module>",
			Consts:   consts,
			Names:    names,
		}); asmErr != nil {
			errored++
			return nil
		}
		generated++
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}
	t.Logf("compileall %s: total=%d generated=%d not-implemented=%d errored=%d skipped=%d",
		root, total, generated, notImplemented, errored, skipped)
}
