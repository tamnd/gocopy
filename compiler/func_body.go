package compiler

import (
	"strconv"

	"github.com/tamnd/gocopy/bytecode"
	parser2 "github.com/tamnd/gopapy/parser"
)

// compileFuncBodyExpr lowers a function definition whose body is composed
// of zero or more local-assignment statements followed by one return
// statement. All expressions are recursively composed of Name nodes
// (params and assigned locals), BinOp, and UnaryOp (USub/Invert).
func compileFuncBodyExpr(filename string, cls classification) (*bytecode.CodeObject, error) {
	g := cls.funcBodyAsgn

	// Build the slot map: params first (slots 0..len(params)-1),
	// then body-assigned locals in declaration order.
	slots := map[string]byte{}
	localsPlusNames := make([]string, 0, len(g.params)+8)
	localsPlusKinds := make([]byte, 0, len(g.params)+8)
	for i, p := range g.params {
		slots[p.name] = byte(i)
		localsPlusNames = append(localsPlusNames, p.name)
		localsPlusKinds = append(localsPlusKinds, 0x26) // CO_FAST_LOCAL | CO_FAST_ARG
	}
	// Assign slots to body locals in declaration order.
	nextSlot := byte(len(g.params))
	for _, st := range g.stmts {
		if st.isReturn || st.isIfReturn || st.isIfAssign {
			continue
		}
		if _, ok := slots[st.targetName]; !ok {
			slots[st.targetName] = nextSlot
			localsPlusNames = append(localsPlusNames, st.targetName)
			localsPlusKinds = append(localsPlusKinds, 0x20) // CO_FAST_LOCAL
			nextSlot++
		}
	}

	fs := newFuncState(g.defLine, slots, g.srcLines)

	// RESUME at def line: SHORT0(1)[0,0)
	fs.bc = append(fs.bc, byte(bytecode.RESUME), 0)
	fs.lt = bytecode.GenExprSameLine(fs.lt, 1, 0, 0)

	// Compile each body statement.
	for _, st := range g.stmts {
		fs.newStmt(st.line)
		if st.isReturn {
			if constExpr, isConst := st.expr.(*parser2.Constant); isConst {
				// Single constant: emit load (no lt entry) + RETURN_VALUE
				// as one combined 2-CU linetable entry.
				sc, ec := fs.loadConst(constExpr)
				fs.trackDepth(1)
				fs.bc = append(fs.bc, byte(bytecode.RETURN_VALUE), 0)
				fs.emit(2, sc, ec)
			} else if ifexpr, isIfExpr := st.expr.(*parser2.IfExp); isIfExpr {
				// Ternary return: `return Body if Test else OrElse`
				// Pre-compute ternEnd (= end of OrElse) from the AST so we can
				// emit the true-branch RETURN_VALUE linetable entry before walking
				// OrElse (linetable entries must be in CU order).
				ternEnd := fs.exprEndCol(ifexpr.OrElse)

				cmpNode := ifexpr.Test.(*parser2.Compare)
				_, cmpBase, _ := cmpOpFromOp(cmpNode.Ops[0])
				cmpOparg := cmpBase + 16
				cmpCacheWords := int(bytecode.CacheSize[bytecode.COMPARE_OP])
				pjifCacheWords := int(bytecode.CacheSize[bytecode.POP_JUMP_IF_FALSE])

				leftExpr := cmpNode.Left
				rightExpr := cmpNode.Comparators[0]

				lflblflb := false
				var condStart, condEnd byte
				if ln, lok := leftExpr.(*parser2.Name); lok {
					if rn, rok := rightExpr.(*parser2.Name); rok {
						ls, lOK := fs.slots[ln.Id]
						rs, rOK := fs.slots[rn.Id]
						if lOK && rOK && ls <= 15 && rs <= 15 {
							lsc := byte(ln.P.Col)
							lec := lsc + byte(len(ln.Id))
							rec := byte(rn.P.Col) + byte(len(rn.Id))
							fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
							fs.emit(1, lsc, lec)
							fs.trackDepth(2)
							condStart, condEnd = lsc, rec
							lflblflb = true
						}
					}
				}
				if !lflblflb {
					cs, _, _ := fs.walkExpr(leftExpr)
					_, ce, _ := fs.walkExpr(rightExpr)
					fs.trackDepth(2)
					condStart, condEnd = cs, ce
				}
				fs.bc = append(fs.bc, byte(bytecode.COMPARE_OP), cmpOparg)
				for range cmpCacheWords {
					fs.bc = append(fs.bc, 0, 0)
				}
				pjifPos := len(fs.bc) + 1
				fs.bc = append(fs.bc, byte(bytecode.POP_JUMP_IF_FALSE), 0)
				for range pjifCacheWords {
					fs.bc = append(fs.bc, 0, 0)
				}
				fs.bc = append(fs.bc, byte(bytecode.NOT_TAKEN), 0)
				fs.emitSame(1+cmpCacheWords+1+pjifCacheWords+1, condStart, condEnd)

				// True branch: walkExpr emits load + linetable for Body.
				fs.walkExpr(ifexpr.Body) //nolint:unused
				fs.bc = append(fs.bc, byte(bytecode.RETURN_VALUE), 0)
				fs.emitSame(1, st.retKwCol, ternEnd) // CU after true load

				// Backpatch: false branch starts at current position.
				pjifCU := (pjifPos - 1) / 2
				targetCU := len(fs.bc) / 2
				fs.bc[pjifPos] = byte(targetCU - (pjifCU + 1 + pjifCacheWords))

				// False branch: local Name uses LOAD_FAST; global Name and other
				// exprs use walkExpr.
				var falseEnd byte
				if falseName, isFN := ifexpr.OrElse.(*parser2.Name); isFN {
					slot, isLocal := fs.slots[falseName.Id]
					falseStart := byte(falseName.P.Col)
					falseEnd = falseStart + byte(len(falseName.Id))
					if isLocal {
						fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST), slot)
						fs.emitSame(1, falseStart, falseEnd)
						fs.trackDepth(1)
						fs.lastExprEnd = falseEnd
					} else {
						_, falseEnd2, _ := fs.walkExpr(falseName)
						falseEnd = falseEnd2
					}
				} else {
					_, falseEnd2, _ := fs.walkExpr(ifexpr.OrElse)
					falseEnd = falseEnd2
				}
				fs.bc = append(fs.bc, byte(bytecode.RETURN_VALUE), 0)
				fs.emitSame(1, st.retKwCol, falseEnd)
			} else {
				_, exprEnd, _ := fs.walkExpr(st.expr)
				fs.bc = append(fs.bc, byte(bytecode.RETURN_VALUE), 0)
				fs.emitSame(1, st.retKwCol, exprEnd)
			}
		} else if st.isIfReturn {
			// `if <cond>: return <expr>` early-return pattern.
			cmpNode := st.condExpr.(*parser2.Compare)
			op, cmpBase, _ := cmpOpFromOp(cmpNode.Ops[0])

			var pjifPos int
			var pjifCacheWords int

			if op == bytecode.IS_OP {
				// `if x is None:` / `if x is not None:` — single operand load +
				// POP_JUMP_IF_NOT_NONE / POP_JUMP_IF_NONE + 1 cache + NOT_TAKEN.
				fs.noneCheckFunc = true // integers not stored in co_consts for None-check functions
				varName := cmpNode.Left.(*parser2.Name)
				noneConst := cmpNode.Comparators[0].(*parser2.Constant)
				varSlot := fs.slots[varName.Id]
				varStart := byte(varName.P.Col)
				varEnd := varStart + byte(len(varName.Id))
				condEnd := byte(noneConst.P.Col) + 4 // "None" = 4 chars

				var jumpOp bytecode.Opcode
				if cmpBase == 0 { // "is" → jump past then-branch when NOT None
					jumpOp = bytecode.POP_JUMP_IF_NOT_NONE
				} else { // "is not" → jump past then-branch when None
					jumpOp = bytecode.POP_JUMP_IF_NONE
				}

				fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW), varSlot)
				fs.emit(1, varStart, varEnd)
				fs.trackDepth(1)

				pjifCacheWords = int(bytecode.CacheSize[jumpOp])
				pjifPos = len(fs.bc) + 1
				fs.bc = append(fs.bc, byte(jumpOp), 0)
				for range pjifCacheWords {
					fs.bc = append(fs.bc, 0, 0)
				}
				fs.bc = append(fs.bc, byte(bytecode.NOT_TAKEN), 0)
				fs.emitSame(1+pjifCacheWords+1, varStart, condEnd)
			} else {
				// COMPARE_OP (conditional) + POP_JUMP_IF_FALSE path.
				cmpOparg := cmpBase + 16
				cmpCacheWords := int(bytecode.CacheSize[bytecode.COMPARE_OP])
				pjifCacheWords = int(bytecode.CacheSize[bytecode.POP_JUMP_IF_FALSE])

				leftExpr := cmpNode.Left
				rightExpr := cmpNode.Comparators[0]

				lflblflbCond := false
				var condStart, condEnd byte
				if ln, lok := leftExpr.(*parser2.Name); lok {
					if rn, rok := rightExpr.(*parser2.Name); rok {
						ls, lOK := fs.slots[ln.Id]
						rs, rOK := fs.slots[rn.Id]
						if lOK && rOK && ls <= 15 && rs <= 15 {
							lsc := byte(ln.P.Col)
							lec := lsc + byte(len(ln.Id))
							rec := byte(rn.P.Col) + byte(len(rn.Id))
							fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
							fs.emit(1, lsc, lec)
							fs.trackDepth(2)
							condStart, condEnd = lsc, rec
							lflblflbCond = true
						}
					}
				}
				if !lflblflbCond {
					cs, _, _ := fs.walkExpr(leftExpr)
					_, ce, _ := fs.walkExpr(rightExpr)
					fs.trackDepth(2)
					condStart, condEnd = cs, ce
				}
				fs.bc = append(fs.bc, byte(bytecode.COMPARE_OP), cmpOparg)
				for range cmpCacheWords {
					fs.bc = append(fs.bc, 0, 0)
				}
				pjifPos = len(fs.bc) + 1
				fs.bc = append(fs.bc, byte(bytecode.POP_JUMP_IF_FALSE), 0)
				for range pjifCacheWords {
					fs.bc = append(fs.bc, 0, 0)
				}
				fs.bc = append(fs.bc, byte(bytecode.NOT_TAKEN), 0)
				fs.emitSame(1+cmpCacheWords+1+pjifCacheWords+1, condStart, condEnd)
			}

			// Then-branch: load return value + RETURN_VALUE.
			fs.newStmt(st.thenLine)
			if constExpr, isConst := st.expr.(*parser2.Constant); isConst {
				sc, ec := fs.loadConst(constExpr)
				fs.trackDepth(1)
				fs.bc = append(fs.bc, byte(bytecode.RETURN_VALUE), 0)
				fs.emit(2, sc, ec)
			} else {
				_, exprEnd, _ := fs.walkExpr(st.expr)
				fs.bc = append(fs.bc, byte(bytecode.RETURN_VALUE), 0)
				fs.emitSame(1, st.retKwCol, exprEnd)
			}

			// Backpatch the jump oparg.
			pjifCU := (pjifPos - 1) / 2
			targetCU := len(fs.bc) / 2
			fs.bc[pjifPos] = byte(targetCU - (pjifCU + 1 + pjifCacheWords))

		} else if st.isIfAssign {
			// `if <cond>: target = expr` conditional assignment (no else).
			cmpNode := st.condExpr.(*parser2.Compare)
			op, cmpBase, _ := cmpOpFromOp(cmpNode.Ops[0])

			var pjifPos int
			var pjifCacheWords int

			if op == bytecode.IS_OP {
				// `if x is None:` / `if x is not None:` path.
				fs.noneCheckFunc = true
				varName := cmpNode.Left.(*parser2.Name)
				noneConst := cmpNode.Comparators[0].(*parser2.Constant)
				varSlot := fs.slots[varName.Id]
				varStart := byte(varName.P.Col)
				varEnd := varStart + byte(len(varName.Id))
				condEnd := byte(noneConst.P.Col) + 4

				var jumpOp bytecode.Opcode
				if cmpBase == 0 {
					jumpOp = bytecode.POP_JUMP_IF_NOT_NONE
				} else {
					jumpOp = bytecode.POP_JUMP_IF_NONE
				}

				fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW), varSlot)
				fs.emit(1, varStart, varEnd)
				fs.trackDepth(1)

				pjifCacheWords = int(bytecode.CacheSize[jumpOp])
				pjifPos = len(fs.bc) + 1
				fs.bc = append(fs.bc, byte(jumpOp), 0)
				for range pjifCacheWords {
					fs.bc = append(fs.bc, 0, 0)
				}
				fs.bc = append(fs.bc, byte(bytecode.NOT_TAKEN), 0)
				fs.emitSame(1+pjifCacheWords+1, varStart, condEnd)
			} else {
				// COMPARE_OP (conditional) + POP_JUMP_IF_FALSE path.
				cmpOparg := cmpBase + 16
				cmpCacheWords := int(bytecode.CacheSize[bytecode.COMPARE_OP])
				pjifCacheWords = int(bytecode.CacheSize[bytecode.POP_JUMP_IF_FALSE])

				leftExpr := cmpNode.Left
				rightExpr := cmpNode.Comparators[0]

				lflblflb := false
				var condStart, condEnd byte
				if ln, lok := leftExpr.(*parser2.Name); lok {
					if rn, rok := rightExpr.(*parser2.Name); rok {
						ls := fs.slots[ln.Id]
						rs := fs.slots[rn.Id]
						if ls <= 15 && rs <= 15 {
							lsc := byte(ln.P.Col)
							lec := lsc + byte(len(ln.Id))
							rec := byte(rn.P.Col) + byte(len(rn.Id))
							fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
							fs.emit(1, lsc, lec)
							fs.trackDepth(2)
							condStart, condEnd = lsc, rec
							lflblflb = true
						}
					}
				}
				if !lflblflb {
					cs, _, _ := fs.walkExpr(leftExpr)
					_, ce, _ := fs.walkExpr(rightExpr)
					fs.trackDepth(2)
					condStart, condEnd = cs, ce
				}
				fs.bc = append(fs.bc, byte(bytecode.COMPARE_OP), cmpOparg)
				for range cmpCacheWords {
					fs.bc = append(fs.bc, 0, 0)
				}
				pjifPos = len(fs.bc) + 1
				fs.bc = append(fs.bc, byte(bytecode.POP_JUMP_IF_FALSE), 0)
				for range pjifCacheWords {
					fs.bc = append(fs.bc, 0, 0)
				}
				fs.bc = append(fs.bc, byte(bytecode.NOT_TAKEN), 0)
				fs.emitSame(1+cmpCacheWords+1+pjifCacheWords+1, condStart, condEnd)
			}

			// Then-body: assignment.
			thenSlot := fs.slots[st.targetName]
			tsc := st.targetCol
			tec := tsc + byte(len(st.targetName))
			fs.newStmt(st.thenLine)
			if nameExpr, isName := st.expr.(*parser2.Name); isName {
				srcSlot, isLocal := fs.slots[nameExpr.Id]
				if isLocal {
					sc := byte(nameExpr.P.Col)
					ec := sc + byte(len(nameExpr.Id))
					fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST), srcSlot)
					fs.emit(1, sc, ec)
					fs.trackDepth(1)
					fs.lastExprEnd = ec
				} else {
					fs.walkExpr(nameExpr) //nolint:unused
				}
			} else {
				fs.walkExpr(st.expr) //nolint:unused
			}
			fs.bc = append(fs.bc, byte(bytecode.STORE_FAST), byte(thenSlot))
			fs.emitSame(1, tsc, tec)

			// Backpatch.
			pjifCU := (pjifPos - 1) / 2
			targetCU := len(fs.bc) / 2
			fs.bc[pjifPos] = byte(targetCU - (pjifCU + 1 + pjifCacheWords))

		} else if st.isAugAssign {
			slot := slots[st.targetName]
			tsc := st.targetCol
			tec := tsc + byte(len(st.targetName))
			cacheWords := int(bytecode.CacheSize[bytecode.BINARY_OP])

			// LFLBLFLB optimisation: target and RHS are both local Name nodes.
			if rhsName, isRhsName := st.expr.(*parser2.Name); isRhsName {
				rs, rsOK := slots[rhsName.Id]
				if rsOK && slot <= 15 && rs <= 15 {
					rec := byte(rhsName.P.Col) + byte(len(rhsName.Id))
					fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (slot<<4)|rs)
					fs.emit(1, tsc, tec)
					fs.emitBinOp(st.augOp, cacheWords, tsc, rec)
					fs.bc = append(fs.bc, byte(bytecode.STORE_FAST), slot)
					fs.emitSame(1, tsc, tec)
					fs.trackDepth(2)
				}
			} else {
				fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW), slot)
				fs.emit(1, tsc, tec)
				_, rhsEnd, _ := fs.walkExpr(st.expr)
				fs.emitBinOp(st.augOp, cacheWords, tsc, rhsEnd)
				fs.bc = append(fs.bc, byte(bytecode.STORE_FAST), slot)
				fs.emitSame(1, tsc, tec)
				fs.trackDepth(2)
			}
		} else {
			// Simple assignment. A bare local-Name RHS uses LOAD_FAST (move
			// semantics); globals and other expressions use walkExpr (BORROW).
			if nameExpr, isName := st.expr.(*parser2.Name); isName {
				srcSlot, isLocal := fs.slots[nameExpr.Id]
				if isLocal {
					sc := byte(nameExpr.P.Col)
					ec := sc + byte(len(nameExpr.Id))
					fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST), srcSlot)
					fs.emit(1, sc, ec)
					fs.trackDepth(1)
					fs.lastExprEnd = ec
				} else {
					fs.walkExpr(nameExpr) //nolint:unused
				}
			} else {
				fs.walkExpr(st.expr) //nolint:unused
			}
			slot := slots[st.targetName]
			fs.bc = append(fs.bc, byte(bytecode.STORE_FAST), slot)
			fs.emitSame(1, st.targetCol, st.targetCol+byte(len(st.targetName)))
		}
	}

	maxDepth := fs.maxDepth
	if maxDepth < 1 {
		maxDepth = 1
	}

	lastStmt := g.stmts[len(g.stmts)-1]
	bodyEndLine := lastStmt.line
	bodyEndCol := fs.lastExprEnd

	innerCode := &bytecode.CodeObject{
		ArgCount:        int32(len(g.params)),
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       int32(maxDepth),
		Flags:           0x3,
		Bytecode:        fs.bc,
		Consts:          fs.buildConsts(),
		Names:           fs.buildNames(),
		LocalsPlusNames: localsPlusNames,
		LocalsPlusKinds: localsPlusKinds,
		Filename:        filename,
		Name:            g.funcName,
		QualName:        g.funcName,
		FirstLineNo:     int32(g.defLine),
		LineTable:       fs.lt,
		ExcTable:        []byte{},
	}

	return &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0,
		Bytecode:        bytecode.FuncDefModuleBytecode(0),
		Consts:          []any{innerCode, nil},
		Names:           []string{g.funcName},
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        filename,
		Name:            "<module>",
		QualName:        "<module>",
		FirstLineNo:     int32(g.defLine),
		LineTable:       bytecode.FuncDefModuleLineTable(g.defLine, bodyEndLine, bodyEndCol),
		ExcTable:        []byte{},
	}, nil
}

