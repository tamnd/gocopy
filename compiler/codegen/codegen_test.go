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

// TestBuildTupleAssignFallsThrough guarantees the driver's
// fallback contract: shapes codegen does not yet own surface as
// ErrUnsupported so the classifier path takes over. Tuple
// unpacking (`x, y = 1, 2`) has a single Tuple target, not a
// Name; the classifier still owns it.
func TestBuildTupleAssignFallsThrough(t *testing.T) {
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Tuple{
				P: ast.Pos{Line: 1, Col: 0},
				Elts: []ast.Expr{
					&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"},
					&ast.Name{P: ast.Pos{Line: 1, Col: 3}, Id: "y"},
				},
			}},
			Value: &ast.Tuple{
				P: ast.Pos{Line: 1, Col: 7},
				Elts: []ast.Expr{
					&ast.Constant{P: ast.Pos{Line: 1, Col: 7}, Kind: "int", Value: int64(1)},
					&ast.Constant{P: ast.Pos{Line: 1, Col: 10}, Kind: "int", Value: int64(2)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: []byte("x, y = 1, 2\n"), Name: "<module>", QualName: "<module>",
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(tuple assign) error = %v, want ErrUnsupported", err)
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

// TestBuildMultiAssignTwoSmallInts covers the simplest multi-assign
// shape: two `<name> = <small int>` statements. Both lower to
// LOAD_SMALL_INT; consts has a phantom slot for the first value.
func TestBuildMultiAssignTwoSmallInts(t *testing.T) {
	src := []byte("x = 1\ny = 2\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(1)},
		},
		&ast.Assign{
			P:       ast.Pos{Line: 2, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "y"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 4}, Kind: "int", Value: int64(2)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(multi small ints): %v", err)
	}
	wantLT := bytecode.MultiAssignLineTable([]bytecode.AssignInfo{
		{Line: 1, NameLen: 1, ValStart: 4, ValEnd: 5},
		{Line: 2, NameLen: 1, ValStart: 4, ValEnd: 5},
	}, nil)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 2 || co.Consts[0] != int64(1) || co.Consts[1] != nil {
		t.Fatalf("consts = %v, want [1, nil]", co.Consts)
	}
	if len(co.Names) != 2 || co.Names[0] != "x" || co.Names[1] != "y" {
		t.Fatalf("names = %v, want [x y]", co.Names)
	}
}

// TestBuildMultiAssignNoneAndString covers consts dedup and mixed
// non-int types: `x = None\ny = "hi"`.
func TestBuildMultiAssignNoneAndString(t *testing.T) {
	src := []byte("x = None\ny = \"hi\"\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "None"},
		},
		&ast.Assign{
			P:       ast.Pos{Line: 2, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "y"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 4}, Kind: "str", Value: "hi"},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(multi None+str): %v", err)
	}
	if len(co.Consts) != 2 || co.Consts[0] != nil || co.Consts[1] != "hi" {
		t.Fatalf("consts = %v, want [nil, \"hi\"]", co.Consts)
	}
}

// TestBuildMultiAssignWithTail covers multi-assign followed by
// trailing pass statements.
func TestBuildMultiAssignWithTail(t *testing.T) {
	src := []byte("x = 1\ny = 2\npass\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(1)},
		},
		&ast.Assign{
			P:       ast.Pos{Line: 2, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "y"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 4}, Kind: "int", Value: int64(2)},
		},
		&ast.Pass{P: ast.Pos{Line: 3}},
		&ast.Pass{P: ast.Pos{Line: 4}},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(multi+tail): %v", err)
	}
	wantLT := bytecode.MultiAssignLineTable([]bytecode.AssignInfo{
		{Line: 1, NameLen: 1, ValStart: 4, ValEnd: 5},
		{Line: 2, NameLen: 1, ValStart: 4, ValEnd: 5},
	}, []bytecode.NoOpStmt{
		{Line: 3, EndCol: 4},
		{Line: 4, EndCol: 4},
	})
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildChainedAssignTwoTargets covers `x = y = 1`. CPython
// emits LOAD_SMALL_INT, COPY 1, STORE_NAME x, STORE_NAME y, then
// the trailing return pair. StackSize is 2.
func TestBuildChainedAssignTwoTargets(t *testing.T) {
	src := []byte("x = y = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{
				&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"},
				&ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "y"},
			},
			Value: &ast.Constant{P: ast.Pos{Line: 1, Col: 8}, Kind: "int", Value: int64(1)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(chain 2): %v", err)
	}
	wantLT := bytecode.ChainedAssignLineTable(1, []bytecode.ChainedTarget{
		{NameStart: 0, NameLen: 1},
		{NameStart: 4, NameLen: 1},
	}, 8, 9, nil)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
	if len(co.Names) != 2 || co.Names[0] != "x" || co.Names[1] != "y" {
		t.Fatalf("names = %v, want [x y]", co.Names)
	}
}

// TestBuildChainedAssignThreeTargets covers `a = b = c = 1`.
func TestBuildChainedAssignThreeTargets(t *testing.T) {
	src := []byte("a = b = c = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{
				&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "a"},
				&ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "b"},
				&ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "c"},
			},
			Value: &ast.Constant{P: ast.Pos{Line: 1, Col: 12}, Kind: "int", Value: int64(1)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(chain 3): %v", err)
	}
	if len(co.Names) != 3 {
		t.Fatalf("names = %v, want 3 entries", co.Names)
	}
}

// TestBuildChainedAssignNone covers consts collapse: `x = y = None`
// has consts = [nil] (LOAD_CONST 0 reused for the trailing return).
func TestBuildChainedAssignNone(t *testing.T) {
	src := []byte("x = y = None\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{
				&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"},
				&ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "y"},
			},
			Value: &ast.Constant{P: ast.Pos{Line: 1, Col: 8}, Kind: "None"},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(chain None): %v", err)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
}

