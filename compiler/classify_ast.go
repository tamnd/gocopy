package compiler

import (
	"strconv"
	"strings"

	parser2 "github.com/tamnd/gopapy/v2/parser2"

	"github.com/tamnd/gocopy/v1/bytecode"
)

// classifyAST walks a gopapy v2 Module and returns the classification
// of the module body if it falls within the supported subset.
func classifyAST(src []byte, mod *parser2.Module) (classification, bool) {
	lines := splitLines(src)

	lineEndCol := func(line int) (byte, bool) {
		if line < 1 || line > len(lines) {
			return 0, false
		}
		ln := trimRight(stripLineComment(lines[line-1]))
		if len(ln) > 255 {
			return 0, false
		}
		return byte(len(ln)), true
	}

	stmts := make([]rawStmt, 0, len(mod.Body))

	for _, stmt := range mod.Body {
		switch s := stmt.(type) {
		case *parser2.Pass:
			line := s.P.Line
			ec, ok := lineEndCol(line)
			if !ok {
				return classification{}, false
			}
			stmts = append(stmts, rawStmt{line: line, endLine: line, endCol: ec, kind: stmtNoOp})

		case *parser2.ExprStmt:
			line := s.P.Line
			ec, ok := lineEndCol(line)
			if !ok {
				return classification{}, false
			}
			c, isConst := s.Value.(*parser2.Constant)
			if !isConst {
				return classification{}, false
			}
			switch c.Kind {
			case "str":
				v := c.Value.(string)
				N := strings.Count(v, "\n")
				endLine := line + N
				if endLine > len(lines) {
					return classification{}, false
				}
				ec2, ok2 := lineEndCol(endLine)
				if !ok2 {
					return classification{}, false
				}
				for _, seg := range strings.Split(v, "\n") {
					if !isPlainAscii([]byte(seg), 0) {
						return classification{}, false
					}
				}
				stmts = append(stmts, rawStmt{
					line:    line,
					endLine: endLine,
					endCol:  ec2,
					kind:    stmtString,
					text:    v,
				})
			case "None", "True", "False", "Ellipsis", "int", "float", "complex", "bytes":
				stmts = append(stmts, rawStmt{line: line, endLine: line, endCol: ec, kind: stmtNoOp})
			default:
				return classification{}, false
			}

		case *parser2.Assign:
			line := s.P.Line
			ec, ok := lineEndCol(line)
			if !ok {
				return classification{}, false
			}
			// Try literal-value assignment first.
			val, vStart, ok2 := extractValue(s.Value)
			if !ok2 {
				// Try arithmetic expression forms (name binop name, unary name).
				if len(s.Targets) != 1 {
					return classification{}, false
				}
				target, isName := s.Targets[0].(*parser2.Name)
				if !isName || len(target.Id) > 15 {
					return classification{}, false
				}
				rs, ok3 := extractExprAssign(line, target, s.Value)
				if !ok3 {
					return classification{}, false
				}
				stmts = append(stmts, rs)
				continue
			}
			if sv, isStr := val.(string); isStr && strings.Contains(sv, "\n") {
				return classification{}, false
			}

			if len(s.Targets) == 1 {
				name, isName := s.Targets[0].(*parser2.Name)
				if !isName || len(name.Id) > 15 {
					return classification{}, false
				}
				stmts = append(stmts, rawStmt{
					line:         line,
					endLine:      line,
					endCol:       ec,
					kind:         stmtAssign,
					text:         name.Id,
					asgnNameLen:  byte(len(name.Id)),
					asgnValStart: vStart,
					asgnValEnd:   ec,
					asgnValue:    val,
				})
			} else {
				targets := make([]chainedTarget, len(s.Targets))
				for i, t := range s.Targets {
					name, isName := t.(*parser2.Name)
					if !isName || name.P.Col > 255 || len(name.Id) > 15 {
						return classification{}, false
					}
					targets[i] = chainedTarget{
						name:      name.Id,
						nameStart: byte(name.P.Col),
						nameLen:   byte(len(name.Id)),
					}
				}
				stmts = append(stmts, rawStmt{
					line:         line,
					endLine:      line,
					endCol:       ec,
					kind:         stmtChainedAssign,
					asgnValStart: vStart,
					asgnValEnd:   ec,
					asgnValue:    val,
					chainTargets: targets,
				})
			}

		case *parser2.AugAssign:
			line := s.P.Line
			ec, ok := lineEndCol(line)
			if !ok {
				return classification{}, false
			}
			target, isName := s.Target.(*parser2.Name)
			if !isName || len(target.Id) > 15 {
				return classification{}, false
			}
			oparg, ok2 := augOpargFromOp(s.Op)
			if !ok2 {
				return classification{}, false
			}
			c, isConst := s.Value.(*parser2.Constant)
			if !isConst || c.Kind != "int" {
				return classification{}, false
			}
			iv := c.Value.(int64)
			if iv < 0 || c.P.Col > 255 {
				return classification{}, false
			}
			stmts = append(stmts, rawStmt{
				line:         line,
				endLine:      line,
				endCol:       ec,
				kind:         stmtAugAssign,
				text:         target.Id,
				asgnNameLen:  byte(len(target.Id)),
				asgnValStart: byte(c.P.Col),
				asgnValEnd:   ec,
				asgnValue:    iv,
				augOparg:     oparg,
			})

		default:
			return classification{}, false
		}
	}

	return stmtsToClassification(stmts)
}

