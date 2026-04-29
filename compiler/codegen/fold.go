package codegen

import (
	"math"

	"github.com/tamnd/gocopy/compiler/ast"
)

// numConstVal extracts the numeric value (int64 or float64) from a
// constant node. Mirrors compiler/classify_ast.go::numConstVal.
//
// At v0.7.2 the codegen package owns the parse-time fold predicate so
// the visitor can short-circuit BinOp(Const,op,Const) and
// UnaryOp(USub, numeric) into a single LOAD opcode. v0.7.16 lifts the
// fold into a real optimizer pass and this helper retires.
func numConstVal(c *ast.Constant) (any, bool) {
	switch c.Kind {
	case "int":
		v, ok := c.Value.(int64)
		return v, ok
	case "float":
		v, ok := c.Value.(float64)
		return v, ok
	}
	return nil, false
}

// foldBinOp computes the CPython compile-time folded result of
// `left op right`, where left and right are int64 or float64. Returns
// (nil, false) when the op or operand types are not foldable. Mirrors
// compiler/classify_ast.go::foldBinOp byte-for-byte.
func foldBinOp(left any, op string, right any) (any, bool) {
	li, leftIsInt := left.(int64)
	ri, rightIsInt := right.(int64)
	lf, leftIsFloat := left.(float64)
	rf, rightIsFloat := right.(float64)

	if leftIsFloat || rightIsFloat {
		if leftIsInt {
			lf = float64(li)
		}
		if rightIsInt {
			rf = float64(ri)
		}
		switch op {
		case "Add":
			return lf + rf, true
		case "Sub":
			return lf - rf, true
		case "Mult":
			return lf * rf, true
		case "Div":
			return lf / rf, true
		case "Pow":
			return math.Pow(lf, rf), true
		case "Mod":
			result := math.Mod(lf, rf)
			if result != 0 && (math.Signbit(result) != math.Signbit(rf)) {
				result += rf
			}
			return result, true
		case "FloorDiv":
			return math.Floor(lf / rf), true
		}
		return nil, false
	}

	if !leftIsInt || !rightIsInt {
		return nil, false
	}

	switch op {
	case "Add":
		return li + ri, true
	case "Sub":
		return li - ri, true
	case "Mult":
		return li * ri, true
	case "Div":
		if ri == 0 {
			return nil, false
		}
		return float64(li) / float64(ri), true
	case "FloorDiv":
		if ri == 0 {
			return nil, false
		}
		q := li / ri
		if (li^ri) < 0 && q*ri != li {
			q--
		}
		return q, true
	case "Mod":
		if ri == 0 {
			return nil, false
		}
		result := li % ri
		if result != 0 && (result < 0) != (ri < 0) {
			result += ri
		}
		return result, true
	case "Pow":
		if ri < 0 {
			return math.Pow(float64(li), float64(ri)), true
		}
		if ri > 62 {
			return nil, false
		}
		result := int64(1)
		base := li
		for j := int64(0); j < ri; j++ {
			result *= base
		}
		return result, true
	}
	return nil, false
}