// compileFuncBodyCore compiles a function body and returns the inner code object
// together with the last body statement's line and the end column of the last
// expression, needed to build the enclosing module's line table.
func compileFuncBodyCore(filename string, cls classification) (innerCode *bytecode.CodeObject, bodyEndLine int, bodyEndCol byte, err error) {
	outerMod, e := compileFuncBodyExpr(filename, cls)
	if e != nil {
		return nil, 0, 0, e
	}
	inner := outerMod.Consts[0].(*bytecode.CodeObject)
	// Decode bodyEndLine and bodyEndCol from the outer module's line table.
	// Layout: 5-byte prologue + LONG-5CU header + lineDelta + endLineDelta + startCol+1 + endCol+1.
	lt := outerMod.LineTable
	pos := 6 // skip prologue (5) + header byte (1)
	// skip lineDelta (signed varint)
	for lt[pos]&0x40 != 0 {
		pos++
	}
	pos++
	// read endLineDelta (unsigned varint)
	endLineDelta := 0
	for shift := 0; ; shift += 6 {
		b := lt[pos]
		pos++
		endLineDelta |= int(b&0x3f) << shift
		if b < 0x40 {
			break
		}
	}
	// skip startCol+1 (always 1, one byte)
	pos++
	// read endCol+1 (unsigned varint, always fits in one byte for col < 64)
	endCol1 := int(lt[pos] & 0x3f)
	defLine := int(outerMod.FirstLineNo)
	return inner, defLine + endLineDelta, byte(endCol1 - 1), nil
}