func extractValue(e parser2.Expr) (val any, valStart byte, ok bool) {
	switch v := e.(type) {
	case *parser2.Constant:
		cval, cok := constantToValue(v)
		if !cok || v.P.Col > 255 {
			return nil, 0, false
		}
		return cval, byte(v.P.Col), true
	case *parser2.UnaryOp:
		if v.Op != "USub" || v.P.Col > 255 {
			return nil, 0, false
		}
		c, isConst := v.Operand.(*parser2.Constant)
		if !isConst {
			return nil, 0, false
		}
		switch c.Kind {
		case "int":
			iv := c.Value.(int64)
			if iv == 0 {
				return nil, 0, false
			}
			return negLiteral{pos: iv, neg: -iv}, byte(v.P.Col), true
		case "float":
			fv := c.Value.(float64)
			return negLiteral{pos: fv, neg: -fv}, byte(v.P.Col), true
		}
	}
	return nil, 0, false
}

func constantToValue(c *parser2.Constant) (any, bool) {
	switch c.Kind {
	case "None":
		return nil, true
	case "True":
		return true, true
	case "False":
		return false, true
	case "Ellipsis":
		return bytecode.Ellipsis, true
	case "int":
		return c.Value.(int64), true
	case "float":
		return c.Value.(float64), true
	case "complex":
		s := c.Value.(string)
		if len(s) < 2 {
			return nil, false
		}
		f, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil {
			return nil, false
		}
		return complex(0, f), true
	case "str":
		return c.Value.(string), true
	case "bytes":
		return []byte(c.Value.(string)), true
	}
	return nil, false
}

