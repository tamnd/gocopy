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
	"github.com/tamnd/gocopy/compiler/codegen"
	"github.com/tamnd/gocopy/compiler/lower"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// TestCodegenParity is the v0.6.6 foundation parallel-output gate.
// For every fixture: if codegen.Build produces a CodeObject (i.e.
// the shape is one codegen owns), it must byte-equal the classifier
// path's CodeObject. Shapes codegen does not own (returns
// ErrUnsupported) skip — they exercise the classifier through
// compiler.Compile and the oracle covers them.
//
// The gate grows automatically: every release that teaches codegen
// a new shape lights up the corresponding fixtures here without
// touching this test.
func TestCodegenParity(t *testing.T) {
	root := findFixturesDir(t)
	files, err := filepath.Glob(filepath.Join(root, "*.py"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Skipf("no fixtures found under %s", root)
	}
	codegenHits := 0
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			pmod, parseErr := parser.ParseFile(filepath.Base(f), string(src))
			if parseErr != nil {
				t.Skipf("parser rejected: %v", parseErr)
			}
			mod, lowerErr := lower.Lower(pmod)
			if lowerErr != nil {
				t.Skipf("lower rejected: %v", lowerErr)
			}
			scope, _ := symtable.Build(mod) // best-effort, mirrors Compile
			co, cgErr := codegen.Build(mod, scope, codegen.Options{
				Source:      src,
				Filename:    filepath.Base(f),
				Name:        "<module>",
				QualName:    "<module>",
				FirstLineNo: 1,
			})
			if errors.Is(cgErr, codegen.ErrUnsupported) {
				return // shape not yet owned by codegen
			}
			if cgErr != nil {
				t.Fatalf("codegen.Build: %v", cgErr)
			}
			ref := compileViaClassifier(t, src, filepath.Base(f), mod)
			diffCodeObject(t, co, ref)
			codegenHits++
		})
	}
	if codegenHits == 0 {
		t.Fatalf("no fixture exercised the codegen path; the gate is dormant")
	}
}

// compileViaClassifier reproduces the classifier path inside Compile
// (skipping the codegen attempt) so the parity test can compare
// codegen output against a deterministic reference.
func compileViaClassifier(t *testing.T, source []byte, filename string, mod *parserModule) *bytecode.CodeObject {
	t.Helper()
	cls, ok := classifyAST(source, mod)
	if !ok {
		t.Fatalf("classifier rejected fixture")
	}
	switch cls.kind {
	case modAssign:
		lt := bytecode.AssignLineTable(cls.asgnLine, cls.asgnNameLen, cls.asgnValStart, cls.asgnValEnd, cls.stmts)
		if iv, ok := cls.asgnValue.(int64); ok {
			if iv >= 0 && iv <= 255 {
				return module(filename,
					bytecode.AssignSmallIntBytecode(byte(iv), len(cls.stmts)),
					lt, []any{iv, nil}, []string{cls.asgnName},
				)
			}
			return module(filename,
				bytecode.AssignBytecode(1, len(cls.stmts)),
				lt, []any{iv, nil}, []string{cls.asgnName},
			)
		}
		consts := []any{cls.asgnValue, nil}
		noneIdx := byte(1)
		if cls.asgnValue == nil {
			consts = []any{nil}
			noneIdx = 0
		}
		return module(filename,
			bytecode.AssignBytecode(noneIdx, len(cls.stmts)),
			lt, consts, []string{cls.asgnName},
		)
	case modMultiAssign:
		co, err := compileMultiAssign(filename, cls.asgns, cls.stmts)
		if err != nil {
			t.Fatalf("classifier compileMultiAssign: %v", err)
		}
		return co
	case modChainedAssign:
		co, err := compileChainedAssign(filename, cls.chainLine, cls.chainTargets, cls.chainValStart, cls.chainValEnd, cls.chainValue, cls.stmts)
		if err != nil {
			t.Fatalf("classifier compileChainedAssign: %v", err)
		}
		return co
	case modAugAssign:
		co, err := compileAugAssign(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileAugAssign: %v", err)
		}
		return co
	case modBinOpAssign:
		co, err := compileBinOpAssign(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileBinOpAssign: %v", err)
		}
		return co
	case modUnaryAssign:
		co, err := compileUnaryAssign(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileUnaryAssign: %v", err)
		}
		return co
	case modCmpAssign:
		co, err := compileCmpAssign(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileCmpAssign: %v", err)
		}
		return co
	case modBoolOp:
		co, err := compileBoolOp(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileBoolOp: %v", err)
		}
		return co
	case modTernary:
		co, err := compileTernary(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileTernary: %v", err)
		}
		return co
	case modCollection:
		co, err := compileCollection(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileCollection: %v", err)
		}
		return co
	case modSubscriptLoad:
		co, err := compileSubscriptLoad(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileSubscriptLoad: %v", err)
		}
		return co
	case modAttrLoad:
		co, err := compileAttrLoad(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileAttrLoad: %v", err)
		}
		return co
	case modSubscriptStore:
		co, err := compileSubscriptStore(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileSubscriptStore: %v", err)
		}
		return co
	case modAttrStore:
		co, err := compileAttrStore(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileAttrStore: %v", err)
		}
		return co
	case modCallAssign:
		co, err := compileCallAssign(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileCallAssign: %v", err)
		}
		return co
	case modGenExpr:
		co, err := compileGenExpr(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileGenExpr: %v", err)
		}
		return co
	case modIfElse:
		co, err := compileIfElse(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileIfElse: %v", err)
		}
		return co
	case modWhile:
		co, err := compileWhile(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileWhile: %v", err)
		}
		return co
	case modFor:
		co, err := compileFor(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileFor: %v", err)
		}
		return co
	case modFuncDef:
		co, err := compileFuncDef(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileFuncDef: %v", err)
		}
		return co
	case modClosureDef:
		co, err := compileClosure(filename, cls)
		if err != nil {
			t.Fatalf("classifier compileClosure: %v", err)
		}
		return co
	}
	t.Fatalf("classifier path for kind %d not exposed to parity test yet", cls.kind)
	return nil
}

// parserModule is the type Compile receives after lower.Lower; we
// alias it here so the helper signature reads naturally without
// importing the gopapy parser into every test that touches it.
type parserModule = parser.Module

func diffCodeObject(t *testing.T, got, want *bytecode.CodeObject) {
	t.Helper()
	if !bytes.Equal(got.Bytecode, want.Bytecode) {
		t.Fatalf("bytecode diverges\n got  %x\n want %x", got.Bytecode, want.Bytecode)
	}
	if !bytes.Equal(got.LineTable, want.LineTable) {
		t.Fatalf("linetable diverges\n got  %x\n want %x", got.LineTable, want.LineTable)
	}
	if !bytes.Equal(got.ExcTable, want.ExcTable) {
		t.Fatalf("exctable diverges (got %d bytes, want %d)", len(got.ExcTable), len(want.ExcTable))
	}
	if got.StackSize != want.StackSize {
		t.Fatalf("stacksize = %d, want %d", got.StackSize, want.StackSize)
	}
	if got.FirstLineNo != want.FirstLineNo {
		t.Fatalf("firstlineno = %d, want %d", got.FirstLineNo, want.FirstLineNo)
	}
	if got.Filename != want.Filename || got.Name != want.Name || got.QualName != want.QualName {
		t.Fatalf("metadata = %q/%q/%q, want %q/%q/%q",
			got.Filename, got.Name, got.QualName,
			want.Filename, want.Name, want.QualName)
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
