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
