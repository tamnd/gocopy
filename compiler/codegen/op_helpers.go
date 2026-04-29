package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
)

// constantToValue maps an *ast.Constant to its Go-typed value with
// kind-vs-value validation (mismatched payloads return false). Used
// by the visitor pipeline for Assign/AugAssign RHS lowering and by
// the gen-expr classifier path.
//
// Distinct from constantValue in visit_const.go: that helper trusts
// the parser to set c.Value to the right Go type for the kind, while
// this one type-asserts and bails on mismatch.
func constantToValue(c *ast.Constant) (any, bool) {
	switch c.Kind {
	case "None":
		return nil, true
	case "True":
		return true, true
	case "False":
		return false, true
	case "int":
		v, ok := c.Value.(int64)
		return v, ok
	case "float":
		v, ok := c.Value.(float64)
		return v, ok
	case "complex":
		v, ok := c.Value.(complex128)
		return v, ok
	case "str":
		v, ok := c.Value.(string)
		return v, ok
	case "bytes":
		v, ok := c.Value.([]byte)
		return v, ok
	case "Ellipsis":
		return bytecode.Ellipsis, true
	}
	return nil, false
}

// binOpargFromOp maps an *ast.BinOp Op string to its CPython NB_*
// enum value for BINARY_OP. Mirrors compiler/classify_ast.go's
// helper of the same name.
func binOpargFromOp(op string) (byte, bool) {
	switch op {
	case "Add":
		return bytecode.NbAdd, true
	case "Sub":
		return bytecode.NbSubtract, true
	case "Mult":
		return bytecode.NbMultiply, true
	case "Div":
		return bytecode.NbTrueDivide, true
	case "FloorDiv":
		return bytecode.NbFloorDivide, true
	case "Mod":
		return bytecode.NbRemainder, true
	case "Pow":
		return bytecode.NbPower, true
	case "BitAnd":
		return bytecode.NbAnd, true
	case "BitOr":
		return bytecode.NbOr, true
	case "BitXor":
		return bytecode.NbXor, true
	case "LShift":
		return bytecode.NbLshift, true
	case "RShift":
		return bytecode.NbRshift, true
	case "MatMult":
		return bytecode.NbMatrixMultiply, true
	}
	return 0, false
}

// augOpargFromOp maps an *ast.AugAssign Op to its CPython
// NB_INPLACE_* enum value. Mirrors compiler/classify_ast.go's
// helper of the same name.
func augOpargFromOp(op string) (byte, bool) {
	switch op {
	case "Add":
		return bytecode.NbInplaceAdd, true
	case "Sub":
		return bytecode.NbInplaceSubtract, true
	case "Mult":
		return bytecode.NbInplaceMultiply, true
	case "Div":
		return bytecode.NbInplaceTrueDivide, true
	case "FloorDiv":
		return bytecode.NbInplaceFloorDivide, true
	case "Mod":
		return bytecode.NbInplaceRemainder, true
	case "Pow":
		return bytecode.NbInplacePower, true
	case "BitAnd":
		return bytecode.NbInplaceAnd, true
	case "BitOr":
		return bytecode.NbInplaceOr, true
	case "BitXor":
		return bytecode.NbInplaceXor, true
	case "LShift":
		return bytecode.NbInplaceLshift, true
	case "RShift":
		return bytecode.NbInplaceRshift, true
	}
	return 0, false
}
