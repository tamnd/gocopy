package compiler

import (
	"strconv"
	"strings"

	parser2 "github.com/tamnd/gopapy/parser"

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
			// Check for subscript/attr store targets: a[b] = x, a.b = x.
			if len(s.Targets) == 1 {
				switch tgt := s.Targets[0].(type) {
				case *parser2.Subscript:
					rs, ok2 := extractSubscriptStore(line, tgt, s.Value)
					if !ok2 {
						return classification{}, false
					}
					stmts = append(stmts, rs)
					continue
				case *parser2.Attribute:
					rs, ok2 := extractAttrStore(line, tgt, s.Value)
					if !ok2 {
						return classification{}, false
					}
					stmts = append(stmts, rs)
					continue
				}
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
				rs, ok3 := extractExprAssign(line, target, s.Value, lines)
				if !ok3 {
					rs2, ok4 := extractGenExprAssign(line, ec, target, s.Value, lines)
					if !ok4 {
						return classification{}, false
					}
					stmts = append(stmts, rs2)
					continue
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

		case *parser2.If:
			rs, ok2 := extractIfElse(s, lines)
			if !ok2 {
				return classification{}, false
			}
			stmts = append(stmts, rs)

		case *parser2.While:
			rs, ok2 := extractWhileAssign(s)
			if !ok2 {
				return classification{}, false
			}
			stmts = append(stmts, rs)

		case *parser2.For:
			rs, ok2 := extractForAssign(s)
			if !ok2 {
				return classification{}, false
			}
			stmts = append(stmts, rs)

		case *parser2.FunctionDef:
			rs, ok2 := extractFuncDef(s)
			if !ok2 {
				return classification{}, false
			}
			stmts = append(stmts, rs)

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

// extractExprAssign tries to parse s.Value as a BinOp(Name, Name),
// UnaryOp(Name), BoolOp, IfExp, or collection literal assignment.
// target is the already-validated single LHS name.
// lines is the split source passed through for collection closing-col lookup.
func extractExprAssign(line int, target *parser2.Name, value parser2.Expr, lines [][]byte) (rawStmt, bool) {
	targetLen := byte(len(target.Id))

	switch e := value.(type) {
	case *parser2.List:
		return extractCollection(line, target, bytecode.CollList, e.P.Col, e.Elts, lines)

	case *parser2.Tuple:
		return extractCollection(line, target, bytecode.CollTuple, e.P.Col, e.Elts, lines)

	case *parser2.Set:
		return extractCollection(line, target, bytecode.CollSet, e.P.Col, e.Elts, lines)

	case *parser2.Dict:
		if len(e.Keys) != len(e.Values) {
			return rawStmt{}, false
		}
		// Flatten keys and values into alternating elts.
		flatElts := make([]parser2.Expr, 0, 2*len(e.Keys))
		for i, k := range e.Keys {
			if k == nil {
				return rawStmt{}, false // **other unpacking not yet supported
			}
			flatElts = append(flatElts, k, e.Values[i])
		}
		return extractCollection(line, target, bytecode.CollDict, e.P.Col, flatElts, lines)

	case *parser2.BoolOp:
		if len(e.Values) != 2 {
			return rawStmt{}, false // chained bool ops deferred
		}
		leftName, leftOK := e.Values[0].(*parser2.Name)
		rightName, rightOK := e.Values[1].(*parser2.Name)
		if !leftOK || !rightOK {
			return rawStmt{}, false
		}
		if len(leftName.Id) > 15 || len(rightName.Id) > 15 {
			return rawStmt{}, false
		}
		if leftName.P.Col > 255 || rightName.P.Col > 255 {
			return rawStmt{}, false
		}
		return rawStmt{
			line:    line,
			endLine: line,
			kind:    stmtBoolOp,
			boolAsgn: boolAssign{
				line:      line,
				target:    target.Id,
				targetLen: targetLen,
				leftName:  leftName.Id,
				leftCol:   byte(leftName.P.Col),
				leftLen:   byte(len(leftName.Id)),
				rightName: rightName.Id,
				rightCol:  byte(rightName.P.Col),
				rightLen:  byte(len(rightName.Id)),
				isOr:      e.Op == "Or",
			},
		}, true

	case *parser2.IfExp:
		condName, condOK := e.Test.(*parser2.Name)
		trueName, trueOK := e.Body.(*parser2.Name)
		falseName, falseOK := e.OrElse.(*parser2.Name)
		if !condOK || !trueOK || !falseOK {
			return rawStmt{}, false
		}
		if len(condName.Id) > 15 || len(trueName.Id) > 15 || len(falseName.Id) > 15 {
			return rawStmt{}, false
		}
		if condName.P.Col > 255 || trueName.P.Col > 255 || falseName.P.Col > 255 {
			return rawStmt{}, false
		}
		return rawStmt{
			line:    line,
			endLine: line,
			kind:    stmtTernary,
			ternaryAsgn: ternaryAssign{
				line:      line,
				target:    target.Id,
				targetLen: targetLen,
				condName:  condName.Id,
				condCol:   byte(condName.P.Col),
				condLen:   byte(len(condName.Id)),
				trueName:  trueName.Id,
				trueCol:   byte(trueName.P.Col),
				trueLen:   byte(len(trueName.Id)),
				falseName: falseName.Id,
				falseCol:  byte(falseName.P.Col),
				falseLen:  byte(len(falseName.Id)),
			},
		}, true

	case *parser2.Call:
		return extractCallAssign(line, target, e, lines)

	case *parser2.Subscript:
		return extractSubscriptLoad(line, target, e)

	case *parser2.Attribute:
		return extractAttrLoad(line, target, e)

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

// extractCollection builds a stmtCollection rawStmt from a collection literal
// whose elements are name-only and all on the same source line.
// lines is the split source (from classifyAST) needed to compute the
// closing-bracket column from the trimmed line end.
func extractCollection(line int, target *parser2.Name, kind bytecode.CollKind, openCol int, eltsExprs []parser2.Expr, lines [][]byte) (rawStmt, bool) {
	if openCol > 255 {
		return rawStmt{}, false
	}
	targetLen := byte(len(target.Id))
	// Validate all elements are names on the same line.
	elts := make([]bytecode.CollElt, 0, len(eltsExprs))
	for _, expr := range eltsExprs {
		n, isName := expr.(*parser2.Name)
		if !isName {
			return rawStmt{}, false
		}
		if n.P.Line != line || n.P.Col > 255 || len(n.Id) > 15 {
			return rawStmt{}, false
		}
		elts = append(elts, bytecode.CollElt{
			Name:    n.Id,
			Col:     byte(n.P.Col),
			NameLen: byte(len(n.Id)),
		})
	}
	// closeEnd = lineEndCol for the single-line collection.
	if line < 1 || line > len(lines) {
		return rawStmt{}, false
	}
	ln := trimRight(stripLineComment(lines[line-1]))
	if len(ln) > 255 {
		return rawStmt{}, false
	}
	closeEnd := byte(len(ln))
	return rawStmt{
		line:    line,
		endLine: line,
		kind:    stmtCollection,
		collAsgn: collectionAssign{
			line:      line,
			target:    target.Id,
			targetLen: targetLen,
			openCol:   byte(openCol),
			closeEnd:  closeEnd,
			kind:      kind,
			elts:      elts,
		},
	}, true
}

// extractCallAssign handles `x = f(args...)` where f and all positional
// args are names and there are no keyword arguments.
func extractCallAssign(line int, target *parser2.Name, e *parser2.Call, lines [][]byte) (rawStmt, bool) {
	funcName, funcOK := e.Func.(*parser2.Name)
	if !funcOK || len(e.Keywords) > 0 {
		return rawStmt{}, false
	}
	if funcName.P.Col > 255 || len(funcName.Id) > 15 || len(target.Id) > 15 {
		return rawStmt{}, false
	}
	args := make([]bytecode.CallArg, 0, len(e.Args))
	for _, a := range e.Args {
		n, isName := a.(*parser2.Name)
		if !isName || n.P.Col > 255 || len(n.Id) > 15 {
			return rawStmt{}, false
		}
		args = append(args, bytecode.CallArg{
			Name:    n.Id,
			Col:     byte(n.P.Col),
			NameLen: byte(len(n.Id)),
		})
	}
	if line < 1 || line > len(lines) {
		return rawStmt{}, false
	}
	ln := trimRight(stripLineComment(lines[line-1]))
	if len(ln) > 255 {
		return rawStmt{}, false
	}
	funcEnd := byte(funcName.P.Col) + byte(len(funcName.Id))
	return rawStmt{
		line:    line,
		endLine: line,
		kind:    stmtCallAssign,
		callAsgn: callAssign{
			line:       line,
			funcName:   funcName.Id,
			funcCol:    byte(funcName.P.Col),
			funcEnd:    funcEnd,
			args:       args,
			targetName: target.Id,
			targetLen:  byte(len(target.Id)),
			closeEnd:   byte(len(ln)),
		},
	}, true
}

// extractSubscriptLoad handles `x = a[b]` where a and b are names.
func extractSubscriptLoad(line int, target *parser2.Name, e *parser2.Subscript) (rawStmt, bool) {
	obj, objOK := e.Value.(*parser2.Name)
	key, keyOK := e.Slice.(*parser2.Name)
	if !objOK || !keyOK {
		return rawStmt{}, false
	}
	if obj.P.Col > 255 || key.P.Col > 255 {
		return rawStmt{}, false
	}
	if len(obj.Id) > 15 || len(key.Id) > 15 || len(target.Id) > 15 {
		return rawStmt{}, false
	}
	objEnd := byte(obj.P.Col) + byte(len(obj.Id))
	keyEnd := byte(key.P.Col) + byte(len(key.Id))
	return rawStmt{
		line:    line,
		endLine: line,
		kind:    stmtSubscriptLoad,
		subAsgn: subscriptAssign{
			line:       line,
			isLoad:     true,
			targetName: target.Id,
			targetLen:  byte(len(target.Id)),
			objName:    obj.Id,
			objCol:     byte(obj.P.Col),
			objEnd:     objEnd,
			keyName:    key.Id,
			keyCol:     byte(key.P.Col),
			keyEnd:     keyEnd,
			closeEnd:   keyEnd + 1, // col after ']'
		},
	}, true
}

// extractSubscriptStore handles `a[b] = x` where a, b and x are names.
func extractSubscriptStore(line int, tgt *parser2.Subscript, value parser2.Expr) (rawStmt, bool) {
	obj, objOK := tgt.Value.(*parser2.Name)
	key, keyOK := tgt.Slice.(*parser2.Name)
	val, valOK := value.(*parser2.Name)
	if !objOK || !keyOK || !valOK {
		return rawStmt{}, false
	}
	if obj.P.Col > 255 || key.P.Col > 255 || val.P.Col > 255 {
		return rawStmt{}, false
	}
	if len(obj.Id) > 15 || len(key.Id) > 15 || len(val.Id) > 15 {
		return rawStmt{}, false
	}
	objEnd := byte(obj.P.Col) + byte(len(obj.Id))
	keyEnd := byte(key.P.Col) + byte(len(key.Id))
	valEnd := byte(val.P.Col) + byte(len(val.Id))
	return rawStmt{
		line:    line,
		endLine: line,
		kind:    stmtSubscriptStore,
		subAsgn: subscriptAssign{
			line:     line,
			isLoad:   false,
			valName:  val.Id,
			valCol:   byte(val.P.Col),
			valEnd:   valEnd,
			objName:  obj.Id,
			objCol:   byte(obj.P.Col),
			objEnd:   objEnd,
			keyName:  key.Id,
			keyCol:   byte(key.P.Col),
			keyEnd:   keyEnd,
			closeEnd: keyEnd + 1,
		},
	}, true
}

// extractAttrLoad handles `x = a.b` where a is a name.
func extractAttrLoad(line int, target *parser2.Name, e *parser2.Attribute) (rawStmt, bool) {
	obj, objOK := e.Value.(*parser2.Name)
	if !objOK {
		return rawStmt{}, false
	}
	if obj.P.Col > 255 {
		return rawStmt{}, false
	}
	if len(obj.Id) > 15 || len(e.Attr) > 15 || len(target.Id) > 15 {
		return rawStmt{}, false
	}
	objEnd := byte(obj.P.Col) + byte(len(obj.Id))
	attrEnd := objEnd + 1 + byte(len(e.Attr)) // +1 for the '.'
	return rawStmt{
		line:    line,
		endLine: line,
		kind:    stmtAttrLoad,
		attrAsgn: attrAssign{
			line:       line,
			isLoad:     true,
			targetName: target.Id,
			targetLen:  byte(len(target.Id)),
			objName:    obj.Id,
			objCol:     byte(obj.P.Col),
			objEnd:     objEnd,
			attrName:   e.Attr,
			attrEnd:    attrEnd,
		},
	}, true
}

// extractAttrStore handles `a.b = x` where a and x are names.
func extractAttrStore(line int, tgt *parser2.Attribute, value parser2.Expr) (rawStmt, bool) {
	obj, objOK := tgt.Value.(*parser2.Name)
	val, valOK := value.(*parser2.Name)
	if !objOK || !valOK {
		return rawStmt{}, false
	}
	if obj.P.Col > 255 || val.P.Col > 255 {
		return rawStmt{}, false
	}
	if len(obj.Id) > 15 || len(tgt.Attr) > 15 || len(val.Id) > 15 {
		return rawStmt{}, false
	}
	objEnd := byte(obj.P.Col) + byte(len(obj.Id))
	attrEnd := objEnd + 1 + byte(len(tgt.Attr))
	valEnd := byte(val.P.Col) + byte(len(val.Id))
	return rawStmt{
		line:    line,
		endLine: line,
		kind:    stmtAttrStore,
		attrAsgn: attrAssign{
			line:     line,
			isLoad:   false,
			valName:  val.Id,
			valCol:   byte(val.P.Col),
			valEnd:   valEnd,
			objName:  obj.Id,
			objCol:   byte(obj.P.Col),
			objEnd:   objEnd,
			attrName: tgt.Attr,
			attrEnd:  attrEnd,
		},
	}, true
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

// extractIfElse extracts an if/elif/else chain where each branch body is
// `name = small_int` (0-255). Returns stmtIfElse on success.
func extractIfElse(s *parser2.If, lines [][]byte) (rawStmt, bool) {
	_ = lines // reserved for future use
	var branches []ifElseBranch
	var hasElse bool
	var elseLine int
	var elseVal byte
	var elseVarName string
	var elseVarCol, elseVarEnd, elseValCol, elseValEnd byte

	cur := s
	for cur != nil {
		cond, condOK := cur.Test.(*parser2.Name)
		if !condOK || cond.P.Col > 255 || len(cond.Id) > 15 {
			return rawStmt{}, false
		}
		condLine := cur.P.Line
		condCol := byte(cond.P.Col)
		condEnd := condCol + byte(len(cond.Id))

		if len(cur.Body) != 1 {
			return rawStmt{}, false
		}
		bodyAssign, isAssign := cur.Body[0].(*parser2.Assign)
		if !isAssign || len(bodyAssign.Targets) != 1 {
			return rawStmt{}, false
		}
		varName, isName := bodyAssign.Targets[0].(*parser2.Name)
		if !isName || varName.P.Col > 255 || len(varName.Id) > 15 {
			return rawStmt{}, false
		}
		constVal, isConst := bodyAssign.Value.(*parser2.Constant)
		if !isConst || constVal.Kind != "int" || constVal.P.Col > 255 {
			return rawStmt{}, false
		}
		iv := constVal.Value.(int64)
		if iv < 0 || iv > 255 {
			return rawStmt{}, false
		}
		bodyLine := bodyAssign.P.Line
		vc := byte(constVal.P.Col)
		ve := vc + byte(len(strconv.Itoa(int(iv))))
		vrc := byte(varName.P.Col)
		vre := vrc + byte(len(varName.Id))

		branches = append(branches, ifElseBranch{
			condName: cond.Id,
			condLine: condLine,
			condCol:  condCol,
			condEnd:  condEnd,
			bodyLine: bodyLine,
			bodyVal:  byte(iv),
			varName:  varName.Id,
			varCol:   vrc,
			varEnd:   vre,
			valCol:   vc,
			valEnd:   ve,
		})

		if len(cur.Orelse) == 0 {
			cur = nil
		} else if len(cur.Orelse) == 1 {
			if elif, isIf := cur.Orelse[0].(*parser2.If); isIf {
				cur = elif
			} else {
				// else body: must be one assign
				elseAssign, isAssign2 := cur.Orelse[0].(*parser2.Assign)
				if !isAssign2 || len(elseAssign.Targets) != 1 {
					return rawStmt{}, false
				}
				ev, isName2 := elseAssign.Targets[0].(*parser2.Name)
				if !isName2 || ev.P.Col > 255 || len(ev.Id) > 15 {
					return rawStmt{}, false
				}
				ec, isConst2 := elseAssign.Value.(*parser2.Constant)
				if !isConst2 || ec.Kind != "int" || ec.P.Col > 255 {
					return rawStmt{}, false
				}
				eiv := ec.Value.(int64)
				if eiv < 0 || eiv > 255 {
					return rawStmt{}, false
				}
				hasElse = true
				elseLine = elseAssign.P.Line
				elseVal = byte(eiv)
				elseVarName = ev.Id
				elseVarCol = byte(ev.P.Col)
				elseVarEnd = elseVarCol + byte(len(ev.Id))
				elseValCol = byte(ec.P.Col)
				elseValEnd = elseValCol + byte(len(strconv.Itoa(int(eiv))))
				cur = nil
			}
		} else {
			return rawStmt{}, false
		}
	}

	if len(branches) == 0 {
		return rawStmt{}, false
	}
	return rawStmt{
		line:    branches[0].condLine,
		endLine: branches[len(branches)-1].bodyLine,
		kind:    stmtIfElse,
		ifElseAsgn: ifElseClassify{
			branches:    branches,
			hasElse:     hasElse,
			elseLine:    elseLine,
			elseVal:     elseVal,
			elseVarName: elseVarName,
			elseVarCol:  elseVarCol,
			elseVarEnd:  elseVarEnd,
			elseValCol:  elseValCol,
			elseValEnd:  elseValEnd,
		},
	}, true
}

// extractWhileAssign extracts a simple `while cond: name = val` loop where
// cond is a name and val is a small integer (0-255), with no else/break/continue.
func extractWhileAssign(s *parser2.While) (rawStmt, bool) {
	if len(s.Orelse) != 0 {
		return rawStmt{}, false
	}
	cond, condOK := s.Test.(*parser2.Name)
	if !condOK || cond.P.Col > 255 || len(cond.Id) > 15 {
		return rawStmt{}, false
	}
	if len(s.Body) != 1 {
		return rawStmt{}, false
	}
	bodyAssign, isAssign := s.Body[0].(*parser2.Assign)
	if !isAssign || len(bodyAssign.Targets) != 1 {
		return rawStmt{}, false
	}
	varName, isName := bodyAssign.Targets[0].(*parser2.Name)
	if !isName || varName.P.Col > 255 || len(varName.Id) > 15 {
		return rawStmt{}, false
	}
	constVal, isConst := bodyAssign.Value.(*parser2.Constant)
	if !isConst || constVal.Kind != "int" || constVal.P.Col > 255 {
		return rawStmt{}, false
	}
	iv := constVal.Value.(int64)
	if iv < 0 || iv > 255 {
		return rawStmt{}, false
	}
	condLine := s.P.Line
	condCol := byte(cond.P.Col)
	condEnd := condCol + byte(len(cond.Id))
	bodyLine := bodyAssign.P.Line
	vc := byte(constVal.P.Col)
	ve := vc + byte(len(strconv.Itoa(int(iv))))
	vrc := byte(varName.P.Col)
	vre := vrc + byte(len(varName.Id))
	return rawStmt{
		line:    condLine,
		endLine: bodyLine,
		kind:    stmtWhile,
		whileAsgn: whileAssign{
			condName: cond.Id,
			condLine: condLine,
			condCol:  condCol,
			condEnd:  condEnd,
			bodyLine: bodyLine,
			bodyVal:  byte(iv),
			varName:  varName.Id,
			varCol:   vrc,
			varEnd:   vre,
			valCol:   vc,
			valEnd:   ve,
		},
	}, true
}

// extractForAssign extracts a simple `for loopVar in iter: bodyVar = val` loop
// where iter and loopVar are names and val is a small integer (0-255), with no else/break.
func extractForAssign(s *parser2.For) (rawStmt, bool) {
	if len(s.Orelse) != 0 {
		return rawStmt{}, false
	}
	iter, iterOK := s.Iter.(*parser2.Name)
	if !iterOK || iter.P.Col > 255 || len(iter.Id) > 15 {
		return rawStmt{}, false
	}
	loopVar, loopVarOK := s.Target.(*parser2.Name)
	if !loopVarOK || loopVar.P.Col > 255 || len(loopVar.Id) > 15 {
		return rawStmt{}, false
	}
	if len(s.Body) != 1 {
		return rawStmt{}, false
	}
	bodyAssign, isAssign := s.Body[0].(*parser2.Assign)
	if !isAssign || len(bodyAssign.Targets) != 1 {
		return rawStmt{}, false
	}
	bodyVar, isName := bodyAssign.Targets[0].(*parser2.Name)
	if !isName || bodyVar.P.Col > 255 || len(bodyVar.Id) > 15 {
		return rawStmt{}, false
	}
	constVal, isConst := bodyAssign.Value.(*parser2.Constant)
	if !isConst || constVal.Kind != "int" || constVal.P.Col > 255 {
		return rawStmt{}, false
	}
	iv := constVal.Value.(int64)
	if iv < 0 || iv > 255 {
		return rawStmt{}, false
	}
	forLine := s.P.Line
	iterCol := byte(iter.P.Col)
	iterEnd := iterCol + byte(len(iter.Id))
	loopVarCol := byte(loopVar.P.Col)
	loopVarEnd := loopVarCol + byte(len(loopVar.Id))
	bodyLine := bodyAssign.P.Line
	vc := byte(constVal.P.Col)
	ve := vc + byte(len(strconv.Itoa(int(iv))))
	bvc := byte(bodyVar.P.Col)
	bve := bvc + byte(len(bodyVar.Id))
	return rawStmt{
		line:    forLine,
		endLine: bodyLine,
		kind:    stmtFor,
		forAsgn: forAssign{
			iterName:    iter.Id,
			iterCol:     iterCol,
			iterEnd:     iterEnd,
			loopVarName: loopVar.Id,
			loopVarCol:  loopVarCol,
			loopVarEnd:  loopVarEnd,
			forLine:     forLine,
			bodyLine:    bodyLine,
			bodyVal:     byte(iv),
			bodyVarName: bodyVar.Id,
			bodyVarCol:  bvc,
			bodyVarEnd:  bve,
			valCol:      vc,
			valEnd:      ve,
		},
	}, true
}

// extractFuncDef extracts a simple `def f(arg): return arg` function
// definition where f and arg are single identifiers with no decorators,
// annotations, defaults, *args, or **kwargs.
func extractFuncDef(s *parser2.FunctionDef) (rawStmt, bool) {
	if len(s.DecoratorList) != 0 || s.Returns != nil || len(s.TypeParams) != 0 {
		return rawStmt{}, false
	}
	args := s.Args
	if args == nil || len(args.PosOnly) != 0 || len(args.KwOnly) != 0 ||
		args.Vararg != nil || args.Kwarg != nil || len(args.Defaults) != 0 {
		return rawStmt{}, false
	}
	if len(args.Args) != 1 {
		return rawStmt{}, false
	}
	arg := args.Args[0]
	if arg.Annotation != nil || arg.P.Col > 255 || len(arg.Name) > 15 {
		return rawStmt{}, false
	}
	if len(s.Name) > 15 || s.P.Col > 255 {
		return rawStmt{}, false
	}
	switch len(s.Body) {
	case 1:
		ret, isReturn := s.Body[0].(*parser2.Return)
		if !isReturn || ret.Value == nil {
			return rawStmt{}, false
		}
		retName, isName := ret.Value.(*parser2.Name)
		if !isName || retName.Id != arg.Name {
			return rawStmt{}, false
		}
		if ret.P.Col > 255 || retName.P.Col > 255 {
			return rawStmt{}, false
		}
		defLine := s.P.Line
		bodyLine := ret.P.Line
		retKwCol := byte(ret.P.Col)
		argCol := byte(retName.P.Col)
		argEnd := argCol + byte(len(retName.Id))
		return rawStmt{
			line:    defLine,
			endLine: bodyLine,
			kind:    stmtFuncDef,
			funcDefAsgn: funcDefClassify{
				funcName: s.Name,
				argName:  arg.Name,
				defLine:  defLine,
				bodyLine: bodyLine,
				retKwCol: retKwCol,
				argCol:   argCol,
				argEnd:   argEnd,
			},
		}, true
	case 2:
		return extractClosure(s, arg.Name)
	}
	return rawStmt{}, false
}

// extractClosure recognises `def f(outerArg): def g(): return outerArg; return g`
// and returns a stmtClosureDef rawStmt with all source positions.
func extractClosure(outer *parser2.FunctionDef, outerArgName string) (rawStmt, bool) {
	// Body[0]: inner def with 0 args, single body statement `return outerArgName`
	innerDef, ok := outer.Body[0].(*parser2.FunctionDef)
	if !ok {
		return rawStmt{}, false
	}
	if len(innerDef.DecoratorList) != 0 || innerDef.Returns != nil || len(innerDef.TypeParams) != 0 {
		return rawStmt{}, false
	}
	innerArgs := innerDef.Args
	if innerArgs == nil || len(innerArgs.Args) != 0 || len(innerArgs.PosOnly) != 0 ||
		len(innerArgs.KwOnly) != 0 || innerArgs.Vararg != nil || innerArgs.Kwarg != nil {
		return rawStmt{}, false
	}
	if len(innerDef.Body) != 1 {
		return rawStmt{}, false
	}
	innerRet, ok := innerDef.Body[0].(*parser2.Return)
	if !ok || innerRet.Value == nil {
		return rawStmt{}, false
	}
	innerRetName, ok := innerRet.Value.(*parser2.Name)
	if !ok || innerRetName.Id != outerArgName {
		return rawStmt{}, false
	}

	// Body[1]: `return innerFuncName`
	outerRet, ok := outer.Body[1].(*parser2.Return)
	if !ok || outerRet.Value == nil {
		return rawStmt{}, false
	}
	outerRetName, ok := outerRet.Value.(*parser2.Name)
	if !ok || outerRetName.Id != innerDef.Name {
		return rawStmt{}, false
	}

	// Bounds checks
	if outer.P.Col > 255 || innerDef.P.Col > 255 {
		return rawStmt{}, false
	}
	if innerRet.P.Col > 255 || innerRetName.P.Col > 255 {
		return rawStmt{}, false
	}
	if outerRet.P.Col > 255 || outerRetName.P.Col > 255 {
		return rawStmt{}, false
	}
	if len(outer.Name) > 15 || len(outerArgName) > 15 || len(innerDef.Name) > 15 {
		return rawStmt{}, false
	}

	innerBodyEndCol := byte(innerRetName.P.Col) + byte(len(innerRetName.Id))
	outerRetArgEnd := byte(outerRetName.P.Col) + byte(len(outerRetName.Id))

	return rawStmt{
		line:    outer.P.Line,
		endLine: outerRet.P.Line,
		kind:    stmtClosureDef,
		closureAsgn: closureDef{
			outerFuncName: outer.Name,
			argName:       outerArgName,
			innerFuncName: innerDef.Name,
			outerDefLine:  outer.P.Line,
			innerDefLine:  innerDef.P.Line,
			innerRetLine:  innerRet.P.Line,
			outerRetLine:  outerRet.P.Line,
			innerDefCol:      byte(innerDef.P.Col),
			innerBodyEndCol:  innerBodyEndCol,
			innerFreeArgCol:  byte(innerRetName.P.Col),
			innerFreeArgEnd:  innerBodyEndCol,
			innerRetKwCol:    byte(innerRet.P.Col),
			outerRetArgCol:   byte(outerRetName.P.Col),
			outerRetArgEnd:   outerRetArgEnd,
			outerRetKwCol:    byte(outerRet.P.Col),
		},
	}, true
}

// extractGenExprAssign checks whether value is a general expression
// composed recursively of Name, small-int Constant, BinOp, and
// UnaryOp (USub/Invert) nodes on a single source line. Returns a
// stmtGenExpr rawStmt on success.
func extractGenExprAssign(line int, lineEndCol byte, target *parser2.Name, value parser2.Expr, lines [][]byte) (rawStmt, bool) {
	if len(target.Id) > 15 || target.P.Col > 255 {
		return rawStmt{}, false
	}
	if !isGenExpr(value) {
		return rawStmt{}, false
	}
	return rawStmt{
		line:    line,
		endLine: line,
		kind:    stmtGenExpr,
		genExprAsgn: genExprInfo{
			targetName: target.Id,
			targetLen:  byte(len(target.Id)),
			line:       line,
			lineEndCol: lineEndCol,
			expr:       value,
			srcLines:   lines,
		},
	}, true
}

// isGenExpr reports whether e is recursively a valid general expression:
// Name, small-int Constant (0-255), BinOp with supported op, or
// UnaryOp (USub/Invert).
func isGenExpr(e parser2.Expr) bool {
	switch n := e.(type) {
	case *parser2.Name:
		return len(n.Id) <= 15 && n.P.Col <= 255
	case *parser2.Constant:
		if n.P.Col > 255 {
			return false
		}
		switch n.Kind {
		case "int":
			iv, ok := n.Value.(int64)
			return ok && iv >= 0 && iv <= 255
		case "None", "True", "False", "float", "complex":
			return false // defer to later release
		}
		return false
	case *parser2.BinOp:
		_, ok := binOpargFromOp(n.Op)
		return ok && isGenExpr(n.Left) && isGenExpr(n.Right)
	case *parser2.UnaryOp:
		if n.Op != "USub" && n.Op != "Invert" {
			return false
		}
		return isGenExpr(n.Operand)
	}
	return false
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
