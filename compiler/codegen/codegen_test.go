package codegen

import (
	"bytes"
	"errors"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
)

// TestBuildEmptyModule covers the only shape v0.6.6 hands to the
// codegen path. The output must byte-equal the canonical empty
// module CodeObject the classifier emits today.
func TestBuildEmptyModule(t *testing.T) {
	mod := &ast.Module{}
	co, err := Build(mod, nil, Options{
		Filename:    "x.py",
		Name:        "<module>",
		QualName:    "<module>",
		FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(empty): %v", err)
	}
	if !bytes.Equal(co.Bytecode, bytecode.NoOpBytecode(1)) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, bytecode.NoOpBytecode(1))
	}
	if !bytes.Equal(co.LineTable, bytecode.LineTableEmpty()) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, bytecode.LineTableEmpty())
	}
	if len(co.ExcTable) != 0 {
		t.Fatalf("exctable = %x, want empty", co.ExcTable)
	}
	if co.StackSize != 1 {
		t.Fatalf("stacksize = %d, want 1", co.StackSize)
	}
	if co.FirstLineNo != 1 {
		t.Fatalf("firstlineno = %d, want 1", co.FirstLineNo)
	}
	if co.Filename != "x.py" || co.Name != "<module>" || co.QualName != "<module>" {
		t.Fatalf("metadata = %q/%q/%q, want x.py/<module>/<module>",
			co.Filename, co.Name, co.QualName)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 0 {
		t.Fatalf("names = %v, want []", co.Names)
	}
}

// TestBuildSinglePass covers the simplest non-empty no-op shape.
func TestBuildSinglePass(t *testing.T) {
	src := []byte("pass\n")
	mod := &ast.Module{Body: []ast.Stmt{&ast.Pass{P: ast.Pos{Line: 1}}}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(single pass): %v", err)
	}
	wantBC := bytecode.NoOpBytecode(1)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.LineTableNoOps([]bytecode.NoOpStmt{{Line: 1, EndCol: 4}})
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if co.StackSize != 1 {
		t.Fatalf("stacksize = %d, want 1", co.StackSize)
	}
}

// TestBuildThreePass covers the multi-stmt no-op shape: each
// non-last statement contributes one NOP, the last contributes the
// LOAD_CONST + RETURN_VALUE pair.
func TestBuildThreePass(t *testing.T) {
	src := []byte("pass\npass\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Pass{P: ast.Pos{Line: 1}},
		&ast.Pass{P: ast.Pos{Line: 2}},
		&ast.Pass{P: ast.Pos{Line: 3}},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(three pass): %v", err)
	}
	wantBC := bytecode.NoOpBytecode(3)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.LineTableNoOps([]bytecode.NoOpStmt{
		{Line: 1, EndCol: 4},
		{Line: 2, EndCol: 4},
		{Line: 3, EndCol: 4},
	})
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildChainedAssignReturnsErrUnsupported guarantees the
// driver's fallback contract: shapes codegen does not yet own
// surface as ErrUnsupported so the classifier path takes over.
// Chained assignment (`a = b = 1`) is a separate modKind the
// classifier still owns.
func TestBuildChainedAssignReturnsErrUnsupported(t *testing.T) {
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{
				&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "a"},
				&ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "b"},
			},
			Value: &ast.Constant{P: ast.Pos{Line: 1, Col: 8}, Kind: "int", Value: int64(1)},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: []byte("a = b = 1\n"), Name: "<module>", QualName: "<module>",
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(chained assign) error = %v, want ErrUnsupported", err)
	}
}

// TestBuildSingleDocstring covers the simplest docstring shape: one
// string literal, no tail. All four post-RESUME instructions share
// the docstring's Loc — the encoder run-merges them into a single
// 4-unit entry that byte-equals bytecode.DocstringLineTable.
func TestBuildSingleDocstring(t *testing.T) {
	src := []byte("\"hi\"\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.ExprStmt{P: ast.Pos{Line: 1}, Value: &ast.Constant{
			P: ast.Pos{Line: 1, Col: 0}, Kind: "str", Value: "hi",
		}},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(single docstring): %v", err)
	}
	wantBC := bytecode.DocstringBytecode(0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.DocstringLineTable(1, 1, 4, nil)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 2 || co.Consts[0] != "hi" || co.Consts[1] != nil {
		t.Fatalf("consts = %v, want [\"hi\", nil]", co.Consts)
	}
	if len(co.Names) != 1 || co.Names[0] != "__doc__" {
		t.Fatalf("names = %v, want [__doc__]", co.Names)
	}
}

