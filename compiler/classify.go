package compiler

import (
	"github.com/tamnd/gocopy/v1/bytecode"
	parser2 "github.com/tamnd/gopapy/parser"
)

type modKind uint8

const (
	modUnsupported modKind = iota
	modEmpty
	modNoOps
	modDocstring
	modAssign
	modMultiAssign
	modChainedAssign
	modAugAssign
	modBinOpAssign   // x = a op b  (both operands are names, arithmetic/bitwise)
	modUnaryAssign   // x = -a / ~a / not a  (operand is a name)
	modCmpAssign     // x = a cmpop b  (both operands are names, comparison)
	modBoolOp        // x = a and b  /  x = a or b  (both operands are names)
	modTernary       // x = a if c else b  (all operands are names)
	modCollection    // x = [...] / (...) / {...} collection literal with name elements
	modSubscriptLoad  // x = a[b]  (object and key are names)
	modSubscriptStore // a[b] = x  (object, key and value are names)
	modAttrLoad       // x = a.b  (object is a name)
	modAttrStore      // a.b = x  (object and value are names)
	modCallAssign     // x = f(args...) (f and all args are names, no kwargs)
	modIfElse         // if cond: var=val [elif ...] [else: var=val]
	modWhile          // while cond: var=val  (single-assignment body, no break/continue)
	modFor            // for loopVar in iter: bodyVar=val  (single-assignment body, no break)
	modFuncDef        // def f(arg): return arg  (single-arg, single return-arg body)
	modClosureDef     // def f(x): def g(): return x; return g  (simple one-free-var closure)
	modGenExpr        // x = <general expression> (recursive Name/Constant/BinOp/UnaryOp)
)

type classification struct {
	kind modKind
	// modNoOps: every statement, in source order.
	// modDocstring / modAssign / modMultiAssign: the no-op tail, after
	// the leading docstring or assignment(s).
	stmts []bytecode.NoOpStmt
	// modDocstring fields:
	docLine    int
	docEndLine int
	docCol     byte
	docText    string
	// modAssign fields:
	asgnLine     int
	asgnName     string
	asgnNameLen  byte
	asgnValStart byte
	asgnValEnd   byte
	asgnValue    any
	// modMultiAssign fields:
	asgns []asgn
	// modChainedAssign fields:
	chainLine     int
	chainTargets  []chainedTarget
	chainValStart byte
	chainValEnd   byte
	chainValue    any
	// modAugAssign fields:
	augLine     int
	augName     string
	augNameLen  byte
	augValStart byte
	augValEnd   byte
	augValue    any
	augOparg    byte
	// modBinOpAssign fields:
	binAsgn binOpAssign
	// modUnaryAssign fields:
	unaryAsgn unaryAssign
	// modCmpAssign fields:
	cmpAsgn cmpAssign
	// modBoolOp fields:
	boolAsgn boolAssign
	// modTernary fields:
	ternaryAsgn ternaryAssign
	// modCollection fields:
	collAsgn collectionAssign
	// modSubscriptLoad / modSubscriptStore fields:
	subAsgn subscriptAssign
	// modAttrLoad / modAttrStore fields:
	attrAsgn attrAssign
	// modCallAssign fields:
	callAsgn callAssign
	// modIfElse fields:
	ifElseAsgn ifElseClassify
	// modWhile fields:
	whileAsgn whileAssign
	// modFor fields:
	forAsgn forAssign
	// modFuncDef fields:
	funcDefAsgn funcDefClassify
	// modClosureDef fields:
	closureAsgn closureDef
	// modGenExpr fields:
	genExprAsgn genExprInfo
}

