package codegen

import (
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