// extractExprAssign tries to parse s.Value as a BinOp(Name, Name) or
// UnaryOp(Name) assignment. target is the already-validated single LHS name.
func extractExprAssign(line int, target *parser2.Name, value parser2.Expr) (rawStmt, bool) {
	targetLen := byte(len(target.Id))

	switch e := value.(type) {
	case *parser2.Compare:
		if len(e.Ops) != 1 {
			return rawStmt{}, false // chained comparisons deferred
		}
		leftName, leftOK := e.Left.(*parser2.Name)
		rightName, rightOK := e.Comparators[0].(*parser2.Name)
		if !leftOK || !rightOK {
			return rawStmt{}, false
		}
		if len(leftName.Id) > 15 || len(rightName.Id) > 15 {
			return rawStmt{}, false
		}
		if leftName.P.Col > 255 || rightName.P.Col > 255 {
			return rawStmt{}, false
		}
		op, oparg, cmpOK := cmpOpFromOp(e.Ops[0])
		if !cmpOK {
			return rawStmt{}, false
		}
		return rawStmt{
			line:    line,
			endLine: line,
			kind:    stmtCmpAssign,
			cmpAsgn: cmpAssign{
				line:      line,
				target:    target.Id,
				targetLen: targetLen,
				leftName:  leftName.Id,
				leftCol:   byte(leftName.P.Col),
				leftLen:   byte(len(leftName.Id)),
				rightName: rightName.Id,
				rightCol:  byte(rightName.P.Col),
				rightLen:  byte(len(rightName.Id)),
				op:        op,
				oparg:     oparg,
			},
		}, true

	case *parser2.BinOp:
		leftName, leftOK := e.Left.(*parser2.Name)
		rightName, rightOK := e.Right.(*parser2.Name)
		if !leftOK || !rightOK {
			return rawStmt{}, false
		}
		if len(leftName.Id) > 15 || len(rightName.Id) > 15 {
			return rawStmt{}, false
		}
		if leftName.P.Col > 255 || rightName.P.Col > 255 {
			return rawStmt{}, false
		}
		oparg, opOK := binOpargFromOp(e.Op)
		if !opOK {
			return rawStmt{}, false
		}
		leftLen := byte(len(leftName.Id))
		rightLen := byte(len(rightName.Id))
		return rawStmt{
			line:    line,
			endLine: line,
			kind:    stmtBinOpAssign,
			binAsgn: binOpAssign{
				line:      line,
				target:    target.Id,
				targetLen: targetLen,
				leftName:  leftName.Id,
				leftCol:   byte(leftName.P.Col),
				leftLen:   leftLen,
				rightName: rightName.Id,
				rightCol:  byte(rightName.P.Col),
				rightLen:  rightLen,
				oparg:     oparg,
			},
		}, true

	case *parser2.UnaryOp:
		operandName, operandOK := e.Operand.(*parser2.Name)
		if !operandOK {
			return rawStmt{}, false
		}
		if len(operandName.Id) > 15 || operandName.P.Col > 255 || e.P.Col > 255 {
			return rawStmt{}, false
		}
		var kind unaryKind
		switch e.Op {
		case "USub":
			kind = unaryNeg
		case "Invert":
			kind = unaryInvert
		case "Not":
			kind = unaryNot
		default:
			return rawStmt{}, false
		}
		return rawStmt{
			line:    line,
			endLine: line,
			kind:    stmtUnaryAssign,
			unaryAsgn: unaryAssign{
				line:       line,
				target:     target.Id,
				targetLen:  targetLen,
				operand:    operandName.Id,
				operandCol: byte(operandName.P.Col),
				operandLen: byte(len(operandName.Id)),
				opCol:      byte(e.P.Col),
				kind:       kind,
			},
		}, true
	}
	return rawStmt{}, false
}

// cmpOpFromOp maps a gopapy Compare.Ops string to (opcode, oparg, ok).
// Returns COMPARE_OP/IS_OP/CONTAINS_OP and the appropriate oparg.
func cmpOpFromOp(op string) (bytecode.Opcode, byte, bool) {
	switch op {
	case "Lt":
		return bytecode.COMPARE_OP, bytecode.CmpLt, true
	case "LtE":
		return bytecode.COMPARE_OP, bytecode.CmpLtE, true
	case "Eq":
		return bytecode.COMPARE_OP, bytecode.CmpEq, true
	case "NotEq":
		return bytecode.COMPARE_OP, bytecode.CmpNotEq, true
	case "Gt":
		return bytecode.COMPARE_OP, bytecode.CmpGt, true
	case "GtE":
		return bytecode.COMPARE_OP, bytecode.CmpGtE, true
	case "Is":
		return bytecode.IS_OP, 0, true
	case "IsNot":
		return bytecode.IS_OP, 1, true
	case "In":
		return bytecode.CONTAINS_OP, 0, true
	case "NotIn":
		return bytecode.CONTAINS_OP, 1, true
	}
	return 0, 0, false
}

// binOpargFromOp maps a gopapy BinOp.Op string to the NB_* oparg for BINARY_OP.
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