// rawStmt is the intermediate form produced by classifyAST before
// the shape-detection pass. A no-op token, a string literal whose
// contents are captured in text, or a name = literal assignment with
// its name/value fields populated. For a single-line statement
// endLine == line.
type rawStmt struct {
	line    int
	endLine int
	endCol  byte
	kind    rawStmtKind
	text    string
	// stmtAssign / stmtAugAssign fields:
	asgnNameLen  byte
	asgnValStart byte
	asgnValEnd   byte
	asgnValue    any
	// stmtChainedAssign fields:
	chainTargets []chainedTarget
	// stmtAugAssign fields:
	augOparg byte
	// stmtBinOpAssign fields:
	binAsgn binOpAssign
	// stmtUnaryAssign fields:
	unaryAsgn unaryAssign
	// stmtCmpAssign fields:
	cmpAsgn cmpAssign
	// stmtBoolOp fields:
	boolAsgn boolAssign
	// stmtTernary fields:
	ternaryAsgn ternaryAssign
	// stmtCollection fields:
	collAsgn collectionAssign
	// stmtSubscriptLoad / stmtSubscriptStore fields:
	subAsgn subscriptAssign
	// stmtAttrLoad / stmtAttrStore fields:
	attrAsgn attrAssign
	// stmtCallAssign fields:
	callAsgn callAssign
	// stmtIfElse fields:
	ifElseAsgn ifElseClassify
	// stmtWhile fields:
	whileAsgn whileAssign
	// stmtFor fields:
	forAsgn forAssign
	// stmtFuncDef fields:
	funcDefAsgn funcDefClassify
	// stmtClosureDef fields:
	closureAsgn closureDef
	// stmtGenExpr fields:
	genExprAsgn genExprInfo
}

type rawStmtKind uint8

const (
	stmtNoOp rawStmtKind = iota
	stmtString
	stmtAssign
	stmtChainedAssign
	stmtAugAssign
	stmtBinOpAssign  // x = a op b  (both operands are names, arithmetic/bitwise)
	stmtUnaryAssign  // x = -a / ~a / not a  (operand is a name)
	stmtCmpAssign    // x = a cmpop b  (both operands are names, comparison)
	stmtBoolOp       // x = a and/or b  (both operands are names)
	stmtTernary      // x = a if c else b  (all operands are names)
	stmtCollection   // x = [...] / (...) / {...} collection literal
	stmtSubscriptLoad  // x = a[b]
	stmtSubscriptStore // a[b] = x
	stmtAttrLoad       // x = a.b
	stmtAttrStore      // a.b = x
	stmtCallAssign     // x = f(args...)
	stmtIfElse         // if cond: var=val [elif ...] [else: var=val]
	stmtWhile          // while cond: var=val  (single-assignment body)
	stmtFor            // for loopVar in iter: bodyVar=val  (single-assignment body)
	stmtFuncDef        // def f(arg): return arg  (single-arg, single return-arg body)
	stmtClosureDef     // def f(x): def g(): return x; return g  (simple closure)
	stmtGenExpr        // x = <general expression>
)

// asgn is the parsed form of a single `name = literal` assignment.
type asgn struct {
	name     string
	nameLen  byte
	valStart byte
	valEnd   byte
	value    any
	line     int
}

// chainedTarget is one assignment target in `t0 = t1 = ... = literal`.
type chainedTarget struct {
	name      string
	nameStart byte
	nameLen   byte
}

// cmpAssign holds the parsed form of `target = left cmpop right`
// where both operands are names and cmpop is a comparison operator.
type cmpAssign struct {
	line      int
	target    string
	targetLen byte
	leftName  string
	leftCol   byte
	leftLen   byte
	rightName string
	rightCol  byte
	rightLen  byte
	op        bytecode.Opcode // COMPARE_OP, IS_OP, or CONTAINS_OP
	oparg     byte
}

// negLiteral is the value type for `name = -literal` assignments.
// CPython keeps both the un-negated literal and the negated result in
// the consts tuple.
type negLiteral struct {
	pos any
	neg any
}

// binOpAssign holds the parsed form of `target = left op right`
// where both operands are names.
type binOpAssign struct {
	line      int
	target    string
	targetLen byte
	leftName  string
	leftCol   byte
	leftLen   byte
	rightName string
	rightCol  byte
	rightLen  byte
	oparg     byte // NB_* constant for BINARY_OP
}

// unaryKind identifies which unary operator a modUnaryAssign uses.
type unaryKind uint8

const (
	unaryNeg    unaryKind = iota // UNARY_NEGATIVE (-x)
	unaryInvert                  // UNARY_INVERT (~x)
	unaryNot                     // UNARY_NOT (not x; emits TO_BOOL + UNARY_NOT)
)

// unaryAssign holds the parsed form of `target = -operand`, `target = ~operand`,
// or `target = not operand` where the operand is a name.
type unaryAssign struct {
	line        int
	target      string
	targetLen   byte
	operand     string
	operandCol  byte
	operandLen  byte
	opCol       byte      // column of the unary operator token (-, ~, or 'n' of not)
	kind        unaryKind
}

