package codegen

import (
	"bytes"
	"errors"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
)

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

// TestBuildFuncDefSimple covers `def f(x):\n    return x`. Verifies
// the dual-CodeObject construction: outer module with consts=
// [funcCode, nil], names=[f], StackSize=1; inner function with
// ArgCount=1, Flags=0x3, LocalsPlusNames=[x], LocalsPlusKinds=
// [LocalsKindArg], StackSize=1.
func TestBuildFuncDefSimple(t *testing.T) {
	src := []byte("def f(x):\n    return x\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.FunctionDef{
			P:    ast.Pos{Line: 1, Col: 0},
			Name: "f",
			Args: &ast.Arguments{
				Args: []*ast.Arg{
					{P: ast.Pos{Line: 1, Col: 6}, Name: "x"},
				},
			},
			Body: []ast.Stmt{
				&ast.Return{
					P:     ast.Pos{Line: 2, Col: 4},
					Value: &ast.Name{P: ast.Pos{Line: 2, Col: 11}, Id: "x"},
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
	if len(co.Names) != 1 || co.Names[0] != "f" {
		t.Fatalf("outer names = %v, want [f]", co.Names)
	}
	if len(co.Consts) != 2 {
		t.Fatalf("outer consts len = %d, want 2", len(co.Consts))
	}
	if co.Consts[1] != nil {
		t.Fatalf("outer consts[1] = %v, want nil", co.Consts[1])
	}
	inner, ok := co.Consts[0].(*bytecode.CodeObject)
	if !ok || inner == nil {
		t.Fatalf("outer consts[0] = %T, want *bytecode.CodeObject", co.Consts[0])
	}
	if inner.ArgCount != 1 {
		t.Fatalf("inner ArgCount = %d, want 1", inner.ArgCount)
	}
	if inner.Flags != 0x3 {
		t.Fatalf("inner Flags = %#x, want 0x3", inner.Flags)
	}
	if inner.StackSize != 1 {
		t.Fatalf("inner StackSize = %d, want 1", inner.StackSize)
	}
	if len(inner.LocalsPlusNames) != 1 || inner.LocalsPlusNames[0] != "x" {
		t.Fatalf("inner LocalsPlusNames = %v, want [x]", inner.LocalsPlusNames)
	}
	if len(inner.LocalsPlusKinds) != 1 || inner.LocalsPlusKinds[0] != bytecode.LocalsKindArg {
		t.Fatalf("inner LocalsPlusKinds = %v, want [LocalsKindArg]", inner.LocalsPlusKinds)
	}
	if inner.Name != "f" || inner.QualName != "f" {
		t.Fatalf("inner Name/QualName = %q/%q, want f/f", inner.Name, inner.QualName)
	}
	if co.StackSize != 1 {
		t.Fatalf("outer StackSize = %d, want 1", co.StackSize)
	}
}

// TestBuildFuncDefDifferentNames asserts the inner code object's
// Name and QualName both equal the function name (no `<module>.`
// prefix at module scope).
func TestBuildFuncDefDifferentNames(t *testing.T) {
	src := []byte("def myFunc(arg):\n    return arg\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.FunctionDef{
			P:    ast.Pos{Line: 1, Col: 0},
			Name: "myFunc",
			Args: &ast.Arguments{
				Args: []*ast.Arg{
					{P: ast.Pos{Line: 1, Col: 11}, Name: "arg"},
				},
			},
			Body: []ast.Stmt{
				&ast.Return{
					P:     ast.Pos{Line: 2, Col: 4},
					Value: &ast.Name{P: ast.Pos{Line: 2, Col: 11}, Id: "arg"},
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
	inner := co.Consts[0].(*bytecode.CodeObject)
	if inner.Name != "myFunc" || inner.QualName != "myFunc" {
		t.Fatalf("inner Name/QualName = %q/%q, want myFunc/myFunc",
			inner.Name, inner.QualName)
	}
	if len(inner.LocalsPlusNames) != 1 || inner.LocalsPlusNames[0] != "arg" {
		t.Fatalf("inner LocalsPlusNames = %v, want [arg]", inner.LocalsPlusNames)
	}
}

// TestBuildFuncDefMismatchedReturnFallsThrough rejects
// `def f(x): return y` where the return target name differs from
// the arg name.
func TestBuildFuncDefMismatchedReturnFallsThrough(t *testing.T) {
	src := []byte("def f(x):\n    return y\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.FunctionDef{
			P:    ast.Pos{Line: 1, Col: 0},
			Name: "f",
			Args: &ast.Arguments{
				Args: []*ast.Arg{
					{P: ast.Pos{Line: 1, Col: 6}, Name: "x"},
				},
			},
			Body: []ast.Stmt{
				&ast.Return{
					P:     ast.Pos{Line: 2, Col: 4},
					Value: &ast.Name{P: ast.Pos{Line: 2, Col: 11}, Id: "y"},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(mismatched return) = %v, want ErrUnsupported", err)
	}
}

// TestBuildFuncDefMultipleArgsFallsThrough rejects
// `def f(x, y): return x` — modFuncDef requires exactly one arg.
func TestBuildFuncDefMultipleArgsFallsThrough(t *testing.T) {
	src := []byte("def f(x, y):\n    return x\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.FunctionDef{
			P:    ast.Pos{Line: 1, Col: 0},
			Name: "f",
			Args: &ast.Arguments{
				Args: []*ast.Arg{
					{P: ast.Pos{Line: 1, Col: 6}, Name: "x"},
					{P: ast.Pos{Line: 1, Col: 9}, Name: "y"},
				},
			},
			Body: []ast.Stmt{
				&ast.Return{
					P:     ast.Pos{Line: 2, Col: 4},
					Value: &ast.Name{P: ast.Pos{Line: 2, Col: 11}, Id: "x"},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(2-arg func) = %v, want ErrUnsupported", err)
	}
}

// TestBuildFuncDefDecoratorFallsThrough rejects `@d\ndef f(x):
// return x` — modFuncDef requires no decorators.
func TestBuildFuncDefDecoratorFallsThrough(t *testing.T) {
	src := []byte("@d\ndef f(x):\n    return x\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.FunctionDef{
			P:    ast.Pos{Line: 2, Col: 0},
			Name: "f",
			Args: &ast.Arguments{
				Args: []*ast.Arg{
					{P: ast.Pos{Line: 2, Col: 6}, Name: "x"},
				},
			},
			Body: []ast.Stmt{
				&ast.Return{
					P:     ast.Pos{Line: 3, Col: 4},
					Value: &ast.Name{P: ast.Pos{Line: 3, Col: 11}, Id: "x"},
				},
			},
			DecoratorList: []ast.Expr{
				&ast.Name{P: ast.Pos{Line: 1, Col: 1}, Id: "d"},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(decorated func) = %v, want ErrUnsupported", err)
	}
}

// closureDefAST builds the canonical fixture-137 AST for `def f(x):
// def g(): return x; return g` with parametric outer/inner/arg names
// for use across the closure-def tests.
func closureDefAST(outerName, argName, innerName, innerRetName, outerRetName string) *ast.Module {
	innerArgCol := 6 + len(outerName) // after `def <outer>(`
	return &ast.Module{Body: []ast.Stmt{
		&ast.FunctionDef{
			P:    ast.Pos{Line: 1, Col: 0},
			Name: outerName,
			Args: &ast.Arguments{
				Args: []*ast.Arg{
					{P: ast.Pos{Line: 1, Col: innerArgCol}, Name: argName},
				},
			},
			Body: []ast.Stmt{
				&ast.FunctionDef{
					P:    ast.Pos{Line: 2, Col: 4},
					Name: innerName,
					Args: &ast.Arguments{},
					Body: []ast.Stmt{
						&ast.Return{
							P:     ast.Pos{Line: 3, Col: 8},
							Value: &ast.Name{P: ast.Pos{Line: 3, Col: 15}, Id: innerRetName},
						},
					},
				},
				&ast.Return{
					P:     ast.Pos{Line: 4, Col: 4},
					Value: &ast.Name{P: ast.Pos{Line: 4, Col: 11}, Id: outerRetName},
				},
			},
		},
	}}
}

// TestBuildClosureDefSimple covers `def f(x): def g(): return x;
// return g`. Verifies the triple-CodeObject construction and the
// cell+free closure-variable machinery.
func TestBuildClosureDefSimple(t *testing.T) {
	src := []byte("def f(x):\n    def g():\n        return x\n    return g\n")
	mod := closureDefAST("f", "x", "g", "x", "g")
	co, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(co.Names) != 1 || co.Names[0] != "f" {
		t.Fatalf("module names = %v, want [f]", co.Names)
	}
	if len(co.Consts) != 2 || co.Consts[1] != nil {
		t.Fatalf("module consts = %v, want [outerCode, nil]", co.Consts)
	}
	outer, ok := co.Consts[0].(*bytecode.CodeObject)
	if !ok || outer == nil {
		t.Fatalf("module consts[0] = %T, want *bytecode.CodeObject", co.Consts[0])
	}
	if outer.ArgCount != 1 || outer.StackSize != 2 || outer.Flags != 0x03 {
		t.Fatalf("outer ArgCount/StackSize/Flags = %d/%d/%#x, want 1/2/0x3",
			outer.ArgCount, outer.StackSize, outer.Flags)
	}
	if outer.Name != "f" || outer.QualName != "f" {
		t.Fatalf("outer Name/QualName = %q/%q, want f/f", outer.Name, outer.QualName)
	}
	if len(outer.LocalsPlusNames) != 2 ||
		outer.LocalsPlusNames[0] != "x" || outer.LocalsPlusNames[1] != "g" {
		t.Fatalf("outer LocalsPlusNames = %v, want [x g]", outer.LocalsPlusNames)
	}
	if len(outer.LocalsPlusKinds) != 2 ||
		outer.LocalsPlusKinds[0] != bytecode.LocalsKindArgCell ||
		outer.LocalsPlusKinds[1] != bytecode.LocalsKindLocal {
		t.Fatalf("outer LocalsPlusKinds = %v, want [ArgCell Local]", outer.LocalsPlusKinds)
	}
	if len(outer.Consts) != 1 {
		t.Fatalf("outer consts len = %d, want 1 (no None)", len(outer.Consts))
	}
	inner, ok := outer.Consts[0].(*bytecode.CodeObject)
	if !ok || inner == nil {
		t.Fatalf("outer consts[0] = %T, want *bytecode.CodeObject", outer.Consts[0])
	}
	if inner.Name != "g" || inner.QualName != "f.<locals>.g" {
		t.Fatalf("inner Name/QualName = %q/%q, want g/f.<locals>.g",
			inner.Name, inner.QualName)
	}
	if inner.Flags != 0x13 {
		t.Fatalf("inner Flags = %#x, want 0x13", inner.Flags)
	}
	if len(inner.LocalsPlusNames) != 1 || inner.LocalsPlusNames[0] != "x" {
		t.Fatalf("inner LocalsPlusNames = %v, want [x]", inner.LocalsPlusNames)
	}
	if len(inner.LocalsPlusKinds) != 1 || inner.LocalsPlusKinds[0] != bytecode.LocalsKindFree {
		t.Fatalf("inner LocalsPlusKinds = %v, want [Free]", inner.LocalsPlusKinds)
	}
}

// TestBuildClosureDefMismatchedFreeFallsThrough rejects
// `def f(x): def g(): return y; return g` — the inner return name
// must equal the outer arg name.
func TestBuildClosureDefMismatchedFreeFallsThrough(t *testing.T) {
	src := []byte("def f(x):\n    def g():\n        return y\n    return g\n")
	mod := closureDefAST("f", "x", "g", "y", "g")
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(mismatched free) = %v, want ErrUnsupported", err)
	}
}

// TestBuildClosureDefInnerWithArgsFallsThrough rejects
// `def f(x): def g(z): return x; return g` — inner must be zero-arg.
func TestBuildClosureDefInnerWithArgsFallsThrough(t *testing.T) {
	src := []byte("def f(x):\n    def g(z):\n        return x\n    return g\n")
	mod := &ast.Module{Body: []ast.Stmt{
		&ast.FunctionDef{
			P:    ast.Pos{Line: 1, Col: 0},
			Name: "f",
			Args: &ast.Arguments{
				Args: []*ast.Arg{{P: ast.Pos{Line: 1, Col: 6}, Name: "x"}},
			},
			Body: []ast.Stmt{
				&ast.FunctionDef{
					P:    ast.Pos{Line: 2, Col: 4},
					Name: "g",
					Args: &ast.Arguments{
						Args: []*ast.Arg{{P: ast.Pos{Line: 2, Col: 10}, Name: "z"}},
					},
					Body: []ast.Stmt{
						&ast.Return{
							P:     ast.Pos{Line: 3, Col: 8},
							Value: &ast.Name{P: ast.Pos{Line: 3, Col: 15}, Id: "x"},
						},
					},
				},
				&ast.Return{
					P:     ast.Pos{Line: 4, Col: 4},
					Value: &ast.Name{P: ast.Pos{Line: 4, Col: 11}, Id: "g"},
				},
			},
		},
	}}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(inner-with-args) = %v, want ErrUnsupported", err)
	}
}

// TestBuildClosureDefMismatchedOuterReturnFallsThrough rejects
// `def f(x): def g(): return x; return h` — the outer return name
// must equal the inner func name.
func TestBuildClosureDefMismatchedOuterReturnFallsThrough(t *testing.T) {
	src := []byte("def f(x):\n    def g():\n        return x\n    return h\n")
	mod := closureDefAST("f", "x", "g", "x", "h")
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(mismatched outer return) = %v, want ErrUnsupported", err)
	}
}

// TestBuildClosureDefDecoratorFallsThrough rejects a decorated outer
// def — modClosureDef requires no decorators.
func TestBuildClosureDefDecoratorFallsThrough(t *testing.T) {
	src := []byte("@d\ndef f(x):\n    def g():\n        return x\n    return g\n")
	mod := closureDefAST("f", "x", "g", "x", "g")
	mod.Body[0].(*ast.FunctionDef).DecoratorList = []ast.Expr{
		&ast.Name{P: ast.Pos{Line: 1, Col: 1}, Id: "d"},
	}
	_, err := Build(mod, nil, Options{
		Source: src, Filename: "x.py", Name: "<module>",
		QualName: "<module>", FirstLineNo: 1,
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Build(decorated closure) = %v, want ErrUnsupported", err)
	}
}
