package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParserImportBoundary enforces the v0.6.2 invariant: the only
// production files under compiler/ that import gopapy/parser are
//
//   - compiler.go            — orchestration root (calls ParseFile)
//   - compiler/lower/lower.go — the parser→ast translation boundary
//   - compiler/ast/ast.go     — the alias surface itself
//
// Anything else under compiler/ must speak ast types, not parser
// types. Test files are exempt: they call ParseFile to construct
// inputs and the constraint is on shipped code.
func TestParserImportBoundary(t *testing.T) {
	allowed := []string{
		"compiler/compiler.go",
		"compiler/lower/lower.go",
		"compiler/ast/ast.go",
	}
	const banned = `"github.com/tamnd/gopapy/parser"`

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	// Walk the package's parent — i.e., everything under
	// compiler/. filepath.Abs(".") points at the package dir, so
	// step up once to land at compiler/.
	root = filepath.Dir(filepath.Join(root, "x"))
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), banned) {
			return nil
		}
		for _, suf := range allowed {
			if strings.HasSuffix(path, suf) {
				return nil
			}
		}
		t.Errorf("file %s imports gopapy/parser — only %v may", path, allowed)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