// TestBuildAugAssignSmallSmall covers the canonical aug-assign
// shape: `x = 0; x += 1`. Both values are LOAD_SMALL_INT; consts =
// [0, nil] (phantom slot for init, None last).
func TestBuildAugAssignSmallSmall(t *testing.T) {
	src := []byte("x = 0\nx += 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(0)},
		},
		&ast.AugAssign{
			P:      ast.Pos{Line: 2, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "x"},
			Op:     "Add",
			Value:  &ast.Constant{P: ast.Pos{Line: 2, Col: 5}, Kind: "int", Value: int64(1)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(aug small/small): %v", err)
	}
	wantBC := bytecode.AugAssignBytecode(0, 1, bytecode.NbInplaceAdd, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.AugAssignLineTable(1, 1, 4, 5, 2, 5, 6, nil)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 2 || co.Consts[0] != int64(0) || co.Consts[1] != nil {
		t.Fatalf("consts = %v, want [0, nil]", co.Consts)
	}
	if len(co.Names) != 1 || co.Names[0] != "x" {
		t.Fatalf("names = %v, want [x]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildAugAssignSmallLarge covers `x = 0; x += 1000`: the aug
// value is too large for LOAD_SMALL_INT and lands at consts[1].
func TestBuildAugAssignSmallLarge(t *testing.T) {
	src := []byte("x = 0\nx += 1000\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(0)},
		},
		&ast.AugAssign{
			P:      ast.Pos{Line: 2, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "x"},
			Op:     "Add",
			Value:  &ast.Constant{P: ast.Pos{Line: 2, Col: 5}, Kind: "int", Value: int64(1000)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(aug small/large): %v", err)
	}
	wantBC := bytecode.AugAssignBytecode(0, 1000, bytecode.NbInplaceAdd, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	if len(co.Consts) != 3 || co.Consts[0] != int64(0) ||
		co.Consts[1] != int64(1000) || co.Consts[2] != nil {
		t.Fatalf("consts = %v, want [0, 1000, nil]", co.Consts)
	}
}

// TestBuildAugAssignLargeSmall covers `x = 1000; x += 1`: the init
// value is the LOAD_CONST 0 path; aug stays small.
func TestBuildAugAssignLargeSmall(t *testing.T) {
	src := []byte("x = 1000\nx += 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(1000)},
		},
		&ast.AugAssign{
			P:      ast.Pos{Line: 2, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "x"},
			Op:     "Add",
			Value:  &ast.Constant{P: ast.Pos{Line: 2, Col: 5}, Kind: "int", Value: int64(1)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(aug large/small): %v", err)
	}
	wantBC := bytecode.AugAssignBytecode(1000, 1, bytecode.NbInplaceAdd, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	if len(co.Consts) != 2 || co.Consts[0] != int64(1000) || co.Consts[1] != nil {
		t.Fatalf("consts = %v, want [1000, nil]", co.Consts)
	}
}

// TestBuildAugAssignSub covers operator dispatch via augOpargFromOp:
// `x = 0; x -= 1` produces BINARY_OP NbInplaceSubtract.
func TestBuildAugAssignSub(t *testing.T) {
	src := []byte("x = 0\nx -= 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(0)},
		},
		&ast.AugAssign{
			P:      ast.Pos{Line: 2, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "x"},
			Op:     "Sub",
			Value:  &ast.Constant{P: ast.Pos{Line: 2, Col: 5}, Kind: "int", Value: int64(1)},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(aug -=): %v", err)
	}
	wantBC := bytecode.AugAssignBytecode(0, 1, bytecode.NbInplaceSubtract, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
}

// TestBuildAugAssignWithTail covers aug-assign followed by a single
// tail no-op: the trailing pair shifts to the tail's Loc and the
// final STORE_NAME shrinks to a 1-unit entry.
func TestBuildAugAssignWithTail(t *testing.T) {
	src := []byte("x = 0\nx += 1\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "int", Value: int64(0)},
		},
		&ast.AugAssign{
			P:      ast.Pos{Line: 2, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "x"},
			Op:     "Add",
			Value:  &ast.Constant{P: ast.Pos{Line: 2, Col: 5}, Kind: "int", Value: int64(1)},
		},
		&ast.Pass{P: ast.Pos{Line: 3}},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(aug+tail): %v", err)
	}
	wantLT := bytecode.AugAssignLineTable(
		1, 1, 4, 5, 2, 5, 6,
		[]bytecode.NoOpStmt{{Line: 3, EndCol: 4}},
	)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildBinOpAssignAdd covers the canonical binop shape:
// `x = a + b`. Both operands are LOAD_NAME; consts = [None].
func TestBuildBinOpAssignAdd(t *testing.T) {
	src := []byte("x = a + b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 4},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Op:    "Add",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(binop add): %v", err)
	}
	wantBC := bytecode.BinOpAssignBytecode(bytecode.NbAdd)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.BinOpAssignLineTable(1, 4, 1, 8, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [a b x]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildBinOpAssignSub covers operator dispatch via
// binOpargFromOp: `x = a - b` produces BINARY_OP NbSubtract.
func TestBuildBinOpAssignSub(t *testing.T) {
	src := []byte("x = a - b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 4},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Op:    "Sub",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(binop sub): %v", err)
	}
	wantBC := bytecode.BinOpAssignBytecode(bytecode.NbSubtract)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
}

// TestBuildBinOpAssignBitwise covers `x = a & b`: same cache
// zero-fill path, different oparg.
func TestBuildBinOpAssignBitwise(t *testing.T) {
	src := []byte("x = a & b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 4},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Op:    "BitAnd",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(binop bitand): %v", err)
	}
	wantBC := bytecode.BinOpAssignBytecode(bytecode.NbAnd)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
}

// TestBuildBinOpAssignMultiCharNames covers `result = lhs + rhs`:
// names longer than 1 char must still SHORT0-encode (lengths
// 1..15 stay in range).
func TestBuildBinOpAssignMultiCharNames(t *testing.T) {
	src := []byte("result = lhs + rhs\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "result"}},
			Value: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 9},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "lhs"},
				Op:    "Add",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 15}, Id: "rhs"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(binop multichar): %v", err)
	}
	wantLT := bytecode.BinOpAssignLineTable(1, 9, 3, 15, 3, 6)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "lhs" || co.Names[1] != "rhs" || co.Names[2] != "result" {
		t.Fatalf("names = %v, want [lhs rhs result]", co.Names)
	}
}

// TestBuildBinOpAssignWithTailFallsThrough ensures codegen rejects
// trailing no-ops — the v0.0.16 BinOpAssignBytecode helper has no
// slot for them, so multi-statement modules must fall through to
// the classifier path to stay byte-equal.
func TestBuildBinOpAssignWithTailFallsThrough(t *testing.T) {
	src := []byte("x = a + b\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 4},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Op:    "Add",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(binop+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildUnaryAssignNeg covers `x = -a`: UNARY_NEGATIVE on a
// name operand. co_names = [a, x], stacksize = 1.
func TestBuildUnaryAssignNeg(t *testing.T) {
	src := []byte("x = -a\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.UnaryOp{
				P:       ast.Pos{Line: 1, Col: 4},
				Op:      "USub",
				Operand: &ast.Name{P: ast.Pos{Line: 1, Col: 5}, Id: "a"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(unary -): %v", err)
	}
	wantBC := bytecode.UnaryNegInvertBytecode(bytecode.UNARY_NEGATIVE)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.UnaryNegInvertLineTable(1, 4, 5, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 2 || co.Names[0] != "a" || co.Names[1] != "x" {
		t.Fatalf("names = %v, want [a x]", co.Names)
	}
	if co.StackSize < 1 {
		t.Fatalf("stacksize = %d, want >= 1", co.StackSize)
	}
}

// TestBuildUnaryAssignInvert covers `x = ~a`: UNARY_INVERT path
// (operator dispatch through the same Neg/Invert helper).
func TestBuildUnaryAssignInvert(t *testing.T) {
	src := []byte("x = ~a\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.UnaryOp{
				P:       ast.Pos{Line: 1, Col: 4},
				Op:      "Invert",
				Operand: &ast.Name{P: ast.Pos{Line: 1, Col: 5}, Id: "a"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(unary ~): %v", err)
	}
	wantBC := bytecode.UnaryNegInvertBytecode(bytecode.UNARY_INVERT)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
}

// TestBuildUnaryAssignNot covers `x = not a`: TO_BOOL (with 3 cache
// words) followed by UNARY_NOT, sharing one Loc so the encoder
// merges them into a single 5-unit linetable entry.
func TestBuildUnaryAssignNot(t *testing.T) {
	src := []byte("x = not a\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.UnaryOp{
				P:       ast.Pos{Line: 1, Col: 4},
				Op:      "Not",
				Operand: &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "a"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(unary not): %v", err)
	}
	wantBC := bytecode.UnaryNotBytecode()
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.UnaryNotLineTable(1, 4, 8, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildUnaryAssignNotMultiCharNames covers
// `result = not operand`: multi-char names in the Not path.
func TestBuildUnaryAssignNotMultiCharNames(t *testing.T) {
	src := []byte("result = not operand\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "result"}},
			Value: &ast.UnaryOp{
				P:       ast.Pos{Line: 1, Col: 9},
				Op:      "Not",
				Operand: &ast.Name{P: ast.Pos{Line: 1, Col: 13}, Id: "operand"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(unary not multichar): %v", err)
	}
	wantLT := bytecode.UnaryNotLineTable(1, 9, 13, 7, 6)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 2 || co.Names[0] != "operand" || co.Names[1] != "result" {
		t.Fatalf("names = %v, want [operand result]", co.Names)
	}
}

// TestBuildUnaryAssignWithTailFallsThrough ensures codegen rejects
// trailing no-ops (the v0.0.16 unary helpers have no slot for them).
func TestBuildUnaryAssignWithTailFallsThrough(t *testing.T) {
	src := []byte("x = -a\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.UnaryOp{
				P:       ast.Pos{Line: 1, Col: 4},
				Op:      "USub",
				Operand: &ast.Name{P: ast.Pos{Line: 1, Col: 5}, Id: "a"},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(unary+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildCmpAssignLt covers the canonical cmp shape:
// `x = a < b`. COMPARE_OP with CmpLt, 1 cache word, names = [a, b, x].
func TestBuildCmpAssignLt(t *testing.T) {
	src := []byte("x = a < b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Compare{
				P:           ast.Pos{Line: 1, Col: 4},
				Left:        &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Ops:         []string{"Lt"},
				Comparators: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(cmp <): %v", err)
	}
	wantBC := bytecode.CmpAssignBytecode(bytecode.COMPARE_OP, bytecode.CmpLt)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.CmpAssignLineTable(bytecode.COMPARE_OP, 1, 4, 1, 8, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [a b x]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildCmpAssignEq covers operator dispatch via cmpOpFromAstOp:
// `x = a == b` produces COMPARE_OP with CmpEq.
func TestBuildCmpAssignEq(t *testing.T) {
	src := []byte("x = a == b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Compare{
				P:           ast.Pos{Line: 1, Col: 4},
				Left:        &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Ops:         []string{"Eq"},
				Comparators: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "b"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(cmp ==): %v", err)
	}
	wantBC := bytecode.CmpAssignBytecode(bytecode.COMPARE_OP, bytecode.CmpEq)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
}

// TestBuildCmpAssignIs covers the IS_OP path: `x = a is b`. IS_OP
// has 0 cache words so the bytecode is two bytes shorter than the
// COMPARE_OP form. Confirms codegen lets the IR encoder decide the
// cache count from bytecode.CacheSize.
func TestBuildCmpAssignIs(t *testing.T) {
	src := []byte("x = a is b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Compare{
				P:           ast.Pos{Line: 1, Col: 4},
				Left:        &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Ops:         []string{"Is"},
				Comparators: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "b"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(cmp is): %v", err)
	}
	wantBC := bytecode.CmpAssignBytecode(bytecode.IS_OP, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.CmpAssignLineTable(bytecode.IS_OP, 1, 4, 1, 9, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildCmpAssignIsNot exercises the IS_OP / oparg=1 path:
// `x = a is not b`.
func TestBuildCmpAssignIsNot(t *testing.T) {
	src := []byte("x = a is not b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Compare{
				P:           ast.Pos{Line: 1, Col: 4},
				Left:        &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Ops:         []string{"IsNot"},
				Comparators: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 13}, Id: "b"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(cmp is not): %v", err)
	}
	wantBC := bytecode.CmpAssignBytecode(bytecode.IS_OP, 1)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
}

// TestBuildCmpAssignIn exercises the CONTAINS_OP / oparg=0 path:
// `x = a in b`.
func TestBuildCmpAssignIn(t *testing.T) {
	src := []byte("x = a in b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Compare{
				P:           ast.Pos{Line: 1, Col: 4},
				Left:        &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Ops:         []string{"In"},
				Comparators: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "b"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(cmp in): %v", err)
	}
	wantBC := bytecode.CmpAssignBytecode(bytecode.CONTAINS_OP, 0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.CmpAssignLineTable(bytecode.CONTAINS_OP, 1, 4, 1, 9, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildCmpAssignNotIn exercises the CONTAINS_OP / oparg=1 path:
// `x = a not in b`.
func TestBuildCmpAssignNotIn(t *testing.T) {
	src := []byte("x = a not in b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Compare{
				P:           ast.Pos{Line: 1, Col: 4},
				Left:        &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Ops:         []string{"NotIn"},
				Comparators: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 13}, Id: "b"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(cmp not in): %v", err)
	}
	wantBC := bytecode.CmpAssignBytecode(bytecode.CONTAINS_OP, 1)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
}

// TestBuildCmpAssignMultiCharNames covers `result = left < right`:
// names longer than 1 char must still SHORT0-encode.
func TestBuildCmpAssignMultiCharNames(t *testing.T) {
	src := []byte("result = left < right\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "result"}},
			Value: &ast.Compare{
				P:           ast.Pos{Line: 1, Col: 9},
				Left:        &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "left"},
				Ops:         []string{"Lt"},
				Comparators: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 16}, Id: "right"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(cmp multichar): %v", err)
	}
	wantLT := bytecode.CmpAssignLineTable(bytecode.COMPARE_OP, 1, 9, 4, 16, 5, 6)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "left" || co.Names[1] != "right" || co.Names[2] != "result" {
		t.Fatalf("names = %v, want [left right result]", co.Names)
	}
}

// TestBuildCmpAssignWithTailFallsThrough ensures codegen rejects
// trailing no-ops — the v0.0.x CmpAssignBytecode helper has no slot
// for them, so multi-statement modules must fall through to the
// classifier path.
func TestBuildCmpAssignWithTailFallsThrough(t *testing.T) {
	src := []byte("x = a < b\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Compare{
				P:           ast.Pos{Line: 1, Col: 4},
				Left:        &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Ops:         []string{"Lt"},
				Comparators: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"}},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(cmp+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildBoolOpAssignAnd covers `x = a and b`: POP_JUMP_IF_FALSE
// short-circuit, names = [a, b, x], stacksize = 2.
func TestBuildBoolOpAssignAnd(t *testing.T) {
	src := []byte("x = a and b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BoolOp{
				P:  ast.Pos{Line: 1, Col: 4},
				Op: "And",
				Values: []ast.Expr{
					&ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
					&ast.Name{P: ast.Pos{Line: 1, Col: 10}, Id: "b"},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(bool and): %v", err)
	}
	wantBC := bytecode.BoolAndBytecode()
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.BoolAndOrLineTable(1, 4, 1, 10, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [a b x]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildBoolOpAssignOr covers operator dispatch via the IsOr
// flag: `x = a or b` produces POP_JUMP_IF_TRUE in place of
// POP_JUMP_IF_FALSE.
func TestBuildBoolOpAssignOr(t *testing.T) {
	src := []byte("x = a or b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BoolOp{
				P:  ast.Pos{Line: 1, Col: 4},
				Op: "Or",
				Values: []ast.Expr{
					&ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
					&ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "b"},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(bool or): %v", err)
	}
	wantBC := bytecode.BoolOrBytecode()
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.BoolAndOrLineTable(1, 4, 1, 9, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
}

// TestBuildBoolOpAssignMultiCharNames covers `result = left and right`:
// names longer than 1 char must still SHORT-encode through the run
// merge / split-at-8 boundary.
func TestBuildBoolOpAssignMultiCharNames(t *testing.T) {
	src := []byte("result = left and right\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "result"}},
			Value: &ast.BoolOp{
				P:  ast.Pos{Line: 1, Col: 9},
				Op: "And",
				Values: []ast.Expr{
					&ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "left"},
					&ast.Name{P: ast.Pos{Line: 1, Col: 18}, Id: "right"},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(bool multichar): %v", err)
	}
	wantLT := bytecode.BoolAndOrLineTable(1, 9, 4, 18, 5, 6)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "left" || co.Names[1] != "right" || co.Names[2] != "result" {
		t.Fatalf("names = %v, want [left right result]", co.Names)
	}
}

// TestBuildBoolOpAssignWithTailFallsThrough ensures codegen rejects
// trailing no-ops — the v0.0.x BoolAndBytecode helper has no slot
// for them, so multi-statement modules must fall through to the
// classifier path.
func TestBuildBoolOpAssignWithTailFallsThrough(t *testing.T) {
	src := []byte("x = a and b\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BoolOp{
				P:  ast.Pos{Line: 1, Col: 4},
				Op: "And",
				Values: []ast.Expr{
					&ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
					&ast.Name{P: ast.Pos{Line: 1, Col: 10}, Id: "b"},
				},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(bool+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildTernaryAssignBasic covers `x = a if c else b`:
// POP_JUMP_IF_FALSE 5 + two return-bearing branches.
func TestBuildTernaryAssignBasic(t *testing.T) {
	src := []byte("x = a if c else b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.IfExp{
				P:      ast.Pos{Line: 1, Col: 4},
				Test:   &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "c"},
				Body:   &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				OrElse: &ast.Name{P: ast.Pos{Line: 1, Col: 16}, Id: "b"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(ternary): %v", err)
	}
	wantBC := bytecode.TernaryBytecode()
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.TernaryLineTable(1, 9, 1, 4, 1, 16, 1, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 4 || co.Names[0] != "c" || co.Names[1] != "a" || co.Names[2] != "b" || co.Names[3] != "x" {
		t.Fatalf("names = %v, want [c a b x]", co.Names)
	}
	if co.StackSize < 1 {
		t.Fatalf("stacksize = %d, want >= 1", co.StackSize)
	}
}

// TestBuildTernaryAssignMultiCharNames exercises the column
// arithmetic with names longer than one char.
func TestBuildTernaryAssignMultiCharNames(t *testing.T) {
	// `result = trueVal if cond else falseVal`
	//  0       9       17 20    25   30
	src := []byte("result = trueVal if cond else falseVal\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "result"}},
			Value: &ast.IfExp{
				P:      ast.Pos{Line: 1, Col: 9},
				Test:   &ast.Name{P: ast.Pos{Line: 1, Col: 20}, Id: "cond"},
				Body:   &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "trueVal"},
				OrElse: &ast.Name{P: ast.Pos{Line: 1, Col: 30}, Id: "falseVal"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(ternary multichar): %v", err)
	}
	wantLT := bytecode.TernaryLineTable(1, 20, 4, 9, 7, 30, 8, 6)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 4 || co.Names[0] != "cond" || co.Names[1] != "trueVal" || co.Names[2] != "falseVal" || co.Names[3] != "result" {
		t.Fatalf("names = %v, want [cond trueVal falseVal result]", co.Names)
	}
}

// TestBuildTernaryAssignWithTailFallsThrough ensures codegen rejects
// trailing no-ops — the v0.0.x TernaryBytecode helper has no slot
// for them, so multi-statement modules must fall through to the
// classifier path.
func TestBuildTernaryAssignWithTailFallsThrough(t *testing.T) {
	src := []byte("x = a if c else b\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.IfExp{
				P:      ast.Pos{Line: 1, Col: 4},
				Test:   &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "c"},
				Body:   &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				OrElse: &ast.Name{P: ast.Pos{Line: 1, Col: 16}, Id: "b"},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(ternary+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildCollectionEmptyList covers `x = []` — BUILD_LIST 0,
// co_consts = [None], co_names = [x].
func TestBuildCollectionEmptyList(t *testing.T) {
	src := []byte("x = []\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.List{P: ast.Pos{Line: 1, Col: 4}},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(empty list): %v", err)
	}
	wantBC := bytecode.CollectionEmptyBytecode(bytecode.CollList)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.CollectionEmptyLineTable(1, 4, 6, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 1 || co.Names[0] != "x" {
		t.Fatalf("names = %v, want [x]", co.Names)
	}
}

// TestBuildCollectionEmptyTuple covers `x = ()` — LOAD_CONST 1,
// co_consts = [None, ConstTuple{}].
func TestBuildCollectionEmptyTuple(t *testing.T) {
	src := []byte("x = ()\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Tuple{P: ast.Pos{Line: 1, Col: 4}},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(empty tuple): %v", err)
	}
	wantBC := bytecode.CollectionEmptyBytecode(bytecode.CollTuple)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	if len(co.Consts) != 2 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil, ConstTuple{}]", co.Consts)
	}
	if _, ok := co.Consts[1].(bytecode.ConstTuple); !ok {
		t.Fatalf("consts[1] = %T, want bytecode.ConstTuple", co.Consts[1])
	}
}

// TestBuildCollectionEmptyDict covers `x = {}` — BUILD_MAP 0.
func TestBuildCollectionEmptyDict(t *testing.T) {
	src := []byte("x = {}\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Dict{P: ast.Pos{Line: 1, Col: 4}},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(empty dict): %v", err)
	}
	wantBC := bytecode.CollectionEmptyBytecode(bytecode.CollDict)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
}

// TestBuildCollectionListNames covers `x = [a, b]` — two LOAD_NAME
// instructions then BUILD_LIST 2, co_names = [a, b, x],
// co.StackSize = 2.
func TestBuildCollectionListNames(t *testing.T) {
	src := []byte("x = [a, b]\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.List{
				P: ast.Pos{Line: 1, Col: 4},
				Elts: []ast.Expr{
					&ast.Name{P: ast.Pos{Line: 1, Col: 5}, Id: "a"},
					&ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(list names): %v", err)
	}
	wantBC := bytecode.CollectionNamesBytecode(bytecode.CollList, 2)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantElts := []bytecode.CollElt{
		{Name: "a", Col: 5, NameLen: 1},
		{Name: "b", Col: 8, NameLen: 1},
	}
	wantLT := bytecode.CollectionNamesLineTable(1, wantElts, 4, 10, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [a b x]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildCollectionDictNames covers `x = {a: b}` — flattened
// key/value LOAD_NAMEs then BUILD_MAP 1.
func TestBuildCollectionDictNames(t *testing.T) {
	src := []byte("x = {a: b}\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Dict{
				P:      ast.Pos{Line: 1, Col: 4},
				Keys:   []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 5}, Id: "a"}},
				Values: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(dict names): %v", err)
	}
	wantBC := bytecode.CollectionNamesBytecode(bytecode.CollDict, 2)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [a b x]", co.Names)
	}
}

// TestBuildCollectionWithTailFallsThrough ensures codegen rejects
// trailing no-ops — the classifier helpers have no slot for them.
func TestBuildCollectionWithTailFallsThrough(t *testing.T) {
	src := []byte("x = [a, b]\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.List{
				P: ast.Pos{Line: 1, Col: 4},
				Elts: []ast.Expr{
					&ast.Name{P: ast.Pos{Line: 1, Col: 5}, Id: "a"},
					&ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
				},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(coll+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildSubscriptLoad covers `x = a[b]`: BINARY_OP NbGetItem with
// 5 cache words, names = [a, b, x], stacksize = 2.
func TestBuildSubscriptLoad(t *testing.T) {
	src := []byte("x = a[b]\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Subscript{
				P:     ast.Pos{Line: 1, Col: 4},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Slice: &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "b"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(subscript load): %v", err)
	}
	wantBC := bytecode.SubscriptLoadBytecode()
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.SubscriptLoadLineTable(1, 4, 5, 6, 7, 8, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [a b x]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildSubscriptLoadMultiCharNames exercises wider names and
// columns to verify the column arithmetic in the line table.
func TestBuildSubscriptLoadMultiCharNames(t *testing.T) {
	// `result = arr[idx]`
	//  0       9   13 14 17
	src := []byte("result = arr[idx]\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "result"}},
			Value: &ast.Subscript{
				P:     ast.Pos{Line: 1, Col: 9},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "arr"},
				Slice: &ast.Name{P: ast.Pos{Line: 1, Col: 13}, Id: "idx"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(subscript multichar): %v", err)
	}
	wantLT := bytecode.SubscriptLoadLineTable(1, 9, 12, 13, 16, 17, 6)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "arr" || co.Names[1] != "idx" || co.Names[2] != "result" {
		t.Fatalf("names = %v, want [arr idx result]", co.Names)
	}
}

// TestBuildSubscriptLoadWithTailFallsThrough ensures codegen rejects
// trailing no-ops — the SubscriptLoadBytecode helper has no slot for
// them.
func TestBuildSubscriptLoadWithTailFallsThrough(t *testing.T) {
	src := []byte("x = a[b]\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Subscript{
				P:     ast.Pos{Line: 1, Col: 4},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Slice: &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "b"},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(subscript+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildAttrLoad covers `x = a.b`: LOAD_ATTR with 9 cache words
// (10-unit run split 8+2 in the line table), names = [a, b, x],
// stacksize = 1.
func TestBuildAttrLoad(t *testing.T) {
	src := []byte("x = a.b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Attribute{
				P:     ast.Pos{Line: 1, Col: 4},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Attr:  "b",
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(attr load): %v", err)
	}
	wantBC := bytecode.AttrLoadBytecode()
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.AttrLoadLineTable(1, 4, 5, 7, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [a b x]", co.Names)
	}
	if co.StackSize < 1 {
		t.Fatalf("stacksize = %d, want >= 1", co.StackSize)
	}
}

// TestBuildAttrLoadMultiCharNames exercises wider names and columns.
func TestBuildAttrLoadMultiCharNames(t *testing.T) {
	// `result = obj.field`
	//  0       9   13 14
	src := []byte("result = obj.field\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "result"}},
			Value: &ast.Attribute{
				P:     ast.Pos{Line: 1, Col: 9},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "obj"},
				Attr:  "field",
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(attr multichar): %v", err)
	}
	wantLT := bytecode.AttrLoadLineTable(1, 9, 12, 18, 6)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "obj" || co.Names[1] != "field" || co.Names[2] != "result" {
		t.Fatalf("names = %v, want [obj field result]", co.Names)
	}
}

// TestBuildAttrLoadWithTailFallsThrough ensures codegen rejects
// trailing no-ops — the AttrLoadBytecode helper has no slot for them.
func TestBuildAttrLoadWithTailFallsThrough(t *testing.T) {
	src := []byte("x = a.b\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Attribute{
				P:     ast.Pos{Line: 1, Col: 4},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Attr:  "b",
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(attr+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildSubscriptStore covers `a[b] = x`: STORE_SUBSCR with 1
// cache word, names = [x, a, b], stacksize = 3.
func TestBuildSubscriptStore(t *testing.T) {
	src := []byte("a[b] = x\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Subscript{
				P:     ast.Pos{Line: 1, Col: 0},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "a"},
				Slice: &ast.Name{P: ast.Pos{Line: 1, Col: 2}, Id: "b"},
			}},
			Value: &ast.Name{P: ast.Pos{Line: 1, Col: 7}, Id: "x"},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(subscript store): %v", err)
	}
	wantBC := bytecode.SubscriptStoreBytecode()
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.SubscriptStoreLineTable(1, 7, 8, 0, 1, 2, 3, 4)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 3 || co.Names[0] != "x" || co.Names[1] != "a" || co.Names[2] != "b" {
		t.Fatalf("names = %v, want [x a b]", co.Names)
	}
	if co.StackSize < 3 {
		t.Fatalf("stacksize = %d, want >= 3", co.StackSize)
	}
}

// TestBuildSubscriptStoreMultiCharNames exercises the column
// arithmetic with wider names.
func TestBuildSubscriptStoreMultiCharNames(t *testing.T) {
	// `arr[idx] = result`
	//  0   4   8 9 11    17
	src := []byte("arr[idx] = result\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Subscript{
				P:     ast.Pos{Line: 1, Col: 0},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "arr"},
				Slice: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "idx"},
			}},
			Value: &ast.Name{P: ast.Pos{Line: 1, Col: 11}, Id: "result"},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(subscript store multichar): %v", err)
	}
	wantLT := bytecode.SubscriptStoreLineTable(1, 11, 17, 0, 3, 4, 7, 8)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "result" || co.Names[1] != "arr" || co.Names[2] != "idx" {
		t.Fatalf("names = %v, want [result arr idx]", co.Names)
	}
}

// TestBuildSubscriptStoreWithTailFallsThrough ensures codegen rejects
// trailing no-ops.
func TestBuildSubscriptStoreWithTailFallsThrough(t *testing.T) {
	src := []byte("a[b] = x\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Subscript{
				P:     ast.Pos{Line: 1, Col: 0},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "a"},
				Slice: &ast.Name{P: ast.Pos{Line: 1, Col: 2}, Id: "b"},
			}},
			Value: &ast.Name{P: ast.Pos{Line: 1, Col: 7}, Id: "x"},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(subscript store+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildAttrStore covers `a.b = x`: STORE_ATTR oparg=2 with 4
// cache words, names = [x, a, b], stacksize = 2.
func TestBuildAttrStore(t *testing.T) {
	src := []byte("a.b = x\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Attribute{
				P:     ast.Pos{Line: 1, Col: 0},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "a"},
				Attr:  "b",
			}},
			Value: &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "x"},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(attr store): %v", err)
	}
	wantBC := bytecode.AttrStoreBytecode()
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.AttrStoreLineTable(1, 6, 7, 0, 1, 3)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 3 || co.Names[0] != "x" || co.Names[1] != "a" || co.Names[2] != "b" {
		t.Fatalf("names = %v, want [x a b]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildAttrStoreMultiCharNames exercises wider names.
func TestBuildAttrStoreMultiCharNames(t *testing.T) {
	// `obj.field = result`
	//  0   4    9 10 12     18
	src := []byte("obj.field = result\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Attribute{
				P:     ast.Pos{Line: 1, Col: 0},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "obj"},
				Attr:  "field",
			}},
			Value: &ast.Name{P: ast.Pos{Line: 1, Col: 12}, Id: "result"},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(attr store multichar): %v", err)
	}
	wantLT := bytecode.AttrStoreLineTable(1, 12, 18, 0, 3, 9)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "result" || co.Names[1] != "obj" || co.Names[2] != "field" {
		t.Fatalf("names = %v, want [result obj field]", co.Names)
	}
}

// TestBuildAttrStoreWithTailFallsThrough ensures codegen rejects
// trailing no-ops.
func TestBuildAttrStoreWithTailFallsThrough(t *testing.T) {
	src := []byte("a.b = x\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P: ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Attribute{
				P:     ast.Pos{Line: 1, Col: 0},
				Value: &ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "a"},
				Attr:  "b",
			}},
			Value: &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "x"},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(attr store+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildCallAssignNoArgs covers `x = f()`: PUSH_NULL after
// LOAD_NAME f, CALL 0 with 3 cache words, names = [f, x],
// stacksize = 2.
func TestBuildCallAssignNoArgs(t *testing.T) {
	src := []byte("x = f()\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Call{
				P:    ast.Pos{Line: 1, Col: 4},
				Func: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "f"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(call no args): %v", err)
	}
	wantBC := bytecode.CallAssignBytecode(0)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantLT := bytecode.CallAssignLineTable(1, 4, 5, nil, 7, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if len(co.Names) != 2 || co.Names[0] != "f" || co.Names[1] != "x" {
		t.Fatalf("names = %v, want [f x]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildCallAssignOneArg covers `x = f(a)`.
func TestBuildCallAssignOneArg(t *testing.T) {
	src := []byte("x = f(a)\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Call{
				P:    ast.Pos{Line: 1, Col: 4},
				Func: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "f"},
				Args: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "a"}},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(call one arg): %v", err)
	}
	wantBC := bytecode.CallAssignBytecode(1)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	wantArgs := []bytecode.CallArg{{Name: "a", Col: 6, NameLen: 1}}
	wantLT := bytecode.CallAssignLineTable(1, 4, 5, wantArgs, 8, 1)
	if !bytes.Equal(co.LineTable, wantLT) {
		t.Fatalf("linetable = %x, want %x", co.LineTable, wantLT)
	}
	if len(co.Names) != 3 || co.Names[0] != "f" || co.Names[1] != "a" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [f a x]", co.Names)
	}
	if co.StackSize < 3 {
		t.Fatalf("stacksize = %d, want >= 3", co.StackSize)
	}
}

// TestBuildCallAssignTwoArgs covers `x = f(a, b)`.
func TestBuildCallAssignTwoArgs(t *testing.T) {
	src := []byte("x = f(a, b)\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Call{
				P:    ast.Pos{Line: 1, Col: 4},
				Func: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "f"},
				Args: []ast.Expr{
					&ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "a"},
					&ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "b"},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build(call two args): %v", err)
	}
	wantBC := bytecode.CallAssignBytecode(2)
	if !bytes.Equal(co.Bytecode, wantBC) {
		t.Fatalf("bytecode = %x, want %x", co.Bytecode, wantBC)
	}
	if len(co.Names) != 4 || co.Names[0] != "f" || co.Names[1] != "a" || co.Names[2] != "b" || co.Names[3] != "x" {
		t.Fatalf("names = %v, want [f a b x]", co.Names)
	}
	if co.StackSize < 4 {
		t.Fatalf("stacksize = %d, want >= 4", co.StackSize)
	}
}

// TestBuildCallAssignWithTailFallsThrough ensures codegen rejects
// trailing no-ops.
func TestBuildCallAssignWithTailFallsThrough(t *testing.T) {
	src := []byte("x = f()\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Call{
				P:    ast.Pos{Line: 1, Col: 4},
				Func: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "f"},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(call+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildCallAssignKwargRejected ensures codegen rejects keyword
// arguments — they are not part of the modCallAssign shape.
func TestBuildCallAssignKwargRejected(t *testing.T) {
	src := []byte("x = f(k=a)\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.Call{
				P:    ast.Pos{Line: 1, Col: 4},
				Func: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "f"},
				Keywords: []*ast.Keyword{
					{Arg: "k", Value: &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "a"}},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(call kwarg) = %v, want ErrUnsupported", err)
	}
}

// TestBuildGenExprAddChain covers `x = a + b + c`: nested BinOp where
// every operand is a Name. Verifies recursive walker emits LOAD_NAME
// in left-to-right order, BINARY_OP after each pair, and StackSize=2.
func TestBuildGenExprAddChain(t *testing.T) {
	src := []byte("x = a + b + c\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P: ast.Pos{Line: 1, Col: 4},
				Left: &ast.BinOp{
					P:    ast.Pos{Line: 1, Col: 4},
					Left: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
					Op:   "Add",
					Right: &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
				},
				Op:    "Add",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 12}, Id: "c"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 4 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "c" || co.Names[3] != "x" {
		t.Fatalf("names = %v, want [a b c x]", co.Names)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildGenExprNestedPrec covers `x = a + b * c`: precedence
// produces BinOp(a, Add, BinOp(b, Mult, c)) — right-deeper than left.
func TestBuildGenExprNestedPrec(t *testing.T) {
	src := []byte("x = a + b * c\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P:    ast.Pos{Line: 1, Col: 4},
				Left: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Op:   "Add",
				Right: &ast.BinOp{
					P:     ast.Pos{Line: 1, Col: 8},
					Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
					Op:    "Mult",
					Right: &ast.Name{P: ast.Pos{Line: 1, Col: 12}, Id: "c"},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 4 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "c" || co.Names[3] != "x" {
		t.Fatalf("names = %v, want [a b c x]", co.Names)
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildGenExprIntRight covers `x = a + 1`: BinOp with a small-int
// Constant on the right. The first int constant must be appended to
// co_consts before the trailing None.
func TestBuildGenExprIntRight(t *testing.T) {
	src := []byte("x = a + 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 4},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Op:    "Add",
				Right: &ast.Constant{P: ast.Pos{Line: 1, Col: 8}, Kind: "int", Value: int64(1)},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 2 || co.Names[0] != "a" || co.Names[1] != "x" {
		t.Fatalf("names = %v, want [a x]", co.Names)
	}
	if len(co.Consts) != 2 {
		t.Fatalf("consts len = %d, want 2", len(co.Consts))
	}
	if iv, ok := co.Consts[0].(int64); !ok || iv != 1 {
		t.Fatalf("consts[0] = %v, want int64(1)", co.Consts[0])
	}
	if co.Consts[1] != nil {
		t.Fatalf("consts[1] = %v, want nil", co.Consts[1])
	}
}

// TestBuildGenExprUnaryThenAdd covers `x = -a + b`: UnaryOp(USub) over
// a Name inside a BinOp. The classifier's modAssign extractValue must
// not fold this — only UnaryOp over a numeric Constant collapses.
func TestBuildGenExprUnaryThenAdd(t *testing.T) {
	src := []byte("x = -a + b\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P: ast.Pos{Line: 1, Col: 4},
				Left: &ast.UnaryOp{
					P:       ast.Pos{Line: 1, Col: 4},
					Op:      "USub",
					Operand: &ast.Name{P: ast.Pos{Line: 1, Col: 5}, Id: "a"},
				},
				Op:    "Add",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "b"},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "b" || co.Names[2] != "x" {
		t.Fatalf("names = %v, want [a b x]", co.Names)
	}
	if len(co.Consts) != 1 || co.Consts[0] != nil {
		t.Fatalf("consts = %v, want [nil]", co.Consts)
	}
}

// TestBuildGenExprWithTailFallsThrough ensures codegen rejects
// multi-statement modGenExpr modules (single-statement parity rule).
func TestBuildGenExprWithTailFallsThrough(t *testing.T) {
	src := []byte("x = a + b\npass\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 4},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "a"},
				Op:    "Add",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 8}, Id: "b"},
			},
		},
		&ast.Pass{P: ast.Pos{Line: 2}},
	}}
	// modBinOpAssign owns Name+Name BinOps single-statement, so this
	// multi-statement module falls through both modBinOpAssign and
	// modGenExpr to ErrUnsupported.
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(genexpr+tail) = %v, want ErrUnsupported", err)
	}
}

// TestBuildAugAssignFloatFallsThrough ensures codegen rejects shapes
// the aug-assign classifier does not own (non-int init value).
func TestBuildAugAssignFloatFallsThrough(t *testing.T) {
	src := []byte("x = 0.0\nx += 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.Assign{
			P:       ast.Pos{Line: 1, Col: 0},
			Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 0}, Id: "x"}},
			Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 4}, Kind: "float", Value: float64(0.0)},
		},
		&ast.AugAssign{
			P:      ast.Pos{Line: 2, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 2, Col: 0}, Id: "x"},
			Op:     "Add",
			Value:  &ast.Constant{P: ast.Pos{Line: 2, Col: 5}, Kind: "int", Value: int64(1)},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(float init) = %v, want ErrUnsupported", err)
	}
}

// TestBuildIfElseSimpleNoElse covers `if a: x = 1` with no else
// branch. Verifies single-branch dispatch, no-else implicit return
// tail attributing the trailing 2-cu run back to the condition's
// source position, names=[a, x], consts=[int64(1), nil], StackSize=1.
func TestBuildIfElseSimpleNoElse(t *testing.T) {
	src := []byte("if a: x = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.If{
			P:    ast.Pos{Line: 1, Col: 0},
			Test: &ast.Name{P: ast.Pos{Line: 1, Col: 3}, Id: "a"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 1, Col: 6},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 10}, Kind: "int", Value: int64(1)},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 2 || co.Names[0] != "a" || co.Names[1] != "x" {
		t.Fatalf("names = %v, want [a x]", co.Names)
	}
	if len(co.Consts) != 2 {
		t.Fatalf("consts len = %d, want 2", len(co.Consts))
	}
	if iv, ok := co.Consts[0].(int64); !ok || iv != 1 {
		t.Fatalf("consts[0] = %v, want int64(1)", co.Consts[0])
	}
	if co.Consts[1] != nil {
		t.Fatalf("consts[1] = %v, want nil", co.Consts[1])
	}
	if co.StackSize < 1 {
		t.Fatalf("stacksize = %d, want >= 1", co.StackSize)
	}
}

// TestBuildIfElseWithElse covers `if a: x = 1\nelse: x = 2`. Verifies
// terminal else body emits LOAD_SMALL_INT/STORE_NAME/LOAD_CONST/
// RETURN_VALUE; consts mirrors the classifier's "first branch value
// as phantom" rule.
func TestBuildIfElseWithElse(t *testing.T) {
	src := []byte("if a: x = 1\nelse: x = 2\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.If{
			P:    ast.Pos{Line: 1, Col: 0},
			Test: &ast.Name{P: ast.Pos{Line: 1, Col: 3}, Id: "a"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 1, Col: 6},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 10}, Kind: "int", Value: int64(1)},
				},
			},
			Orelse: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 6},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 6}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 10}, Kind: "int", Value: int64(2)},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 2 || co.Names[0] != "a" || co.Names[1] != "x" {
		t.Fatalf("names = %v, want [a x]", co.Names)
	}
	if len(co.Consts) != 2 {
		t.Fatalf("consts len = %d, want 2", len(co.Consts))
	}
	if iv, ok := co.Consts[0].(int64); !ok || iv != 1 {
		t.Fatalf("consts[0] = %v, want int64(1) (first branch value)", co.Consts[0])
	}
	if co.Consts[1] != nil {
		t.Fatalf("consts[1] = %v, want nil", co.Consts[1])
	}
	if co.StackSize < 1 {
		t.Fatalf("stacksize = %d, want >= 1", co.StackSize)
	}
}

// TestBuildIfElseElifElse covers `if a: x = 1 / elif b: x = 2 / else:
// x = 3`. Verifies the recursive *ast.If chain in Orelse dedupes
// names insertion-order: condName then varName per branch, then else
// varName.
func TestBuildIfElseElifElse(t *testing.T) {
	src := []byte("if a: x = 1\nelif b: x = 2\nelse: x = 3\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.If{
			P:    ast.Pos{Line: 1, Col: 0},
			Test: &ast.Name{P: ast.Pos{Line: 1, Col: 3}, Id: "a"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 1, Col: 6},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 10}, Kind: "int", Value: int64(1)},
				},
			},
			Orelse: []ast.Stmt{
				&ast.If{
					P:    ast.Pos{Line: 2, Col: 0},
					Test: &ast.Name{P: ast.Pos{Line: 2, Col: 5}, Id: "b"},
					Body: []ast.Stmt{
						&ast.Assign{
							P:       ast.Pos{Line: 2, Col: 8},
							Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 8}, Id: "x"}},
							Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 12}, Kind: "int", Value: int64(2)},
						},
					},
					Orelse: []ast.Stmt{
						&ast.Assign{
							P:       ast.Pos{Line: 3, Col: 6},
							Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 3, Col: 6}, Id: "x"}},
							Value:   &ast.Constant{P: ast.Pos{Line: 3, Col: 10}, Kind: "int", Value: int64(3)},
						},
					},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 3 || co.Names[0] != "a" || co.Names[1] != "x" || co.Names[2] != "b" {
		t.Fatalf("names = %v, want [a x b]", co.Names)
	}
	if len(co.Consts) != 2 {
		t.Fatalf("consts len = %d, want 2", len(co.Consts))
	}
	if iv, ok := co.Consts[0].(int64); !ok || iv != 1 {
		t.Fatalf("consts[0] = %v, want int64(1)", co.Consts[0])
	}
	if co.StackSize < 1 {
		t.Fatalf("stacksize = %d, want >= 1", co.StackSize)
	}
}

// TestBuildIfElseMultiVarFallsThrough ensures codegen rejects a body
// that contains two assignments — the single-assign body rule.
func TestBuildIfElseMultiVarFallsThrough(t *testing.T) {
	src := []byte("if a:\n  x = 1\n  y = 2\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.If{
			P:    ast.Pos{Line: 1, Col: 0},
			Test: &ast.Name{P: ast.Pos{Line: 1, Col: 3}, Id: "a"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 2},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 2}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 6}, Kind: "int", Value: int64(1)},
				},
				&ast.Assign{
					P:       ast.Pos{Line: 3, Col: 2},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 3, Col: 2}, Id: "y"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 3, Col: 6}, Kind: "int", Value: int64(2)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(multi-stmt body) = %v, want ErrUnsupported", err)
	}
}

// TestBuildForSimple covers `for x in xs:\n    y = 1`. Verifies the
// FOR_ITER+JUMP_BACKWARD pair, the 4-cu setup at iter location, the
// 3-cu body-trailing run at bodyVar location, and the 4-cu loop-exit
// tail attributed back to the for line via a LONG line-table entry.
func TestBuildForSimple(t *testing.T) {
	src := []byte("for x in xs:\n    y = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.For{
			P:      ast.Pos{Line: 1, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "x"},
			Iter:   &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "xs"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "y"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 3 || co.Names[0] != "xs" || co.Names[1] != "x" || co.Names[2] != "y" {
		t.Fatalf("names = %v, want [xs x y]", co.Names)
	}
	if len(co.Consts) != 2 {
		t.Fatalf("consts len = %d, want 2", len(co.Consts))
	}
	if iv, ok := co.Consts[0].(int64); !ok || iv != 1 {
		t.Fatalf("consts[0] = %v, want int64(1)", co.Consts[0])
	}
	if co.Consts[1] != nil {
		t.Fatalf("consts[1] = %v, want nil", co.Consts[1])
	}
	if co.StackSize < 2 {
		t.Fatalf("stacksize = %d, want >= 2", co.StackSize)
	}
}

// TestBuildForSameNamesDedupe covers `for x in x: x = 1` — same
// identifier as iter, loopVar and bodyVar collapses to a single
// co_names entry via the insertion-order map.
func TestBuildForSameNamesDedupe(t *testing.T) {
	src := []byte("for x in x:\n    x = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.For{
			P:      ast.Pos{Line: 1, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "x"},
			Iter:   &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "x"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 1 || co.Names[0] != "x" {
		t.Fatalf("names = %v, want [x] (dedupe)", co.Names)
	}
}

// TestBuildForMultiVarFallsThrough rejects a body with two
// assignments — the single-assign body rule.
func TestBuildForMultiVarFallsThrough(t *testing.T) {
	src := []byte("for x in xs:\n    y = 1\n    z = 2\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.For{
			P:      ast.Pos{Line: 1, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "x"},
			Iter:   &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "xs"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "y"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
				&ast.Assign{
					P:       ast.Pos{Line: 3, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 3, Col: 4}, Id: "z"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 3, Col: 8}, Kind: "int", Value: int64(2)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(multi-stmt body) = %v, want ErrUnsupported", err)
	}
}

// TestBuildForWithElseFallsThrough rejects `for x in xs: y = 1 /
// else: y = 2` — codegen does not own loops with else clauses.
func TestBuildForWithElseFallsThrough(t *testing.T) {
	src := []byte("for x in xs:\n    y = 1\nelse:\n    y = 2\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.For{
			P:      ast.Pos{Line: 1, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "x"},
			Iter:   &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "xs"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "y"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
			},
			Orelse: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 4, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 4, Col: 4}, Id: "y"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 4, Col: 8}, Kind: "int", Value: int64(2)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(for+else) = %v, want ErrUnsupported", err)
	}
}

// TestBuildForNonNameIterFallsThrough rejects `for x in range(10):
// y = 1` — the iter-must-be-Name rule.
func TestBuildForNonNameIterFallsThrough(t *testing.T) {
	src := []byte("for x in range(10):\n    y = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.For{
			P:      ast.Pos{Line: 1, Col: 0},
			Target: &ast.Name{P: ast.Pos{Line: 1, Col: 4}, Id: "x"},
			Iter: &ast.Call{
				P:    ast.Pos{Line: 1, Col: 9},
				Func: &ast.Name{P: ast.Pos{Line: 1, Col: 9}, Id: "range"},
				Args: []ast.Expr{&ast.Constant{P: ast.Pos{Line: 1, Col: 15}, Kind: "int", Value: int64(10)}},
			},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "y"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(non-Name iter) = %v, want ErrUnsupported", err)
	}
}

// TestBuildWhileSimple covers `while a:\n    x = 1`. Verifies the
// 8-cu condition run, JUMP_BACKWARD 12 after the body, and the
// trailing implicit-return-None run attributed back to the condition
// line (LONG line-table entry produced by the encoder).
func TestBuildWhileSimple(t *testing.T) {
	src := []byte("while a:\n    x = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.While{
			P:    ast.Pos{Line: 1, Col: 0},
			Test: &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "a"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 2 || co.Names[0] != "a" || co.Names[1] != "x" {
		t.Fatalf("names = %v, want [a x]", co.Names)
	}
	if len(co.Consts) != 2 {
		t.Fatalf("consts len = %d, want 2", len(co.Consts))
	}
	if iv, ok := co.Consts[0].(int64); !ok || iv != 1 {
		t.Fatalf("consts[0] = %v, want int64(1)", co.Consts[0])
	}
	if co.Consts[1] != nil {
		t.Fatalf("consts[1] = %v, want nil", co.Consts[1])
	}
	if co.StackSize < 1 {
		t.Fatalf("stacksize = %d, want >= 1", co.StackSize)
	}
}

// TestBuildWhileSameNameDedupes covers `while x: x = 0`. The
// classifier's compileWhile collapses condName == varName to a
// single co_names entry; the codegen path must do the same via the
// insertion-order addName helper.
func TestBuildWhileSameNameDedupes(t *testing.T) {
	src := []byte("while x:\n    x = 0\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.While{
			P:    ast.Pos{Line: 1, Col: 0},
			Test: &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "x"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(0)},
				},
			},
		},
	}}
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 1 || co.Names[0] != "x" {
		t.Fatalf("names = %v, want [x] (dedupe)", co.Names)
	}
	if len(co.Consts) != 2 {
		t.Fatalf("consts len = %d, want 2", len(co.Consts))
	}
	if iv, ok := co.Consts[0].(int64); !ok || iv != 0 {
		t.Fatalf("consts[0] = %v, want int64(0)", co.Consts[0])
	}
}