// funcState accumulates bytecode and linetable for a function body.
type funcState struct {
	bc       []byte
	lt       []byte
	slots    map[string]byte
	srcLines [][]byte
	consts       []any // constants in first-occurrence order; nil = Python None
	intConstSeen bool  // true once the first integer constant has been recorded
	noneCheckFunc bool // true when function has an `is None`/`is not None` condition;
	                   // CPython does not store integers in co_consts in this case.
	names        []string // global names referenced by LOAD_GLOBAL

	defLine     int  // source line of the function definition (RESUME line)
	prevLine    int  // source line of the last emitted linetable entry
	stmtLine    int  // source line of the current statement
	firstOnLine bool // true until first instruction of current stmt is emitted

	maxDepth    int  // peak stack depth seen so far
	lastExprEnd byte // end column of the most recently walked expression
}

func newFuncState(defLine int, slots map[string]byte, srcLines [][]byte) *funcState {
	return &funcState{
		defLine:  defLine,
		prevLine: defLine,
		slots:    slots,
		srcLines: srcLines,
	}
}

// newStmt marks the start of a new body statement on the given source line.
func (fs *funcState) newStmt(line int) {
	fs.stmtLine = line
	fs.firstOnLine = true
}

// emit appends one linetable entry; uses appendFirstLineEntry for the
// first instruction of a new statement, appendSameLine thereafter.
func (fs *funcState) emit(cuCount int, sc, ec byte) {
	if fs.firstOnLine {
		delta := fs.stmtLine - fs.prevLine
		fs.lt = bytecode.GenExprFirstEntry(fs.lt, delta, cuCount, sc, ec)
		fs.prevLine = fs.stmtLine
		fs.firstOnLine = false
	} else {
		fs.lt = bytecode.GenExprSameLine(fs.lt, cuCount, sc, ec)
	}
}