// TestBuildDocstringWithTail covers a docstring followed by trailing
// no-op statements. The trailing pair (LOAD_CONST None + RETURN_VALUE)
// shares the last tail stmt's Loc, not the docstring's.
func TestBuildDocstringWithTail(t *testing.T) {
	src := []byte("\"hi\"\npass\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.ExprStmt{P: ast.Pos{Line: 1}, Value: &ast.Constant{
			P: ast.Pos{Line: 1, Col: 0}, Kind: "str", Value: "hi",
		}},
		&ast.Pass{P: ast.Pos{Line: 2}},
		&ast.Pass{P: ast.Pos{Line: 3}},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(docstring+tail): %v", err)
	}
	wantBC := bytecode.DocstringBytecode(2)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.DocstringLineTable(1, 1, 4, []bytecode.NoOpStmt{
		{Line: 2, EndCol: 4},
		{Line: 3, EndCol: 4},
	})
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildMultiLineDocstring covers a triple-quoted docstring that
// spans several source lines. The docstring entry must be a LONG
// PEP 626 record (end_line != line) — the encoder picks this up
// automatically from the Loc the classifier reports.
func TestBuildMultiLineDocstring(t *testing.T) {
	src := []byte("\"\"\"line one\nline two\"\"\"\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.ExprStmt{P: ast.Pos{Line: 1}, Value: &ast.Constant{
			P: ast.Pos{Line: 1, Col: 0}, Kind: "str", Value: "line one\nline two",
		}},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(multi-line docstring): %v", err)
	}
	wantBC := bytecode.DocstringBytecode(0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.DocstringLineTable(1, 2, 11, nil)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildDocstringNonAsciiRejected ensures codegen mirrors the
// classifier's ASCII-only constraint: a non-ASCII docstring must
// fall back to the classifier path (ErrUnsupported) rather than
// produce a CodeObject.
func TestBuildDocstringNonAsciiRejected(t *testing.T) {
	src := []byte("\"héllo\"\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.ExprStmt{P: ast.Pos{Line: 1}, Value: &ast.Constant{
			P: ast.Pos{Line: 1, Col: 0}, Kind: "str", Value: "héllo",
		}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>", QualName: "<module>",
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(non-ascii) error = %v, want ErrUnsupported", err)
	}
}

// TestBuildAssignSmallInt covers the simplest int-assign shape:
// `x = 1` lowers to LOAD_SMALL_INT (oparg = the int value).
func TestBuildAssignSmallInt(t *testing.T) {
	src := []byte("x = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(1)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(small int): %v", err)
	}
	wantBC := bytecode.AssignSmallIntBytecode(1, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.AssignLineTable(1, 1, 4, 5, nil)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 2 || co.Consts[0] != int64(1) || co.Consts[1] != nil {
		t.Fatalf("consts = %v, want [1, nil]", co.Consts)
	}
	if len(co.Names) != 1 || co.Names[0] != "x" {
		t.Fatalf("names = %v, want [x]", co.Names)
	}
}

// TestBuildAssignLargeInt covers the LOAD_CONST path: an int >= 256
// can't fit in LOAD_SMALL_INT's byte oparg.
func TestBuildAssignLargeInt(t *testing.T) {
	src := []byte("x = 1000\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(1000)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(large int): %v", err)
	}
	wantBC := bytecode.AssignBytecode(1, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.AssignLineTable(1, 1, 4, 8, nil)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildAssignNone covers the consts-collapsed shape: `x = None`
// consts is just [nil] — both LOAD_CONSTs reference index 0.
func TestBuildAssignNone(t *testing.T) {
	src := []byte("x = None\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "None"},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(None): %v", err)
	}
	wantBC := bytecode.AssignBytecode(0, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
}

// TestBuildAssignWithTail covers a string assignment followed by
// trailing no-op statements.
func TestBuildAssignWithTail(t *testing.T) {
	src := []byte("name = \"hi\"\npass\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "name"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 7}, Kind: "str", Value: "hi"},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
		&ast.Pass{P: ast.Pos{Line: 3}},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(assign+tail): %v", err)
	}
	wantBC := bytecode.AssignBytecode(1, 2)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.AssignLineTable(1, 4, 7, 11, []bytecode.NoOpStmt{
		{Line: 2, EndCol: 4},
		{Line: 3, EndCol: 4},
	})
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildAssignNegLiteralFallsThrough ensures codegen rejects
// negative-literal assignments (UnaryOp wrapping a Constant) so the
// classifier's negLiteral path keeps owning that sub-shape.
func TestBuildAssignNegLiteralFallsThrough(t *testing.T) {
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.UnaryOp{
				P:       ast.Pos{Line: 1, Col: 4},
				Op:      "USub",
				Operand: &ast.Constant{P: ast.Pos{Line: 1, Col: 5}, Kind: "int", Value: int64(1)},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: []byte("x = -1\n"), Filename: "x.py",
		Name: "<module>", QualName: "<module>",
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(neg literal) error = %v, want ErrUnsupported", err)
	}
}

func TestBuildNilModule(t *testing.T) {
	_, err := Build(nil, nil, Options{})
	if err == nil {
		t.Fatalf("Build(nil) returned nil error")
	}
	if errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(nil) returned ErrUnsupported, want a distinct error")
	}
}