// collectionAssign holds the parsed form of a collection literal assignment:
//   x = [e0, e1, ...]  (list)
//   x = (e0, e1, ...)  (tuple)
//   x = {e0, e1, ...}  (set)
//   x = {k0: v0, ...}  (dict — elts alternates key/value)
//
// An empty elts slice means an empty collection ([], (), {}).
type collectionAssign struct {
	line      int
	target    string
	targetLen byte
	openCol   byte // column of the opening bracket
	closeEnd  byte // exclusive end col of the closing bracket (= lineEndCol)
	kind      bytecode.CollKind
	elts      []bytecode.CollElt
}

// boolAssign holds the parsed form of `target = left and/or right`
// where both operands are names.
type boolAssign struct {
	line      int
	target    string
	targetLen byte
	leftName  string
	leftCol   byte
	leftLen   byte
	rightName string
	rightCol  byte
	rightLen  byte
	isOr      bool // true = or, false = and
}

// ternaryAssign holds the parsed form of `target = trueVal if cond else falseVal`
// where all three operands are names.
type ternaryAssign struct {
	line      int
	target    string
	targetLen byte
	condName  string
	condCol   byte
	condLen   byte
	trueName  string
	trueCol   byte
	trueLen   byte
	falseName string
	falseCol  byte
	falseLen  byte
}

// subscriptAssign holds the parsed form of `x = a[b]` (isLoad=true) or
// `a[b] = x` (isLoad=false), where object, key and value are all names.
type subscriptAssign struct {
	line     int
	isLoad   bool
	// For load (x = a[b]): targetName/targetLen is the LHS.
	// For store (a[b] = x): valName/valCol/valEnd is the RHS.
	targetName string
	targetLen  byte
	valName    string
	valCol     byte
	valEnd     byte
	objName    string
	objCol     byte
	objEnd     byte // = objCol + len(objName)
	keyName    string
	keyCol     byte
	keyEnd     byte  // = keyCol + len(keyName)
	closeEnd   byte  // col after ']' = keyEnd + 1
}

// attrAssign holds the parsed form of `x = a.b` (isLoad=true) or
// `a.b = x` (isLoad=false), where object and value are names.
type attrAssign struct {
	line     int
	isLoad   bool
	// For load (x = a.b): targetName/targetLen is the LHS.
	// For store (a.b = x): valName/valCol/valEnd is the RHS.
	targetName string
	targetLen  byte
	valName    string
	valCol     byte
	valEnd     byte
	objName    string
	objCol     byte
	objEnd     byte // = objCol + len(objName)
	attrName   string
	attrEnd    byte // col after attr = objCol + objLen + 1 + len(attrName)
}

// callAssign holds the parsed form of `x = f(args...)` where f and all
// positional arguments are names and there are no keyword arguments.
type callAssign struct {
	line       int
	funcName   string
	funcCol    byte
	funcEnd    byte // = funcCol + len(funcName)
	args       []bytecode.CallArg
	targetName string
	targetLen  byte
	closeEnd   byte // col after `)` (= lineEndCol)
}

// ifElseBranch holds one condition+body pair in an if/elif chain.
type ifElseBranch struct {
	condName string
	condLine int
	condCol  byte
	condEnd  byte
	bodyLine int
	bodyVal  byte
	varName  string
	varCol   byte
	varEnd   byte
	valCol   byte
	valEnd   byte
}

// ifElseClassify holds the parsed form of an if/elif/else chain where
// each branch body is a single `name = small_int` assignment.
type ifElseClassify struct {
	branches    []ifElseBranch
	hasElse     bool
	elseLine    int
	elseVal     byte
	elseVarName string
	elseVarCol  byte
	elseVarEnd  byte
	elseValCol  byte
	elseValEnd  byte
}

// forAssign holds the parsed form of `for loopVar in iter: bodyVar = val`
// where iter and loopVar are names and val is a small integer (0-255).
type forAssign struct {
	iterName    string
	iterCol     byte
	iterEnd     byte
	loopVarName string
	loopVarCol  byte
	loopVarEnd  byte
	forLine     int
	bodyLine    int
	bodyVal     byte
	bodyVarName string
	bodyVarCol  byte
	bodyVarEnd  byte
	valCol      byte
	valEnd      byte
}

