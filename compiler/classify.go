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
	modFuncBodyExpr   // def f(args...): [assigns]* return expr  (general function body)
	modImports        // sequence of import / from-import statements
	modConstLitColl   // x = ["a","b","c"] or x = ("a","b","c") — all-string-constant collection
	modConstLitSeq    // [docstring +] 2+ constLitColl assignments in sequence
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
	// modFuncBodyExpr fields:
	funcBodyAsgn funcBodyInfo
	// modImports fields:
	imports []importEntry
	// modConstLitColl fields:
	constLitCollAsgn constLitCollAssign
	// modConstLitSeq fields:
	constLitSeqAsgn constLitSeqClassify
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
	// stmtFuncBodyExpr fields:
	funcBodyAsgn funcBodyInfo
	// stmtImport / stmtFromImport fields:
	importAsgn importEntry
	// stmtConstLitColl fields:
	constLitCollAsgn constLitCollAssign
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
	stmtFuncBodyExpr   // def f(args...): [assigns]* return expr
	stmtImport         // import X [as Y] (one alias)
	stmtFromImport     // from X import Y1 [as Z1], ...
	stmtConstLitColl   // x = ["a","b"] or x = ("a","b") — all-string-constant collection
)

// importAlias is one (name, asname) pair in an import statement.
type importAlias struct {
	Name   string
	Asname string // empty = same as Name
}

// importEntry is one import or from-import statement.
type importEntry struct {
	Line    int
	EndCol  byte
	IsFrom  bool
	// IsFrom=false: simple import
	Module  string
	Asname  string // empty = top-level component of Module
	// IsFrom=true: from-import
	FromMod string
	Level   int
	Aliases []importAlias
}

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

// constLitSeqClassify describes a multi-statement module body: an optional
// docstring followed by one or more constant-literal collection assignments.
type constLitSeqClassify struct {
	hasDocstring bool
	docLine      int    // 1-indexed start line of docstring
	docEndLine   int    // 1-indexed end line of docstring
	docEndCol    byte   // exclusive end column of docstring on docEndLine
	docText      string // the docstring value
	stmts        []constLitCollAssign
}

// constLitElt is one element in a constant-literal collection assignment.
// Only string elements are supported in v0.4.2; val is always a string.
type constLitElt struct {
	val    string
	line   int  // 1-indexed source line (same as assignment line for n<31 single-line)
	col    byte // 0-indexed column of the opening quote
	endCol byte // exclusive end col (after closing quote)
}

// constLitCollAssign holds the parsed form of a constant-literal collection
// assignment: `x = ["a", "b", "c"]` (isList=true) or `x = ("a", "b", "c")` (false).
type constLitCollAssign struct {
	line      int
	target    string
	targetLen byte
	openCol   byte // column of '[' or '('
	closeEnd  byte // exclusive end col of closing bracket (']' or ')')
	closeLine int  // source line of the closing bracket (= line for single-line; > line for multi-line)
	isList    bool
	elts      []constLitElt
}

// foldedBinOp is the value type for `name = const op const` assignments
// where both operands are numeric constants. CPython folds these at compile
// time. leftVal is the left operand (stored at co_consts[0] for the first
// assignment in a sequence); result is the folded value.
type foldedBinOp struct {
	leftVal any // int64 or float64
	result  any // int64 or float64
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

// funcBodyInfo holds the parsed form of a function definition whose body
// consists of zero or more local-assignment statements followed by one
// return statement. All expressions in the body are recursively composed
// of Name nodes referring to function parameters or previously assigned
// locals, plus BinOp and UnaryOp (USub/Invert) nodes.
type funcBodyInfo struct {
	funcName    string
	funcNameLen byte
	defLine     int
	params      []fbParam  // positional parameters in declaration order
	stmts       []fbStmt   // body statements: all assignments then one return
	srcLines    [][]byte
}

// fbParam is one positional parameter in a funcBodyInfo.
type fbParam struct {
	name string
}

// fbStmt is one statement in a funcBodyInfo body.
type fbStmt struct {
	isReturn    bool   // true for the return statement, false for assignments
	isAugAssign bool   // true for augmented assignments (target op= rhs)
	isIfReturn  bool   // true for `if cond: return expr` (early-return if)
	isIfAssign  bool   // true for `if cond: target = expr` (conditional assignment, no else)
	augOp       byte   // NbInplace* oparg, only meaningful when isAugAssign
	line        int    // source line (1-indexed)
	thenLine    int    // line of the then-branch (for isIfReturn / isIfAssign)
	targetName  string // assignment target (for !isReturn; also then-body target for isIfAssign)
	targetCol   byte   // column of the assignment target name
	retKwCol    byte   // column of the `return` keyword (for isReturn / then-return of isIfReturn)
	condExpr    parser2.Expr // condition expression (for isIfReturn / isIfAssign)
	expr        parser2.Expr // return value or assignment RHS
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
	if first := stmts[0]; first.kind == stmtFuncBodyExpr {
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:         modFuncBodyExpr,
			funcBodyAsgn: first.funcBodyAsgn,
		}, true
	}
	if first := stmts[0]; first.kind == stmtConstLitColl {
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:             modConstLitColl,
			constLitCollAsgn: first.constLitCollAsgn,
		}, true
	}
	// modConstLitSeq: [docstring] + (≥1 constLitColl with docstring, or ≥2 without)
	{
		hasdoc := len(stmts) > 0 && stmts[0].kind == stmtString
		start := 0
		if hasdoc {
			start = 1
		}
		allCLC := start < len(stmts)
		for i := start; i < len(stmts); i++ {
			if stmts[i].kind != stmtConstLitColl {
				allCLC = false
				break
			}
		}
		numCLC := len(stmts) - start
		if allCLC && (numCLC >= 2 || (hasdoc && numCLC >= 1)) {
			seq := constLitSeqClassify{hasDocstring: hasdoc}
			if hasdoc {
				seq.docLine = stmts[0].line
				seq.docEndLine = stmts[0].endLine
				seq.docEndCol = stmts[0].endCol
				seq.docText = stmts[0].text
			}
			seq.stmts = make([]constLitCollAssign, numCLC)
			for i := range numCLC {
				seq.stmts[i] = stmts[start+i].constLitCollAsgn
			}
			return classification{
				kind:           modConstLitSeq,
				constLitSeqAsgn: seq,
			}, true
		}
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
	// Check if all non-no-op stmts are imports.
	{
		hasImport := false
		for _, s := range stmts {
			if s.kind == stmtImport || s.kind == stmtFromImport {
				hasImport = true
				break
			}
		}
		if hasImport {
			var imports []importEntry
			for _, s := range stmts {
				switch s.kind {
				case stmtNoOp, stmtString:
					// no-op: only contributes to line numbering
				case stmtImport, stmtFromImport:
					imports = append(imports, s.importAsgn)
				default:
					goto notImports
				}
			}
			return classification{kind: modImports, imports: imports}, true
		notImports:
		}
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