// emitSame appends one same-line linetable entry (always uses appendSameLine).
func (fs *funcState) emitSame(cuCount int, sc, ec byte) {
	fs.lt = bytecode.GenExprSameLine(fs.lt, cuCount, sc, ec)
}

// walkExpr compiles expr recursively, returning (startCol, endCol, depth).
// It always emits the expression instructions into fs.bc and fs.lt.
func (fs *funcState) walkExpr(e parser2.Expr) (startCol, endCol byte, depth int) {
	switch n := e.(type) {
	case *parser2.Name:
		slot, isLocal := fs.slots[n.Id]
		sc := byte(n.P.Col)
		ec := sc + byte(len(n.Id))
		if isLocal {
			fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW), slot)
			fs.emit(1, sc, ec)
		} else {
			nameIdx := fs.nameIndex(n.Id)
			lgCache := int(bytecode.CacheSize[bytecode.LOAD_GLOBAL])
			fs.bc = append(fs.bc, byte(bytecode.LOAD_GLOBAL), byte(nameIdx<<1)) // bit 0 = 0: value, no NULL
			for range lgCache {
				fs.bc = append(fs.bc, 0, 0)
			}
			fs.emit(1+lgCache, sc, ec)
		}
		fs.trackDepth(1)
		fs.lastExprEnd = ec
		return sc, ec, 1

	case *parser2.Constant:
		sc, ec := fs.loadConst(n)
		fs.emit(1, sc, ec)
		fs.trackDepth(1)
		return sc, ec, 1

	case *parser2.Attribute:
		// a.attr: load object then LOAD_ATTR (no method bit).
		// LOAD_ATTR has 9 cache words (10 CUs); split into 8+2.
		obj := n.Value.(*parser2.Name) // validated by isFuncBodyExpr
		objStart := byte(obj.P.Col)
		objEnd := objStart + byte(len(obj.Id))
		methodEnd := objEnd + 1 + byte(len(n.Attr)) // +1 for '.'

		objSlot := fs.slots[obj.Id]
		fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW), objSlot)
		fs.emit(1, objStart, objEnd)
		fs.trackDepth(1)

		nameIdx := fs.nameIndex(n.Attr)
		laCache := int(bytecode.CacheSize[bytecode.LOAD_ATTR])
		fs.bc = append(fs.bc, byte(bytecode.LOAD_ATTR), byte(nameIdx<<1)) // no method bit
		for range laCache {
			fs.bc = append(fs.bc, 0, 0)
		}
		fs.emitSame(8, objStart, methodEnd)
		fs.emitSame(2, objStart, methodEnd)
		fs.lastExprEnd = methodEnd
		return objStart, methodEnd, 1

	case *parser2.Tuple:
		nElts := len(n.Elts)
		var firstStart, lastEnd byte
		loaded := 0

		// LFLBLFLB optimisation for first two Name elements (locals only).
		if nElts >= 2 {
			if ln, lok := n.Elts[0].(*parser2.Name); lok {
				if rn, rok := n.Elts[1].(*parser2.Name); rok {
					ls, lOK := fs.slots[ln.Id]
					rs, rOK := fs.slots[rn.Id]
					if lOK && rOK && ls <= 15 && rs <= 15 {
						lsc := byte(ln.P.Col)
						lec := lsc + byte(len(ln.Id))
						rec := byte(rn.P.Col) + byte(len(rn.Id))
						fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
						fs.emit(1, lsc, lec)
						fs.trackDepth(2)
						firstStart = lsc
						lastEnd = rec
						loaded = 2
					}
				}
			}
		}
		// Load remaining elements.
		for i := loaded; i < nElts; i++ {
			sc, ec, _ := fs.walkExpr(n.Elts[i])
			if i == 0 {
				firstStart = sc
			}
			lastEnd = ec
			fs.trackDepth(i + 1)
		}
		// For parenthesised tuples the parser sets n.P to the '(' position,
		// which is one column before the first element. Extend the span to
		// include both '(' and the matching ')'.
		if tupleStart := byte(n.P.Col); tupleStart < firstStart {
			firstStart = tupleStart
		}
		lastEnd = fs.scanEndCol(lastEnd)
		// BUILD_TUPLE: span from first element (or '(') to last element (or ')').
		fs.bc = append(fs.bc, byte(bytecode.BUILD_TUPLE), byte(nElts))
		fs.emitSame(1, firstStart, lastEnd)
		fs.trackDepth(nElts)
		fs.lastExprEnd = lastEnd
		return firstStart, lastEnd, 1

	case *parser2.Subscript:
		// a[b] compiles to BINARY_OP NbGetItem, same pattern as BinOp.
		cacheWords := int(bytecode.CacheSize[bytecode.BINARY_OP])
		if ln, lok := n.Value.(*parser2.Name); lok {
			if rn, rok := n.Slice.(*parser2.Name); rok {
				ls, lOK := fs.slots[ln.Id]
				rs, rOK := fs.slots[rn.Id]
				if lOK && rOK && ls <= 15 && rs <= 15 {
					lsc := byte(ln.P.Col)
					lec := lsc + byte(len(ln.Id))
					rec := byte(rn.P.Col) + byte(len(rn.Id))
					closeEnd := fs.scanSubscriptEnd(rec)
					fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
					fs.emit(1, lsc, lec)
					fs.emitBinOp(bytecode.NbGetItem, cacheWords, lsc, closeEnd)
					fs.trackDepth(2)
					fs.lastExprEnd = closeEnd
					return lsc, closeEnd, 2
				}
			}
		}
		lsc, _, ld := fs.walkExpr(n.Value)
		_, rec, rd := fs.walkExpr(n.Slice)
		closeEnd := fs.scanSubscriptEnd(rec)
		fs.emitBinOp(bytecode.NbGetItem, cacheWords, lsc, closeEnd)
		d := max(ld, rd+1)
		fs.trackDepth(d)
		fs.lastExprEnd = closeEnd
		return lsc, closeEnd, d

	case *parser2.BinOp:
		// LFLBLFLB optimisation: left is a local Name and right's first load is also local.
		if ln, lok := n.Left.(*parser2.Name); lok {
			ls, lOK := fs.slots[ln.Id]
			if lOK && ls <= 15 {
				if rs, rOK := fs.peekFirstLocalSlot(n.Right); rOK {
					lsc := byte(ln.P.Col)
					lec := lsc + byte(len(ln.Id))
					fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
					fs.emit(1, lsc, lec)
					boparg, _ := binOpargFromOp(n.Op)
					cacheWords := int(bytecode.CacheSize[bytecode.BINARY_OP])
					_, rec, dSkip := fs.walkExprSkipFirstLocal(n.Right)
					fs.emitBinOp(boparg, cacheWords, lsc, rec)
					d := 2 + dSkip
					fs.trackDepth(d)
					fs.lastExprEnd = rec
					return lsc, rec, d
				}
			}
		}
		// General case: walk left then right.
		lsc, _, ld := fs.walkExpr(n.Left)
		_, rec, rd := fs.walkExpr(n.Right)
		if _, isBinOp := n.Right.(*parser2.BinOp); isBinOp {
			rec = fs.scanEndCol(rec)
		}
		boparg, _ := binOpargFromOp(n.Op)
		cacheWords := int(bytecode.CacheSize[bytecode.BINARY_OP])
		fs.emitBinOp(boparg, cacheWords, lsc, rec)
		d := max(ld, rd+1)
		fs.trackDepth(d)
		fs.lastExprEnd = rec
		return lsc, rec, d

	case *parser2.UnaryOp:
		opc := byte(n.P.Col)
		switch n.Op {
		case "USub":
			_, oec, od := fs.walkExpr(n.Operand)
			fs.bc = append(fs.bc, byte(bytecode.UNARY_NEGATIVE), 0)
			fs.emitSame(1, opc, oec)
			fs.lastExprEnd = oec
			return opc, oec, od
		case "Invert":
			_, oec, od := fs.walkExpr(n.Operand)
			fs.bc = append(fs.bc, byte(bytecode.UNARY_INVERT), 0)
			fs.emitSame(1, opc, oec)
			fs.lastExprEnd = oec
			return opc, oec, od
		case "Not":
			// Special case: not (compare) → COMPARE_OP(oparg+16) + UNARY_NOT,
			// no TO_BOOL. This matches CPython's conditional-flag optimisation.
			cmpCache := int(bytecode.CacheSize[bytecode.COMPARE_OP])
			if cmpExpr, isCmp := n.Operand.(*parser2.Compare); isCmp &&
				len(cmpExpr.Ops) == 1 && len(cmpExpr.Comparators) == 1 {
				_, oparg, _ := cmpOpFromOp(cmpExpr.Ops[0])
				oparg += 16 // conditional context flag
				leftExpr := cmpExpr.Left
				rightExpr := cmpExpr.Comparators[0]
				if ln, lok := leftExpr.(*parser2.Name); lok {
					if rn, rok := rightExpr.(*parser2.Name); rok {
						ls, lOK := fs.slots[ln.Id]
						rs, rOK := fs.slots[rn.Id]
						if lOK && rOK && ls <= 15 && rs <= 15 {
							lsc := byte(ln.P.Col)
							lec := lsc + byte(len(ln.Id))
							rec := byte(rn.P.Col) + byte(len(rn.Id))
							closedRec := fs.scanEndCol(rec)
							fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
							fs.emit(1, lsc, lec)
							fs.bc = append(fs.bc, byte(bytecode.COMPARE_OP), oparg)
							for range cmpCache {
								fs.bc = append(fs.bc, 0, 0)
							}
							fs.bc = append(fs.bc, byte(bytecode.UNARY_NOT), 0)
							fs.emitSame(1+cmpCache+1, opc, closedRec)
							fs.trackDepth(2)
							fs.lastExprEnd = closedRec
							return opc, closedRec, 1
						}
					}
				}
				// General not-compare (non-LFLBLFLB).
				_, _, ld := fs.walkExpr(leftExpr)
				_, rec, rd := fs.walkExpr(rightExpr)
				closedRec := fs.scanEndCol(rec)
				fs.bc = append(fs.bc, byte(bytecode.COMPARE_OP), oparg)
				for range cmpCache {
					fs.bc = append(fs.bc, 0, 0)
				}
				fs.bc = append(fs.bc, byte(bytecode.UNARY_NOT), 0)
				fs.emitSame(1+cmpCache+1, opc, closedRec)
				d := max(ld, rd+1)
				fs.trackDepth(d)
				fs.lastExprEnd = closedRec
				return opc, closedRec, 1
			}
			// General not: TO_BOOL + caches + UNARY_NOT.
			_, operandEnd, _ := fs.walkExpr(n.Operand)
			closedEnd := fs.scanEndCol(operandEnd)
			toBoolCaches := int(bytecode.CacheSize[bytecode.TO_BOOL])
			fs.bc = append(fs.bc, byte(bytecode.TO_BOOL), 0)
			for range toBoolCaches {
				fs.bc = append(fs.bc, 0, 0)
			}
			fs.bc = append(fs.bc, byte(bytecode.UNARY_NOT), 0)
			fs.emitSame(1+toBoolCaches+1, opc, closedEnd)
			fs.lastExprEnd = closedEnd
			return opc, closedEnd, 1
		}

	case *parser2.Compare:
		// Single-comparison only (validated by isFuncBodyExpr).
		leftExpr := n.Left
		rightExpr := n.Comparators[0]
		_, oparg, _ := cmpOpFromOp(n.Ops[0])
		cacheWords := int(bytecode.CacheSize[bytecode.COMPARE_OP])

		// LFLBLFLB optimisation: both operands are direct Name references.
		if ln, lok := leftExpr.(*parser2.Name); lok {
			if rn, rok := rightExpr.(*parser2.Name); rok {
				ls, lOK := fs.slots[ln.Id]
				rs, rOK := fs.slots[rn.Id]
				if lOK && rOK && ls <= 15 && rs <= 15 {
					lsc := byte(ln.P.Col)
					lec := lsc + byte(len(ln.Id))
					rec := byte(rn.P.Col) + byte(len(rn.Id))
					fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
					fs.emit(1, lsc, lec)
					fs.bc = append(fs.bc, byte(bytecode.COMPARE_OP), oparg)
					for range cacheWords {
						fs.bc = append(fs.bc, 0, 0)
					}
					fs.emitSame(1+cacheWords, lsc, rec)
					fs.trackDepth(2)
					fs.lastExprEnd = rec
					return lsc, rec, 2
				}
			}
		}
		// General case: walk left then right.
		lsc, _, ld := fs.walkExpr(leftExpr)
		_, rec, rd := fs.walkExpr(rightExpr)
		fs.bc = append(fs.bc, byte(bytecode.COMPARE_OP), oparg)
		for range cacheWords {
			fs.bc = append(fs.bc, 0, 0)
		}
		fs.emitSame(1+cacheWords, lsc, rec)
		d := max(ld, rd+1)
		fs.trackDepth(d)
		fs.lastExprEnd = rec
		return lsc, rec, d

	case *parser2.Call:
		// Determine call kind: global function or method (attribute call).
		var callStart, callFuncEnd byte
		nArgs := len(n.Args)

		if attr, isAttr := n.Func.(*parser2.Attribute); isAttr {
			// Method call: obj.method(args...)
			obj := attr.Value.(*parser2.Name) // validated by isFuncBodyExpr
			objStart := byte(obj.P.Col)
			objEnd := objStart + byte(len(obj.Id))
			methodEnd := objEnd + 1 + byte(len(attr.Attr)) // +1 for '.'

			// Load object.
			objSlot := fs.slots[obj.Id]
			fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW), objSlot)
			fs.emit(1, objStart, objEnd)
			fs.trackDepth(1)

			// LOAD_ATTR with method-call bit; 9 cache words = 10 CUs total.
			// Split linetable entry: 8 CUs + 2 CUs (max SHORT entry = 8).
			nameIdx := fs.nameIndex(attr.Attr)
			laCache := int(bytecode.CacheSize[bytecode.LOAD_ATTR])
			fs.bc = append(fs.bc, byte(bytecode.LOAD_ATTR), byte((nameIdx<<1)|1))
			for range laCache {
				fs.bc = append(fs.bc, 0, 0)
			}
			fs.emitSame(8, objStart, methodEnd)
			fs.emitSame(2, objStart, methodEnd)
			// LOAD_ATTR -1 obj + 2 (NULL + method) = net +1 on stack; peak = 2
			fs.trackDepth(2)

			callStart = objStart
			callFuncEnd = methodEnd
		} else {
			// Global function call: fn(args...)
			fn := n.Func.(*parser2.Name) // validated by isFuncBodyExpr
			callStart = byte(fn.P.Col)
			callFuncEnd = callStart + byte(len(fn.Id))

			nameIdx := fs.nameIndex(fn.Id)
			lgCache := int(bytecode.CacheSize[bytecode.LOAD_GLOBAL])
			fs.bc = append(fs.bc, byte(bytecode.LOAD_GLOBAL), byte((nameIdx<<1)|1))
			for range lgCache {
				fs.bc = append(fs.bc, 0, 0)
			}
			fs.emit(1+lgCache, callStart, callFuncEnd)
			fs.trackDepth(2) // NULL + func
		}

		// Emit args. Use LFLBLFLB when both are Name nodes with slots 0-15.
		// Track peak depth with ambient = 2 (NULL + func/method already on stack).
		lastArgEnd := callFuncEnd
		lflblflb := false
		if nArgs == 2 {
			if ln, lok := n.Args[0].(*parser2.Name); lok {
				if rn, rok := n.Args[1].(*parser2.Name); rok {
					ls, lOK := fs.slots[ln.Id]
					rs, rOK := fs.slots[rn.Id]
					if lOK && rOK && ls <= 15 && rs <= 15 {
						lsc := byte(ln.P.Col)
						lec := lsc + byte(len(ln.Id))
						rec := byte(rn.P.Col) + byte(len(rn.Id))
						fs.bc = append(fs.bc, byte(bytecode.LOAD_FAST_BORROW_LOAD_FAST_BORROW), (ls<<4)|rs)
						fs.emitSame(1, lsc, lec)
						lastArgEnd = rec
						lflblflb = true
						fs.trackDepth(2 + 2)
					}
				}
			}
		}
		if !lflblflb {
			for i, arg := range n.Args {
				savedMax := fs.maxDepth
				fs.maxDepth = 0
				_, argEnd, _ := fs.walkExpr(arg)
				argPeak := fs.maxDepth
				fs.maxDepth = savedMax
				fs.trackDepth(2 + i + argPeak)
				lastArgEnd = argEnd
			}
			fs.trackDepth(2 + nArgs)
		}
		closeEnd := fs.scanCallEnd(lastArgEnd)
		callCache := int(bytecode.CacheSize[bytecode.CALL])
		fs.bc = append(fs.bc, byte(bytecode.CALL), byte(nArgs))
		for range callCache {
			fs.bc = append(fs.bc, 0, 0)
		}
		fs.emitSame(1+callCache, callStart, closeEnd)
		fs.lastExprEnd = closeEnd
		return callStart, closeEnd, 1
	}
	return 0, 0, 1
}