// funcDefClassify holds the parsed form of `def f(arg): return arg` where
// f and arg are single identifiers.
type funcDefClassify struct {
	funcName  string
	argName   string
	defLine   int
	bodyLine  int
	retKwCol  byte // column of `return` keyword
	argCol    byte // column of arg in return expression
	argEnd    byte // exclusive end column of arg (= end of return expression)
}

// closureDef holds the parsed form of `def f(x): def g(): return x; return g`.
type closureDef struct {
	outerFuncName string // e.g. "f"
	argName       string // e.g. "x" — the captured free variable
	innerFuncName string // e.g. "g"

	outerDefLine int // line of `def f(x):`
	innerDefLine int // line of `def g():` (= g.firstlineno)
	innerRetLine int // line of `return x` in g (innerBodyEnd for LONG entry)
	outerRetLine int // line of `return g` in f

	innerDefCol     byte // column of `def` keyword in `def g():`
	innerBodyEndCol byte // exclusive end column of last token in g's body

	innerFreeArgCol byte // column of x in `return x`
	innerFreeArgEnd byte // exclusive end of x in `return x`
	innerRetKwCol   byte // column of `return` keyword in g's `return x`

	outerRetArgCol byte // column of `g` in `return g`
	outerRetArgEnd byte // exclusive end of `g` in `return g`
	outerRetKwCol  byte // column of `return` keyword in f's `return g`
}

// genExprInfo holds the parsed form of a general expression assignment
// x = <expr> where <expr> is recursively composed of Name, small-int
// Constant, BinOp, and UnaryOp (USub/Invert) nodes.
type genExprInfo struct {
	targetName string
	targetLen  byte
	line       int
	lineEndCol byte
	expr       parser2.Expr
	srcLines   [][]byte
}

// whileAssign holds the parsed form of `while cond: name = val` where
// cond is a name and val is a small integer (0-255).
type whileAssign struct {
	condName string
	condLine int
	condCol  byte
	condEnd  byte
	bodyLine int
	bodyVal  byte
	varName  string
	varCol   byte
	varEnd   byte
	valCol   byte
	valEnd   byte
}

