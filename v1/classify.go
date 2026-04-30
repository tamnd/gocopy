package compiler

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
)

type modKind uint8

const (
	modUnsupported modKind = iota
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
	modConstLitSeq         // [docstring +] 2+ constLitColl assignments in sequence
	modFrozenSetContains   // x = frozenset(name).__contains__
	modClcThenImports      // x = [list] + from-imports
	modAssignsThenFuncDef  // N foldedBinOp assigns + funcbody def
	modMultiFuncDef        // 2+ funcbody defs with no other statements
	modMixed               // [docstring?] [constLitColl?] [foldedBinOp assigns*] [funcBodyExprs+]
)

type classification struct {
	kind modKind
	// modAssign / modMultiAssign: the no-op tail, after
	// the leading assignment(s).
	stmts []bytecode.NoOpStmt
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
	// modFrozenSetContains fields:
	frozensetAsgn frozenSetContainsAssign
	// modClcThenImports fields:
	clcThenImportsAsgn clcThenImportsClassify
	// modAssignsThenFuncDef fields:
	assignsThenFuncDefAsgn assignsThenFuncDefClassify
	// modMultiFuncDef fields:
	multiFuncDefAsgns []funcBodyInfo
	// modMixed fields:
	mixedModuleAsgn mixedModuleClassify
}

// mixedModuleClassify holds the parsed form of a module body that combines
// an optional docstring, an optional constant-literal list (__all__), N >= 0
// folded BinOp assignments, and M >= 1 function-body definitions.
type mixedModuleClassify struct {
	hasDocstring bool
	docLine      int
	docEndLine   int
	docEndCol    byte
	docText      string

	hasStarImport    bool
	starImportModule string
	starImportLine   int
	starImportEndCol byte

	hasCLC      bool
	clc         constLitCollAssign

	assigns []asgn
	funcs   []mixedFunc
}