// scanEndCol scans the current statement's source line from col forward,
// skipping whitespace. If the next character is ')', returns col+1; otherwise
// returns col. Used to extend linetable spans past the closing ')' in
// expressions like `not (a < b)`.
func (fs *funcState) scanEndCol(col byte) byte {
	if int(fs.stmtLine) < 1 || int(fs.stmtLine) > len(fs.srcLines) {
		return col
	}
	line := fs.srcLines[fs.stmtLine-1]
	c := int(col)
	for c < len(line) && (line[c] == ' ' || line[c] == '\t') {
		c++
	}
	if c < len(line) && line[c] == ')' {
		return byte(c + 1)
	}
	return col
}

// nameIndex returns the co_names index for name, adding it if not present.
func (fs *funcState) nameIndex(name string) byte {
	for i, n := range fs.names {
		if n == name {
			return byte(i)
		}
	}
	idx := byte(len(fs.names))
	fs.names = append(fs.names, name)
	return idx
}

// buildNames returns the co_names slice.
func (fs *funcState) buildNames() []string {
	if len(fs.names) == 0 {
		return []string{}
	}
	return fs.names
}

// scanChar scans the source line forward from startCol looking for ch.
// Returns col+1 past the first occurrence, or startCol if not found.
func (fs *funcState) scanChar(startCol byte, ch byte) byte {
	if int(fs.stmtLine) < 1 || int(fs.stmtLine) > len(fs.srcLines) {
		return startCol
	}
	line := fs.srcLines[fs.stmtLine-1]
	for c := int(startCol); c < len(line); c++ {
		if line[c] == ch {
			return byte(c + 1)
		}
	}
	return startCol
}

