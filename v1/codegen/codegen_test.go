package codegen

import (
	"errors"
	"testing"

	"github.com/tamnd/gocopy/v1/ast"
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