// mixedFunc is one function definition in a mixed module body.
// Either funcBody (stmtFuncBodyExpr) or funcDef (stmtFuncDef) is valid.
type mixedFunc struct {
	isFuncBodyExpr bool
	funcBody       funcBodyInfo
	funcDef        funcDefClassify
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
	// stmtFrozenSetContains fields:
	frozensetAsgn frozenSetContainsAssign
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
	stmtConstLitColl       // x = ["a","b"] or x = ("a","b") — all-string-constant collection
	stmtFrozenSetContains  // x = frozenset(name).__contains__
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
// docstring followed by zero or more constant-literal collection assignments
// and zero or more frozenset(name).__contains__ assignments.
type constLitSeqClassify struct {
	hasDocstring bool
	docLine      int    // 1-indexed start line of docstring
	docEndLine   int    // 1-indexed end line of docstring
	docEndCol    byte   // exclusive end column of docstring on docEndLine
	docText      string // the docstring value
	stmts        []constLitCollAssign
	frozensetStmts []frozenSetContainsAssign
}

// frozenSetContainsAssign holds the parsed form of `target = frozenset(arg).__contains__`.
type frozenSetContainsAssign struct {
	line         int
	target       string
	targetLen    byte
	argName      string
	argCol       byte
	argLen       byte
	frozensetCol byte
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

// clcThenImportsClassify holds the parsed form of a module body that starts
// with one constant-literal list assignment followed by one or more
// from-import/import statements.
type clcThenImportsClassify struct {
	clcAssign constLitCollAssign
	imports   []importEntry
}

// assignsThenFuncDefClassify holds the parsed form of a module body consisting
// of N ≥ 1 constant-folded BinOp assignments followed by one function definition.
type assignsThenFuncDefClassify struct {
	asgns    []asgn
	funcBody funcBodyInfo
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
	expr       ast.Expr
	srcLines   [][]byte
}

// funcBodyInfo holds the parsed form of a function definition whose body
// consists of zero or more local-assignment statements followed by one
// return statement. All expressions in the body are recursively composed
// of Name nodes referring to function parameters or previously assigned
// locals, plus BinOp and UnaryOp (USub/Invert) nodes.
type funcBodyInfo struct {
	funcName              string
	funcNameLen           byte
	defLine               int
	params                []fbParam   // positional parameters in declaration order
	defaults              []fbDefault // default values for the last len(defaults) params
	stmts                 []fbStmt    // body statements: all assignments then one return
	srcLines              [][]byte
	hasImplicitNoneReturn bool   // function body ends without an explicit return
	hasDocstring          bool   // first statement was a string literal (function docstring)
	docstring             string // docstring text when hasDocstring is true
}

// fbParam is one positional parameter in a funcBodyInfo.
type fbParam struct {
	name string
}

// fbDefault describes one Name-expression default for the last K parameters.
type fbDefault struct {
	name     string // the Name identifier to load via LOAD_NAME
	line     int    // source line of the default value
	colStart byte   // start column of the default value
	colEnd   byte   // end column (exclusive) of the default value
}

// fbStmt is one statement in a funcBodyInfo body.
type fbStmt struct {
	isReturn        bool   // true for the return statement, false for assignments
	isAugAssign     bool   // true for augmented assignments (target op= rhs)
	isBareCall      bool   // true for bare expression call (no assignment target)
	isIfReturn      bool   // true for `if cond: return expr` (early-return if)
	isIfAssign      bool   // true for `if cond: target = expr` (conditional assignment, no else)
	isIfElseAssign  bool   // true for `if cond: t=e [elif cond: t=e ...] else: t=e`
	augOp           byte   // NbInplace* oparg, only meaningful when isAugAssign
	line            int    // source line (1-indexed)
	thenLine        int    // line of the then-branch (for isIfReturn / isIfAssign)
	targetName      string // assignment target (for !isReturn; also then-body target for isIfAssign / isIfElseAssign)
	targetCol       byte   // column of the assignment target name
	retKwCol        byte   // column of the `return` keyword (for isReturn / then-return of isIfReturn)
	condExpr        ast.Expr   // condition expression (for isIfReturn / isIfAssign)
	expr            ast.Expr   // return value or assignment RHS
	ifElseBranches  []ifElseExprBranch // branches for isIfElseAssign
}

// ifElseExprBranch describes one arm of an if/elif/else assign chain
// where each body is an arbitrary expression (not a small-int constant).
// The else arm has condExpr == nil.
type ifElseExprBranch struct {
	condExpr ast.Expr // nil for the else branch
	condLine int          // source line of `if`/`elif` keyword
	bodyLine int          // source line of the assignment
	expr     ast.Expr // RHS of the assignment
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
		// v0.7.1: empty modules are handled by the visitor pipeline
		// before classifyAST is reached; this branch is unreachable.
		return classification{}, false
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
		allNoOp := true
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				allNoOp = false
				break
			}
		}
		if allNoOp {
			return classification{
				kind:         modFuncBodyExpr,
				funcBodyAsgn: first.funcBodyAsgn,
			}, true
		}
		// modMultiFuncDef: 2+ stmtFuncBodyExpr and nothing else
		allFuncBody := true
		for _, s := range stmts[1:] {
			if s.kind != stmtFuncBodyExpr {
				allFuncBody = false
				break
			}
		}
		if allFuncBody {
			funcs := make([]funcBodyInfo, len(stmts))
			for i, s := range stmts {
				funcs[i] = s.funcBodyAsgn
			}
			return classification{
				kind:              modMultiFuncDef,
				multiFuncDefAsgns: funcs,
			}, true
		}
		return classification{}, false
	}
	// modAssignsThenFuncDef: N >= 1 stmtAssign (all foldedBinOp) + stmtFuncBodyExpr
	if len(stmts) >= 2 && stmts[len(stmts)-1].kind == stmtFuncBodyExpr {
		lastIdx := len(stmts) - 1
		asgns := make([]asgn, lastIdx)
		allFolded := true
		for i := 0; i < lastIdx; i++ {
			s := stmts[i]
			if s.kind != stmtAssign {
				allFolded = false
				break
			}
			if _, ok := s.asgnValue.(foldedBinOp); !ok {
				allFolded = false
				break
			}
			asgns[i] = asgn{
				name:     s.text,
				nameLen:  s.asgnNameLen,
				valStart: s.asgnValStart,
				valEnd:   s.asgnValEnd,
				value:    s.asgnValue,
				line:     s.line,
			}
		}
		if allFolded {
			return classification{
				kind: modAssignsThenFuncDef,
				assignsThenFuncDefAsgn: assignsThenFuncDefClassify{
					asgns:    asgns,
					funcBody: stmts[lastIdx].funcBodyAsgn,
				},
			}, true
		}
	}
	if first := stmts[0]; first.kind == stmtConstLitColl {
		// Check for CLC + imports (modClcThenImports).
		if len(stmts) >= 2 {
			allImports := true
			for _, s := range stmts[1:] {
				if s.kind != stmtFromImport && s.kind != stmtImport {
					allImports = false
					break
				}
			}
			if allImports {
				imports := make([]importEntry, len(stmts)-1)
				for i, s := range stmts[1:] {
					imports[i] = s.importAsgn
				}
				return classification{
					kind: modClcThenImports,
					clcThenImportsAsgn: clcThenImportsClassify{
						clcAssign: first.constLitCollAsgn,
						imports:   imports,
					},
				}, true
			}
		}
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
	// modConstLitSeq: [docstring] + constLitColl* + frozenSetContains*
	// Eligible when (hasdoc || numCLC≥1) and total non-doc stmts ≥ 2 (or hasdoc && ≥1).
	{
		hasdoc := len(stmts) > 0 && stmts[0].kind == stmtString
		start := 0
		if hasdoc {
			start = 1
		}
		// Count constLitColl stmts.
		clcEnd := start
		for clcEnd < len(stmts) && stmts[clcEnd].kind == stmtConstLitColl {
			clcEnd++
		}
		// Count trailing frozenset stmts.
		fsEnd := clcEnd
		for fsEnd < len(stmts) && stmts[fsEnd].kind == stmtFrozenSetContains {
			fsEnd++
		}
		// Rest must be no-ops.
		tailOK := true
		for _, s := range stmts[fsEnd:] {
			if s.kind != stmtNoOp {
				tailOK = false
				break
			}
		}
		numCLC := clcEnd - start
		numFS := fsEnd - clcEnd
		total := numCLC + numFS
		if tailOK && (numCLC > 0 || hasdoc) && (total >= 2 || (hasdoc && total >= 1)) {
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
			seq.frozensetStmts = make([]frozenSetContainsAssign, numFS)
			for i := range numFS {
				seq.frozensetStmts[i] = stmts[clcEnd+i].frozensetAsgn
			}
			return classification{
				kind:            modConstLitSeq,
				constLitSeqAsgn: seq,
			}, true
		}
	}
	if first := stmts[0]; first.kind == stmtFrozenSetContains {
		for _, s := range stmts[1:] {
			if s.kind != stmtNoOp {
				return classification{}, false
			}
		}
		return classification{
			kind:          modFrozenSetContains,
			frozensetAsgn: first.frozensetAsgn,
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
	// modMixed: [docstring?] [constLitColl?] [foldedBinOp assigns*] [funcBodyExprs+] [noops*]
	// Requires at least one funcBodyExpr AND at least one of: docstring, CLC, 2+ funcs.
	// Must appear before the stmtString early-return so that a leading docstring followed
	// by non-no-op statements reaches this block rather than returning false.
	{
		idx := 0
		var m mixedModuleClassify

		// Optional leading docstring.
		if idx < len(stmts) && stmts[idx].kind == stmtString {
			m.hasDocstring = true
			m.docLine = stmts[idx].line
			m.docEndLine = stmts[idx].endLine
			m.docEndCol = stmts[idx].endCol
			m.docText = stmts[idx].text
			idx++
		}
		// Skip interspersed no-ops.
		for idx < len(stmts) && stmts[idx].kind == stmtNoOp {
			idx++
		}
		// Optional single star import (from X import *).
		if idx < len(stmts) && stmts[idx].kind == stmtFromImport {
			ie := stmts[idx].importAsgn
			if ie.IsFrom && ie.Level == 0 && len(ie.Aliases) == 1 && ie.Aliases[0].Name == "*" {
				m.hasStarImport = true
				m.starImportModule = ie.FromMod
				m.starImportLine = stmts[idx].line
				m.starImportEndCol = stmts[idx].endCol
				idx++
				for idx < len(stmts) && stmts[idx].kind == stmtNoOp {
					idx++
				}
			}
		}
		// Optional single constLitColl (e.g. __all__ = [...]).
		if idx < len(stmts) && stmts[idx].kind == stmtConstLitColl {
			m.hasCLC = true
			m.clc = stmts[idx].constLitCollAsgn
			idx++
		}
		// Skip no-ops.
		for idx < len(stmts) && stmts[idx].kind == stmtNoOp {
			idx++
		}
		// Zero or more constant assignments: foldedBinOp or small int (0–255).
		for idx < len(stmts) && stmts[idx].kind == stmtAssign {
			s := stmts[idx]
			switch v := s.asgnValue.(type) {
			case foldedBinOp:
				// accepted
			case int64:
				if v < 0 || v > 255 {
					goto doneAssigns
				}
			default:
				goto doneAssigns
			}
			m.assigns = append(m.assigns, asgn{
				name:     s.text,
				nameLen:  s.asgnNameLen,
				valStart: s.asgnValStart,
				valEnd:   s.asgnValEnd,
				value:    s.asgnValue,
				line:     s.line,
			})
			idx++
			for idx < len(stmts) && stmts[idx].kind == stmtNoOp {
				idx++
			}
		}
	doneAssigns:
		// One or more function definitions (stmtFuncBodyExpr or stmtFuncDef).
		for idx < len(stmts) {
			s := stmts[idx]
			if s.kind == stmtFuncBodyExpr {
				m.funcs = append(m.funcs, mixedFunc{isFuncBodyExpr: true, funcBody: s.funcBodyAsgn})
			} else if s.kind == stmtFuncDef {
				m.funcs = append(m.funcs, mixedFunc{isFuncBodyExpr: false, funcDef: s.funcDefAsgn})
			} else {
				break
			}
			idx++
			for idx < len(stmts) && stmts[idx].kind == stmtNoOp {
				idx++
			}
		}
		// Remaining must be no-ops.
		ok := true
		for ; idx < len(stmts); idx++ {
			if stmts[idx].kind != stmtNoOp {
				ok = false
				break
			}
		}
		if ok && len(m.funcs) >= 1 && (m.hasDocstring || m.hasCLC || len(m.funcs) >= 2) {
			return classification{kind: modMixed, mixedModuleAsgn: m}, true
		}
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

	return classification{}, false
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