// scanCallEnd finds the closing ')' of a call starting from startCol.
func (fs *funcState) scanCallEnd(startCol byte) byte { return fs.scanChar(startCol, ')') }

// scanBackOpen checks whether the character immediately before col on the
// current statement's source line is '('. If so, returns col-1; otherwise
// returns col unchanged. Used to include the opening paren in BINARY_OP
// spans when the left sub-expression was parenthesized in source.
func (fs *funcState) scanBackOpen(col byte) byte {
	if col == 0 || int(fs.stmtLine) < 1 || int(fs.stmtLine) > len(fs.srcLines) {
		return col
	}
	line := fs.srcLines[fs.stmtLine-1]
	if int(col) <= len(line) && line[col-1] == '(' {
		return col - 1
	}
	return col
}

// scanTokenEnd scans forward from startCol on the current statement's source
// line, consuming numeric literal characters (digits, '.', '_', 'e', 'E',
// '+', '-'). Returns the column past the last consumed character.
// Used to find the end column of a float literal in the source.
func (fs *funcState) scanTokenEnd(startCol byte) byte {
	if int(fs.stmtLine) < 1 || int(fs.stmtLine) > len(fs.srcLines) {
		return startCol
	}
	line := fs.srcLines[fs.stmtLine-1]
	c := int(startCol)
	for c < len(line) {
		ch := line[c]
		if (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' ||
			ch == 'e' || ch == 'E' || ch == '+' || ch == '-' {
			c++
		} else {
			break
		}
	}
	return byte(c)
}

// exprEndCol returns the end column of expression e by inspecting the AST.
// Used to pre-compute ternEnd before emitting bytecode for ternary returns.
func (fs *funcState) exprEndCol(e parser2.Expr) byte {
	switch n := e.(type) {
	case *parser2.Name:
		return byte(n.P.Col) + byte(len(n.Id))
	case *parser2.Constant:
		switch n.Kind {
		case "int":
			return byte(n.P.Col) + byte(len(strconv.Itoa(int(n.Value.(int64)))))
		case "None":
			return byte(n.P.Col) + 4
		case "True":
			return byte(n.P.Col) + 4
		case "False":
			return byte(n.P.Col) + 5
		case "float":
			return fs.scanTokenEnd(byte(n.P.Col))
		case "str":
			return fs.scanStringEnd(byte(n.P.Col))
		}
	case *parser2.BinOp:
		return fs.exprEndCol(n.Right)
	case *parser2.UnaryOp:
		return fs.exprEndCol(n.Operand)
	case *parser2.Compare:
		return fs.exprEndCol(n.Comparators[0])
	case *parser2.Tuple:
		if len(n.Elts) > 0 {
			return fs.exprEndCol(n.Elts[len(n.Elts)-1])
		}
	case *parser2.Attribute:
		obj := n.Value.(*parser2.Name)
		return byte(obj.P.Col) + byte(len(obj.Id)) + 1 + byte(len(n.Attr))
	case *parser2.Subscript:
		return fs.scanSubscriptEnd(fs.exprEndCol(n.Slice))
	case *parser2.Call:
		var lastEnd byte
		if len(n.Args) > 0 {
			lastEnd = fs.exprEndCol(n.Args[len(n.Args)-1])
		} else {
			switch fn := n.Func.(type) {
			case *parser2.Name:
				lastEnd = byte(fn.P.Col) + byte(len(fn.Id))
			case *parser2.Attribute:
				obj := fn.Value.(*parser2.Name)
				lastEnd = byte(obj.P.Col) + byte(len(obj.Id)) + 1 + byte(len(fn.Attr))
			}
		}
		return fs.scanCallEnd(lastEnd)
	}
	return 0
}

// scanSubscriptEnd finds the closing ']' of a subscript starting from startCol.
func (fs *funcState) scanSubscriptEnd(startCol byte) byte { return fs.scanChar(startCol, ']') }

// scanStringEnd returns the source column past the closing quote of the string
// literal that begins at startCol on the current statement's source line.
// Handles single-quoted and double-quoted literals, including triple-quotes and
// backslash escape sequences. Falls back to end-of-line on any parse error.
func (fs *funcState) scanStringEnd(startCol byte) byte {
	if int(fs.stmtLine) < 1 || int(fs.stmtLine) > len(fs.srcLines) {
		return startCol
	}
	line := fs.srcLines[fs.stmtLine-1]
	c := int(startCol)
	if c >= len(line) {
		return startCol
	}
	q := line[c]
	if q != '"' && q != '\'' {
		return startCol
	}
	c++
	// Triple-quoted?
	if c+1 < len(line) && line[c] == q && line[c+1] == q {
		c += 2
		for c < len(line)-2 {
			if line[c] == q && line[c+1] == q && line[c+2] == q {
				return byte(c + 3)
			}
			if line[c] == '\\' {
				c++
			}
			c++
		}
		return byte(len(line))
	}
	// Single-quoted.
	for c < len(line) {
		if line[c] == '\\' {
			c += 2
			continue
		}
		if line[c] == q {
			return byte(c + 1)
		}
		c++
	}
	return byte(len(line))
}

// emitBinOp appends BINARY_OP + cache bytes and one linetable entry.
func (fs *funcState) emitBinOp(oparg byte, cacheWords int, sc, ec byte) {
	fs.bc = append(fs.bc, byte(bytecode.BINARY_OP), oparg)
	for range cacheWords {
		fs.bc = append(fs.bc, 0, 0)
	}
	fs.emitSame(1+cacheWords, sc, ec)
}

// trackDepth updates maxDepth if d > maxDepth.
func (fs *funcState) trackDepth(d int) {
	if d > fs.maxDepth {
		fs.maxDepth = d
	}
}

// peekFirstLocalSlot returns the local variable slot that would be loaded first
// when evaluating e, following the leftmost path. Returns (0, false) if e does
// not start with a local variable load (slot ≤ 15).
func (fs *funcState) peekFirstLocalSlot(e parser2.Expr) (byte, bool) {
	switch n := e.(type) {
	case *parser2.Name:
		s, ok := fs.slots[n.Id]
		if ok && s <= 15 {
			return s, true
		}
	case *parser2.BinOp:
		return fs.peekFirstLocalSlot(n.Left)
	}
	return 0, false
}

// walkExprSkipFirstLocal emits bytecode for expression e assuming its first
// LOAD_FAST_BORROW was already emitted by a preceding LFLBLFLB instruction.
// Returns (startCol, endCol, depth) where depth is the peak additional stack
// usage above the pre-loaded first-local value.
func (fs *funcState) walkExprSkipFirstLocal(e parser2.Expr) (startCol, endCol byte, depth int) {
	switch n := e.(type) {
	case *parser2.Name:
		sc := byte(n.P.Col)
		ec := sc + byte(len(n.Id))
		fs.lastExprEnd = ec
		return sc, ec, 0
	case *parser2.BinOp:
		lsc, _, ld := fs.walkExprSkipFirstLocal(n.Left)
		// Extend lsc backward to include the opening '(' when the left
		// sub-expression is itself a BinOp (i.e. was parenthesized in source).
		if _, isBinOp := n.Left.(*parser2.BinOp); isBinOp {
			lsc = fs.scanBackOpen(lsc)
		}
		_, rec, rd := fs.walkExpr(n.Right)
		// Extend rec forward to include the closing ')' when the right
		// sub-expression is a BinOp (i.e. was parenthesized in source).
		if _, isBinOp := n.Right.(*parser2.BinOp); isBinOp {
			rec = fs.scanEndCol(rec)
		}
		boparg, _ := binOpargFromOp(n.Op)
		cacheWords := int(bytecode.CacheSize[bytecode.BINARY_OP])
		fs.emitBinOp(boparg, cacheWords, lsc, rec)
		d := max(ld, rd)
		fs.trackDepth(d)
		fs.lastExprEnd = rec
		return lsc, rec, d
	}
	return 0, 0, 0
}

// constIndex returns the co_consts index for v, adding it if not already present.
func (fs *funcState) constIndex(v any) byte {
	for i, c := range fs.consts {
		if c == v {
			return byte(i)
		}
	}
	idx := byte(len(fs.consts))
	fs.consts = append(fs.consts, v)
	return idx
}

// buildConsts returns the co_consts slice: the accumulated constants, or
// (None,) if none were used.
func (fs *funcState) buildConsts() []any {
	if len(fs.consts) == 0 {
		return []any{nil}
	}
	return fs.consts
}

// loadConst emits the load instruction for a constant WITHOUT emitting a
// linetable entry. The caller is responsible for the entry. Returns (sc, ec).
//
// CPython stores only the first integer constant in co_consts; subsequent
// integers use LOAD_SMALL_INT without a co_consts entry. None/True/False are
// always stored in co_consts on first occurrence.
func (fs *funcState) loadConst(c *parser2.Constant) (sc, ec byte) {
	sc = byte(c.P.Col)
	switch c.Kind {
	case "int":
		iv := c.Value.(int64)
		ec = sc + byte(len(strconv.Itoa(int(iv))))
		if iv >= 0 && iv <= 255 {
			if !fs.intConstSeen && !fs.noneCheckFunc {
				fs.constIndex(iv)
				fs.intConstSeen = true
			}
			fs.bc = append(fs.bc, byte(bytecode.LOAD_SMALL_INT), byte(iv))
		} else {
			idx := fs.constIndex(iv)
			fs.bc = append(fs.bc, byte(bytecode.LOAD_CONST), idx)
		}
	case "None":
		idx := fs.constIndex(nil)
		fs.bc = append(fs.bc, byte(bytecode.LOAD_CONST), idx)
		ec = sc + 4
	case "True":
		idx := fs.constIndex(true)
		fs.bc = append(fs.bc, byte(bytecode.LOAD_CONST), idx)
		ec = sc + 4
	case "False":
		idx := fs.constIndex(false)
		fs.bc = append(fs.bc, byte(bytecode.LOAD_CONST), idx)
		ec = sc + 5
	case "float":
		fv := c.Value.(float64)
		idx := fs.constIndex(fv)
		fs.bc = append(fs.bc, byte(bytecode.LOAD_CONST), idx)
		ec = fs.scanTokenEnd(sc)
	case "str":
		sv := c.Value.(string)
		idx := fs.constIndex(sv)
		fs.bc = append(fs.bc, byte(bytecode.LOAD_CONST), idx)
		ec = fs.scanStringEnd(sc)
	}
	fs.lastExprEnd = ec
	return
}