// stmtsToClassification converts the intermediate rawStmt list produced
// by classifyAST into a classification. It validates that the list
// matches one of the supported body shapes and that tail statements
// after header constructs are all no-ops.
func stmtsToClassification(stmts []rawStmt) (classification, bool) {
	if len(stmts) == 0 {
		return classification{kind: modEmpty}, true
	}
	for _, kind := range []rawStmtKind{stmtSubscriptLoad, stmtSubscriptStore, stmtAttrLoad, stmtAttrStore, stmtCallAssign} {
		if stmts[0].kind != kind {
			continue
		}
		first := stmts[0]
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		var mod modKind
		switch kind {
		case stmtSubscriptLoad:
			mod = modSubscriptLoad
		case stmtSubscriptStore:
			mod = modSubscriptStore
		case stmtAttrLoad:
			mod = modAttrLoad
		case stmtAttrStore:
			mod = modAttrStore
		default:
			mod = modCallAssign
		}
		return classification{
			kind:     mod,
			stmts:    tail,
			subAsgn:  first.subAsgn,
			attrAsgn: first.attrAsgn,
			callAsgn: first.callAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtIfElse {
		// if/elif/else: no tail statements allowed (the if block IS the module body)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:       modIfElse,
			ifElseAsgn: first.ifElseAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtWhile {
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:      modWhile,
			whileAsgn: first.whileAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtFor {
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:    modFor,
			forAsgn: first.forAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtFuncDef {
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:        modFuncDef,
			funcDefAsgn: first.funcDefAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtClosureDef {
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:        modClosureDef,
			closureAsgn: first.closureAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtGenExpr {
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:        modGenExpr,
			genExprAsgn: first.genExprAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtBoolOp {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:     modBoolOp,
			stmts:    tail,
			boolAsgn: first.boolAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtTernary {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:        modTernary,
			stmts:       tail,
			ternaryAsgn: first.ternaryAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtCollection {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:     modCollection,
			stmts:    tail,
			collAsgn: first.collAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtCmpAssign {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:    modCmpAssign,
			stmts:   tail,
			cmpAsgn: first.cmpAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtBinOpAssign {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:    modBinOpAssign,
			stmts:   tail,
			binAsgn: first.binAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtUnaryAssign {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:      modUnaryAssign,
			stmts:     tail,
			unaryAsgn: first.unaryAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtChainedAssign {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:          modChainedAssign,
			stmts:         tail,
			chainLine:     first.line,
			chainTargets:  first.chainTargets,
			chainValStart: first.asgnValStart,
			chainValEnd:   first.asgnValEnd,
			chainValue:    first.asgnValue,
		}, true
	}
	if first := stmts[0]; first.kind == stmtString {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:       modDocstring,
			stmts:      tail,
			docLine:    first.line,
			docEndLine: first.endLine,
			docCol:     first.endCol,
			docText:    first.text,
		}, true
	}
	if first := stmts[0]; first.kind == stmtAssign {
		numAsgn := 0
		for _, s := range stmts {
			if s.kind != stmtAssign {
				break
			}
			numAsgn++
		}
		if numAsgn == 1 && len(stmts) >= 2 && stmts[1].kind == stmtAugAssign {
			aug := stmts[1]
			tail := make([]bytecode.NoOpStmt, 0, len(stmts)-2)
			for _, s := range stmts[2:] {
				if s.kind != stmtNoOp {
					return classification{}, false
				}
				tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
			}
			return classification{
				kind:         modAugAssign,
				stmts:        tail,
				asgnLine:     first.line,
				asgnName:     first.text,
				asgnNameLen:  first.asgnNameLen,
				asgnValStart: first.asgnValStart,
				asgnValEnd:   first.asgnValEnd,
				asgnValue:    first.asgnValue,
				augLine:      aug.line,
				augName:      aug.text,
				augNameLen:   aug.asgnNameLen,
				augValStart:  aug.asgnValStart,
				augValEnd:    aug.asgnValEnd,
				augValue:     aug.asgnValue,
				augOparg:     aug.augOparg,
			}, true
		}
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-numAsgn)
		for _, s := range stmts[numAsgn:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		if numAsgn == 1 {
			return classification{
				kind:         modAssign,
				stmts:        tail,
				asgnLine:     first.line,
				asgnName:     first.text,
				asgnNameLen:  first.asgnNameLen,
				asgnValStart: first.asgnValStart,
				asgnValEnd:   first.asgnValEnd,
				asgnValue:    first.asgnValue,
			}, true
		}
		as := make([]asgn, numAsgn)
		for k, s := range stmts[:numAsgn] {
			as[k] = asgn{
				name:     s.text,
				nameLen:  s.asgnNameLen,
				valStart: s.asgnValStart,
				valEnd:   s.asgnValEnd,
				value:    s.asgnValue,
				line:     s.line,
			}
		}
		return classification{kind: modMultiAssign, stmts: tail, asgns: as}, true
	}
	out := make([]bytecode.NoOpStmt, 0, len(stmts))
	for _, s := range stmts {
		// stmtString is a no-op when it is not the first statement (the
		// docstring path is handled above). Reject anything else (assigns,
		// aug-assigns) in the no-op block.
		if s.kind != stmtNoOp && s.kind != stmtString {
			return classification{}, false
		}
		out = append(out, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
	}
	return classification{kind: modNoOps, stmts: out}, true
}

// splitLines splits src on '\n'. A trailing newline does NOT produce an
// empty trailing element; an absent trailing newline still yields the
// last line.
func splitLines(src []byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	var out [][]byte
	start := 0
	for i := range len(src) {
		if src[i] == '\n' {
			out = append(out, src[start:i])
			start = i + 1
		}
	}
	if start < len(src) {
		out = append(out, src[start:])
	}
	return out
}

// stripLineComment returns ln with any unquoted `#` and everything
// after it removed.
func stripLineComment(ln []byte) []byte {
	for i := range len(ln) {
		if ln[i] == '#' {
			return ln[:i]
		}
	}
	return ln
}

// trimRight removes trailing ASCII whitespace from b.
func trimRight(b []byte) []byte {
	n := len(b)
	for n > 0 {
		c := b[n-1]
		if c != ' ' && c != '\t' && c != '\r' && c != '\f' && c != '\v' {
			break
		}
		n--
	}
	return b[:n]
}

// isPlainAscii reports whether body is printable ASCII (0x20..0x7e)
// with no backslashes and no copies of the quote byte.
func isPlainAscii(body []byte, quote byte) bool {
	for _, c := range body {
		if c < 0x20 || c > 0x7e {
			return false
		}
		if c == '\\' || c == quote {
			return false
		}
	}
	return true
}