// TestBuildWhileMultiVarFallsThrough rejects a body with two
// assignments — the single-assign body rule.
func TestBuildWhileMultiVarFallsThrough(t *testing.T) {
	src := []byte("while a:\n    x = 1\n    y = 2\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.While{
			P:    ast.Pos{Line: 1, Col: 0},
			Test: &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "a"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
				&ast.Assign{
					P:       ast.Pos{Line: 3, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 3, Col: 4}, Id: "y"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 3, Col: 8}, Kind: "int", Value: int64(2)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(multi-stmt body) = %v, want ErrUnsupported", err)
	}
}

// TestBuildWhileWithElseFallsThrough rejects `while a: x = 1\nelse:
// x = 2` — codegen does not own loops with else clauses.
func TestBuildWhileWithElseFallsThrough(t *testing.T) {
	src := []byte("while a:\n    x = 1\nelse:\n    x = 2\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.While{
			P:    ast.Pos{Line: 1, Col: 0},
			Test: &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "a"},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
			},
			Orelse: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 4, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 4, Col: 4}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 4, Col: 8}, Kind: "int", Value: int64(2)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(while+else) = %v, want ErrUnsupported", err)
	}
}

// TestBuildWhileNonNameCondFallsThrough rejects `while a + b: x = 1`
// — the cond-must-be-Name rule.
func TestBuildWhileNonNameCondFallsThrough(t *testing.T) {
	src := []byte("while a + b:\n    x = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.While{
			P: ast.Pos{Line: 1, Col: 0},
			Test: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 6},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 6}, Id: "a"},
				Op:    "Add",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 10}, Id: "b"},
			},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 2, Col: 4},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 2, Col: 4}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 2, Col: 8}, Kind: "int", Value: int64(1)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(non-Name cond) = %v, want ErrUnsupported", err)
	}
}

// TestBuildIfElseNonNameCondFallsThrough ensures codegen rejects an
// `if` whose condition is not a bare Name (e.g. `if a + b: x = 1`).
func TestBuildIfElseNonNameCondFallsThrough(t *testing.T) {
	src := []byte("if a + b: x = 1\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.If{
			P: ast.Pos{Line: 1, Col: 0},
			Test: &ast.BinOp{
				P:     ast.Pos{Line: 1, Col: 3},
				Left:  &ast.Name{P: ast.Pos{Line: 1, Col: 3}, Id: "a"},
				Op:    "Add",
				Right: &ast.Name{P: ast.Pos{Line: 1, Col: 7}, Id: "b"},
			},
			Body: []ast.Stmt{
				&ast.Assign{
					P:       ast.Pos{Line: 1, Col: 10},
					Targets: []ast.Expr{&ast.Name{P: ast.Pos{Line: 1, Col: 10}, Id: "x"}},
					Value:   &ast.Constant{P: ast.Pos{Line: 1, Col: 14}, Kind: "int", Value: int64(1)},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(non-Name cond) = %v, want ErrUnsupported", err)
	}
}
