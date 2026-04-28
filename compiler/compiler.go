// Package compiler lowers a Python source file to a bytecode.CodeObject.
// The parser is github.com/tamnd/gopapy (wired in v0.0.17). The
// supported body shapes are:
//
//  1. Empty module (file is empty or contains only whitespace, blank
//     lines, and comments).
//  2. N >= 1 no-op statements (pass, None/True/False/..., numeric and
//     bytes literals, non-leading string literals), each at column 0,
//     with arbitrary blank or comment-only lines anywhere.
//  3. A leading plain-ASCII string literal (the docstring), single-line
//     or triple-quoted across multiple lines, optionally followed by
//     N >= 0 no-op statements.
//  4. A leading `name = literal` assignment, optionally followed by
//     N >= 0 no-op statements.
//  5. N >= 2 consecutive `name = literal` assignments.
//  6. A chained assignment `t0 = t1 = ... = literal` (N >= 2 targets).
//  7. A `name = initVal` assignment followed by `name op= augVal`
//     (augmented assignment; integer initVal and augVal only).
//  8. A `target = left op right` assignment where both operands are
//     names and op is a binary arithmetic or bitwise operator.
//  9. A `target = -operand`, `target = ~operand`, or
//     `target = not operand` assignment where the operand is a name.
//
// Anything else returns ErrUnsupportedSource.
package compiler

import (
	"bytes"
	"errors"
	"strings"

	parser2 "github.com/tamnd/gopapy/parser"

	"github.com/tamnd/gocopy/v1/bytecode"
)

// ErrUnsupportedSource is returned for any module body the v0.0.x
// rungs have not yet learned to compile.
var ErrUnsupportedSource = errors.New("compiler: source not yet supported")

// Options configures the compiler. Filename ends up in CodeObject.Filename
// (a.k.a. CPython's co_filename).
type Options struct {
	Filename string
}

// Compile returns the CodeObject for the given Python source bytes.
func Compile(source []byte, opts Options) (*bytecode.CodeObject, error) {
	mod, parseErr := parser2.ParseFile(opts.Filename, string(source))
	if parseErr != nil {
		return nil, ErrUnsupportedSource
	}
	cls, ok := classifyAST(source, mod)
	if !ok {
		return nil, ErrUnsupportedSource
	}
	switch cls.kind {
	case modEmpty:
		return module(opts.Filename,
			bytecode.NoOpBytecode(1),
			bytecode.LineTableEmpty(),
			[]any{nil}, nil,
		), nil
	case modNoOps:
		return module(opts.Filename,
			bytecode.NoOpBytecode(len(cls.stmts)),
			bytecode.LineTableNoOps(cls.stmts),
			[]any{nil}, nil,
		), nil
	case modDocstring:
		return module(opts.Filename,
			bytecode.DocstringBytecode(len(cls.stmts)),
			bytecode.DocstringLineTable(cls.docLine, cls.docEndLine, cls.docCol, cls.stmts),
			[]any{cls.docText, nil},
			[]string{"__doc__"},
		), nil
	case modAssign:
		// Integer RHS: small ints (0..255) use LOAD_SMALL_INT; larger ints
		// use LOAD_CONST. Either way consts = (int_val, None).
		if iv, ok := cls.asgnValue.(int64); ok {
			lt := bytecode.AssignLineTable(cls.asgnLine, cls.asgnNameLen, cls.asgnValStart, cls.asgnValEnd, cls.stmts)
			if iv >= 0 && iv <= 255 {
				return module(opts.Filename,
					bytecode.AssignSmallIntBytecode(byte(iv), len(cls.stmts)),
					lt,
					[]any{iv, nil},
					[]string{cls.asgnName},
				), nil
			}
			return module(opts.Filename,
				bytecode.AssignBytecode(1, len(cls.stmts)),
				lt,
				[]any{iv, nil},
				[]string{cls.asgnName},
			), nil
		}
		// Constant-folded BinOp: consts = (leftVal, None[, result]).
		// CPython stores leftVal at [0], None at [1], and the folded result
		// at [2] when it doesn't fit LOAD_SMALL_INT.
		if fb, ok := cls.asgnValue.(foldedBinOp); ok {
			lt := bytecode.AssignLineTable(cls.asgnLine, cls.asgnNameLen, cls.asgnValStart, cls.asgnValEnd, cls.stmts)
			if iv, isInt := fb.result.(int64); isInt && iv >= 0 && iv <= 255 {
				return module(opts.Filename,
					bytecode.AssignSmallIntBytecode(byte(iv), len(cls.stmts)),
					lt,
					[]any{fb.leftVal, nil},
					[]string{cls.asgnName},
				), nil
			}
			return module(opts.Filename,
				bytecode.AssignBytecodeAt(2, 1, len(cls.stmts)),
				lt,
				[]any{fb.leftVal, nil, fb.result},
				[]string{cls.asgnName},
			), nil
		}
		// Negative literal: consts = (pos, None, neg), LOAD_CONST 2.
		// CPython's constant folder keeps the original positive literal at
		// index 0, None at index 1, and the folded negative at index 2.
		if nl, ok := cls.asgnValue.(negLiteral); ok {
			lt := bytecode.AssignLineTable(cls.asgnLine, cls.asgnNameLen, cls.asgnValStart, cls.asgnValEnd, cls.stmts)
			return module(opts.Filename,
				bytecode.AssignBytecodeAt(2, 1, len(cls.stmts)),
				lt,
				[]any{nl.pos, nil, nl.neg},
				[]string{cls.asgnName},
			), nil
		}
		consts := []any{cls.asgnValue, nil}
		noneIdx := byte(1)
		if cls.asgnValue == nil {
			consts = []any{nil}
			noneIdx = 0
		}
		return module(opts.Filename,
			bytecode.AssignBytecode(noneIdx, len(cls.stmts)),
			bytecode.AssignLineTable(cls.asgnLine, cls.asgnNameLen, cls.asgnValStart, cls.asgnValEnd, cls.stmts),
			consts,
			[]string{cls.asgnName},
		), nil
	case modMultiAssign:
		return compileMultiAssign(opts.Filename, cls.asgns, cls.stmts)
	case modChainedAssign:
		return compileChainedAssign(opts.Filename, cls.chainLine, cls.chainTargets, cls.chainValStart, cls.chainValEnd, cls.chainValue, cls.stmts)
	case modAugAssign:
		return compileAugAssign(opts.Filename, cls)
	case modBinOpAssign:
		return compileBinOpAssign(opts.Filename, cls)
	case modUnaryAssign:
		return compileUnaryAssign(opts.Filename, cls)
	case modCmpAssign:
		return compileCmpAssign(opts.Filename, cls)
	case modBoolOp:
		return compileBoolOp(opts.Filename, cls)
	case modTernary:
		return compileTernary(opts.Filename, cls)
	case modCollection:
		return compileCollection(opts.Filename, cls)
	case modSubscriptLoad:
		return compileSubscriptLoad(opts.Filename, cls)
	case modSubscriptStore:
		return compileSubscriptStore(opts.Filename, cls)
	case modAttrLoad:
		return compileAttrLoad(opts.Filename, cls)
	case modAttrStore:
		return compileAttrStore(opts.Filename, cls)
	case modCallAssign:
		return compileCallAssign(opts.Filename, cls)
	case modIfElse:
		return compileIfElse(opts.Filename, cls)
	case modWhile:
		return compileWhile(opts.Filename, cls)
	case modFor:
		return compileFor(opts.Filename, cls)
	case modFuncDef:
		return compileFuncDef(opts.Filename, cls)
	case modClosureDef:
		return compileClosure(opts.Filename, cls)
	case modGenExpr:
		return compileGenExpr(opts.Filename, cls)
	case modFuncBodyExpr:
		return compileFuncBodyExpr(opts.Filename, cls)
	case modImports:
		return compileImports(opts.Filename, cls.imports)
	case modConstLitColl:
		return compileConstLitColl(opts.Filename, cls.constLitCollAsgn)
	}
	return nil, ErrUnsupportedSource
}

// compileImports lowers a module whose body is only import/from-import statements.
func compileImports(filename string, entries []importEntry) (*bytecode.CodeObject, error) {
	if len(entries) == 0 {
		return nil, ErrUnsupportedSource
	}

	// Build co_consts and co_names in first-seen order.
	// co_consts[0] = level (int64); then fromlist tuples; then None.
	// co_names: module names, store names, attribute names in encounter order.

	level := int64(entries[0].Level)
	consts := []any{level}
	noneIdx := byte(0)
	noneAdded := false

	addNone := func() byte {
		if !noneAdded {
			noneIdx = byte(len(consts))
			consts = append(consts, nil)
			noneAdded = true
		}
		return noneIdx
	}

	names := []string{}
	nameMap := map[string]byte{}
	addName := func(n string) byte {
		if i, ok := nameMap[n]; ok {
			return i
		}
		i := byte(len(names))
		names = append(names, n)
		nameMap[n] = i
		return i
	}

	// importLocalName returns the local binding name for a simple import entry.
	importLocalName := func(e importEntry) string {
		if e.Asname != "" {
			return e.Asname
		}
		if i := strings.Index(e.Module, "."); i >= 0 {
			return e.Module[:i]
		}
		return e.Module
	}

	type entryCompiled struct {
		constIdx byte
		ref      bytecode.ImportNameRef
	}
	compiled := make([]entryCompiled, len(entries))

	for i, e := range entries {
		var ec entryCompiled
		if !e.IsFrom {
			ec.constIdx = addNone()
			modIdx := addName(e.Module)
			localName := importLocalName(e)
			var storeIdx byte
			if localName == e.Module {
				storeIdx = modIdx
			} else {
				storeIdx = addName(localName)
			}
			ec.ref = bytecode.ImportNameRef{ModuleIdx: modIdx, StoreIdx: storeIdx}
		} else {
			// from-import: fromlist tuple (must be bytecode.ConstTuple)
			fromlist := make(bytecode.ConstTuple, len(e.Aliases))
			for j, a := range e.Aliases {
				fromlist[j] = a.Name
			}
			ec.constIdx = byte(len(consts))
			consts = append(consts, fromlist)

			modIdx := addName(e.FromMod)
			aliasRefs := make([]bytecode.AliasNameRef, len(e.Aliases))
			for j, a := range e.Aliases {
				ni := addName(a.Name)
				var si byte
				if a.Asname == "" {
					si = ni
				} else {
					si = addName(a.Asname)
				}
				aliasRefs[j] = bytecode.AliasNameRef{NameIdx: ni, StoreIdx: si}
			}
			ec.ref = bytecode.ImportNameRef{ModuleIdx: modIdx, AliasIdxs: aliasRefs}
		}
		compiled[i] = ec
	}

	finalNone := addNone()

	// Assemble bytecode and linetable entries.
	constIdxs := make([]byte, len(entries)+1)
	for i, ec := range compiled {
		constIdxs[i] = ec.constIdx
	}
	constIdxs[len(entries)] = finalNone

	refs := make([]bytecode.ImportNameRef, len(entries))
	for i, ec := range compiled {
		refs[i] = ec.ref
	}

	bcEntries := make([]bytecode.ImportEntry, len(entries))
	isLast := make([]bool, len(entries))
	for i, e := range entries {
		bcEntries[i] = bytecode.ImportEntry{
			Line:    e.Line,
			EndCol:  e.EndCol,
			IsFrom:  e.IsFrom,
			Module:  e.Module,
			Asname:  e.Asname,
			FromMod: e.FromMod,
			Level:   e.Level,
			Aliases: make([]bytecode.ImportAlias, len(e.Aliases)),
		}
		for j, a := range e.Aliases {
			bcEntries[i].Aliases[j] = bytecode.ImportAlias{Name: a.Name, Asname: a.Asname}
		}
		isLast[i] = (i == len(entries)-1)
	}

	bc := bytecode.ImportBytecode(bcEntries, constIdxs, refs)
	lt := bytecode.ImportLineTable(bcEntries, isLast)

	co := module(filename, bc, lt, consts, names)
	co.StackSize = 2 // LOAD_SMALL_INT + LOAD_CONST are both on the stack at IMPORT_NAME
	return co, nil
}

// compileAugAssign lowers `name = initVal\nname += augVal\n` at module scope.
// Only non-negative integer initVal and augVal are supported in v0.0.15.
func compileAugAssign(filename string, cls classification) (*bytecode.CodeObject, error) {
	initVal, ok := cls.asgnValue.(int64)
	if !ok {
		return nil, ErrUnsupportedSource
	}
	augVal, ok := cls.augValue.(int64)
	if !ok || augVal < 0 {
		return nil, ErrUnsupportedSource
	}

	// Build consts: initVal always at [0], augVal at [1] if large, None last.
	var consts []any
	augSmall := augVal >= 0 && augVal <= 255
	if augSmall {
		consts = []any{initVal, nil}
	} else {
		consts = []any{initVal, augVal, nil}
	}

	bc := bytecode.AugAssignBytecode(initVal, augVal, cls.augOparg, len(cls.stmts))
	lt := bytecode.AugAssignLineTable(
		cls.asgnLine, cls.asgnNameLen, cls.asgnValStart, cls.asgnValEnd,
		cls.augLine, cls.augValStart, cls.augValEnd,
		cls.stmts,
	)
	co := module(filename, bc, lt, consts, []string{cls.asgnName})
	co.StackSize = 2 // LOAD_NAME + LOAD augVal both on stack at BINARY_OP
	return co, nil
}

// compileCollection lowers a collection-literal assignment.
// For empty tuples CPython emits LOAD_CONST (); for all other empty collections
// it emits BUILD_LIST/MAP 0. For non-empty name-only collections it loads each
// name then emits BUILD_LIST/TUPLE/SET/MAP N.
func compileCollection(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.collAsgn
	if len(a.elts) == 0 {
		// Empty collection.
		bc := bytecode.CollectionEmptyBytecode(a.kind)
		lt := bytecode.CollectionEmptyLineTable(a.line, a.openCol, a.closeEnd, a.targetLen)
		var consts []any
		if a.kind == bytecode.CollTuple {
			consts = []any{nil, bytecode.ConstTuple{}} // [None, ()]
		} else {
			consts = []any{nil}
		}
		names := []string{a.target}
		return module(filename, bc, lt, consts, names), nil
	}
	// Non-empty: load each element by name, then BUILD_*.
	n := len(a.elts)
	bc := bytecode.CollectionNamesBytecode(a.kind, n)
	lt := bytecode.CollectionNamesLineTable(a.line, a.elts, a.openCol, a.closeEnd, a.targetLen)
	names := make([]string, 0, n+1)
	for _, e := range a.elts {
		names = append(names, e.Name)
	}
	names = append(names, a.target)
	co := module(filename, bc, lt, []any{nil}, names)
	if n > 1 {
		co.StackSize = int32(n)
	}
	return co, nil
}

// compileBoolOp lowers `target = left and/or right` where both operands are names.
func compileBoolOp(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.boolAsgn
	var bc []byte
	if a.isOr {
		bc = bytecode.BoolOrBytecode()
	} else {
		bc = bytecode.BoolAndBytecode()
	}
	lt := bytecode.BoolAndOrLineTable(a.line, a.leftCol, a.leftLen, a.rightCol, a.rightLen, a.targetLen)
	names := []string{a.leftName, a.rightName, a.target}
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = 2
	return co, nil
}

// compileTernary lowers `target = trueVal if cond else falseVal` where all operands are names.
func compileTernary(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.ternaryAsgn
	bc := bytecode.TernaryBytecode()
	lt := bytecode.TernaryLineTable(a.line, a.condCol, a.condLen, a.trueCol, a.trueLen, a.falseCol, a.falseLen, a.targetLen)
	names := []string{a.condName, a.trueName, a.falseName, a.target}
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = 1
	return co, nil
}

// compileCmpAssign lowers `target = left cmpop right` where both operands are names.
func compileCmpAssign(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.cmpAsgn
	bc := bytecode.CmpAssignBytecode(a.op, a.oparg)
	lt := bytecode.CmpAssignLineTable(a.op, a.line, a.leftCol, a.leftLen, a.rightCol, a.rightLen, a.targetLen)
	names := []string{a.leftName, a.rightName, a.target}
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = 2
	return co, nil
}

// compileBinOpAssign lowers `target = left op right` where both operands are names.
func compileBinOpAssign(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.binAsgn
	bc := bytecode.BinOpAssignBytecode(a.oparg)
	lt := bytecode.BinOpAssignLineTable(a.line, a.leftCol, a.leftLen, a.rightCol, a.rightLen, a.targetLen)
	// names: [left, right, target] — insertion order as encountered during compilation
	names := []string{a.leftName, a.rightName, a.target}
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = 2
	return co, nil
}

// compileUnaryAssign lowers `target = -operand`, `target = ~operand`, or
// `target = not operand` where the operand is a name.
func compileUnaryAssign(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.unaryAsgn
	names := []string{a.operand, a.target}
	var bc, lt []byte
	switch a.kind {
	case unaryNeg:
		bc = bytecode.UnaryNegInvertBytecode(bytecode.UNARY_NEGATIVE)
		lt = bytecode.UnaryNegInvertLineTable(a.line, a.opCol, a.operandCol, a.operandLen, a.targetLen)
	case unaryInvert:
		bc = bytecode.UnaryNegInvertBytecode(bytecode.UNARY_INVERT)
		lt = bytecode.UnaryNegInvertLineTable(a.line, a.opCol, a.operandCol, a.operandLen, a.targetLen)
	case unaryNot:
		bc = bytecode.UnaryNotBytecode()
		lt = bytecode.UnaryNotLineTable(a.line, a.opCol, a.operandCol, a.operandLen, a.targetLen)
	default:
		return nil, ErrUnsupportedSource
	}
	return module(filename, bc, lt, []any{nil}, names), nil
}

// compileChainedAssign lowers `t0 = t1 = ... = literal` (single line, N >= 2 targets).
func compileChainedAssign(filename string, line int, targets []chainedTarget, valStart, valEnd byte, value any, tail []bytecode.NoOpStmt) (*bytecode.CodeObject, error) {
	n := len(targets)

	// Build deduplicated names list preserving assignment order.
	namesIdx := map[string]byte{}
	names := []string{}
	nameIdxs := make([]byte, n)
	for i, t := range targets {
		if idx, ok := namesIdx[t.name]; ok {
			nameIdxs[i] = idx
		} else {
			idx = byte(len(names))
			namesIdx[t.name] = idx
			names = append(names, t.name)
			nameIdxs[i] = idx
		}
	}

	// Build consts and choose load instruction — same rules as single-assign.
	var consts []any
	var noneIdx byte
	var loadOp bytecode.Opcode
	var loadArg byte

	switch tv := value.(type) {
	case int64:
		if tv >= 0 && tv <= 255 {
			consts = []any{tv, nil}
			noneIdx = 1
			loadOp = bytecode.LOAD_SMALL_INT
			loadArg = byte(tv)
		} else {
			consts = []any{tv, nil}
			noneIdx = 1
			loadOp = bytecode.LOAD_CONST
			loadArg = 0
		}
	case negLiteral:
		consts = []any{tv.pos, nil, tv.neg}
		noneIdx = 1
		loadOp = bytecode.LOAD_CONST
		loadArg = 2
	case nil:
		consts = []any{nil}
		noneIdx = 0
		loadOp = bytecode.LOAD_CONST
		loadArg = 0
	default:
		consts = []any{value, nil}
		noneIdx = 1
		loadOp = bytecode.LOAD_CONST
		loadArg = 0
	}

	// Build bytecode: RESUME + LOAD + [COPY 1, STORE_NAME]×(n-1) + STORE_NAME + NOPs + LOAD_CONST None + RV.
	nops := 0
	if len(tail) > 1 {
		nops = len(tail) - 1
	}
	bc := make([]byte, 0, 2+2+4*(n-1)+2+2*nops+4)
	bc = append(bc, byte(bytecode.RESUME), 0)
	bc = append(bc, byte(loadOp), loadArg)
	for i := 0; i < n-1; i++ {
		bc = append(bc, byte(bytecode.COPY), 1)
		bc = append(bc, byte(bytecode.STORE_NAME), nameIdxs[i])
	}
	bc = append(bc, byte(bytecode.STORE_NAME), nameIdxs[n-1])
	for range nops {
		bc = append(bc, byte(bytecode.NOP), 0)
	}
	bc = append(bc, byte(bytecode.LOAD_CONST), noneIdx, byte(bytecode.RETURN_VALUE), 0)

	// Build line table.
	bTargets := make([]bytecode.ChainedTarget, n)
	for i, t := range targets {
		bTargets[i] = bytecode.ChainedTarget{
			NameStart: t.nameStart,
			NameLen:   t.nameLen,
		}
	}
	lt := bytecode.ChainedAssignLineTable(line, bTargets, valStart, valEnd, tail)

	co := module(filename, bc, lt, consts, names)
	co.StackSize = 2 // COPY pushes a second item; peak depth is 2
	return co, nil
}

// compileMultiAssign lowers N >= 2 consecutive `name = literal` assignments.
func compileMultiAssign(filename string, asgns []asgn, tail []bytecode.NoOpStmt) (*bytecode.CodeObject, error) {
	// Build names (deduplicated, insertion-ordered).
	namesIdx := map[string]byte{}
	names := []string{}
	nameIdxs := make([]byte, len(asgns))
	for i, a := range asgns {
		if idx, ok := namesIdx[a.name]; ok {
			nameIdxs[i] = idx
		} else {
			idx = byte(len(names))
			namesIdx[a.name] = idx
			names = append(names, a.name)
			nameIdxs[i] = idx
		}
	}

	// Build consts and per-assignment load info.
	// loadSmall[i] == true → LOAD_SMALL_INT asgns[i].value.(int64)
	// loadSmall[i] == false → LOAD_CONST loadIdx[i]
	consts := []any{}
	loadSmall := make([]bool, len(asgns))
	loadIdx := make([]byte, len(asgns))
	// Values that must go into co_consts AFTER None (negLiteral.neg and
	// foldedBinOp.result that don't fit LOAD_SMALL_INT).
	type deferredEntry struct {
		asgnIdx int
		val     any
	}
	var deferred []deferredEntry

	// constIdx returns the index of v in consts, or -1.
	constIdx := func(v any) int {
		if bv, ok := v.([]byte); ok {
			for j, c := range consts {
				if bc, ok2 := c.([]byte); ok2 && bytes.Equal(bv, bc) {
					return j
				}
			}
			return -1
		}
		for j, c := range consts {
			if c == v {
				return j
			}
		}
		return -1
	}
	addConst := func(v any) byte {
		if idx := constIdx(v); idx >= 0 {
			return byte(idx)
		}
		idx := byte(len(consts))
		consts = append(consts, v)
		return idx
	}

	// Check if any assignment uses constant folding; if so, all must.
	hasFolded := false
	for _, a := range asgns {
		if _, ok := a.value.(foldedBinOp); ok {
			hasFolded = true
			break
		}
	}
	if hasFolded {
		for _, a := range asgns {
			if _, ok := a.value.(foldedBinOp); !ok {
				return nil, ErrUnsupportedSource
			}
		}
	}

	// resultValues holds the actual value to emit for each assignment.
	resultValues := make([]any, len(asgns))

	for i, a := range asgns {
		v := a.value
		switch tv := v.(type) {
		case foldedBinOp:
			resultValues[i] = tv.result
			if i == 0 {
				addConst(tv.leftVal) // phantom slot at co_consts[0] for first assignment
			}
			if iv, isInt := tv.result.(int64); isInt && iv >= 0 && iv <= 255 {
				loadSmall[i] = true
			} else {
				// Defer until after None so result lands after None in co_consts.
				deferred = append(deferred, deferredEntry{i, tv.result})
				loadIdx[i] = 0 // will be overwritten
			}
		case int64:
			resultValues[i] = tv
			if tv >= 0 && tv <= 255 {
				loadSmall[i] = true
				if i == 0 {
					addConst(tv) // phantom slot
				}
			} else {
				loadIdx[i] = addConst(tv)
			}
		case negLiteral:
			resultValues[i] = tv.neg
			if i == 0 {
				addConst(tv.pos) // phantom slot
			}
			deferred = append(deferred, deferredEntry{i, tv.neg})
			loadIdx[i] = 0 // will be overwritten
		case nil:
			resultValues[i] = nil
			loadIdx[i] = addConst(nil)
		default:
			resultValues[i] = v
			loadIdx[i] = addConst(v)
		}
	}

	// Add None if not present; record noneIdx.
	noneIdx := addConst(nil)

	// Resolve deferred values (foldedBinOp results and negLiteral.neg),
	// adding them to co_consts after None in encounter order.
	deferredIdx := map[any]byte{}
	for _, d := range deferred {
		if _, ok := deferredIdx[d.val]; !ok {
			deferredIdx[d.val] = byte(len(consts))
			consts = append(consts, d.val)
		}
		loadIdx[d.asgnIdx] = deferredIdx[d.val]
	}

	// Build bytecode: RESUME + [LOAD, STORE_NAME]×N + NOPs + LOAD_CONST None + RV.
	nops := 0
	if len(tail) > 1 {
		nops = len(tail) - 1
	}
	bc := make([]byte, 0, 2+4*len(asgns)+2*nops+4)
	bc = append(bc, byte(bytecode.RESUME), 0)
	for i := range asgns {
		if loadSmall[i] {
			bc = append(bc, byte(bytecode.LOAD_SMALL_INT), byte(resultValues[i].(int64)))
		} else {
			bc = append(bc, byte(bytecode.LOAD_CONST), loadIdx[i])
		}
		bc = append(bc, byte(bytecode.STORE_NAME), nameIdxs[i])
	}
	for range nops {
		bc = append(bc, byte(bytecode.NOP), 0)
	}
	bc = append(bc, byte(bytecode.LOAD_CONST), noneIdx, byte(bytecode.RETURN_VALUE), 0)

	// Build line table.
	ltAsgns := make([]bytecode.AssignInfo, len(asgns))
	for i, a := range asgns {
		ltAsgns[i] = bytecode.AssignInfo{
			Line:     a.line,
			NameLen:  a.nameLen,
			ValStart: a.valStart,
			ValEnd:   a.valEnd,
		}
	}
	lt := bytecode.MultiAssignLineTable(ltAsgns, tail)

	return module(filename, bc, lt, consts, names), nil
}

// compileCallAssign lowers `x = f(args...)` where f and all positional args are names.
func compileCallAssign(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.callAsgn
	n := len(a.args)
	bc := bytecode.CallAssignBytecode(n)
	lt := bytecode.CallAssignLineTable(a.line, a.funcCol, a.funcEnd, a.args, a.closeEnd, a.targetLen)
	names := make([]string, 0, n+2)
	names = append(names, a.funcName)
	for _, arg := range a.args {
		names = append(names, arg.Name)
	}
	names = append(names, a.targetName)
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = int32(2 + n)
	return co, nil
}

// compileSubscriptLoad lowers `x = a[b]` where a and b are names.
func compileSubscriptLoad(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.subAsgn
	bc := bytecode.SubscriptLoadBytecode()
	lt := bytecode.SubscriptLoadLineTable(a.line, a.objCol, a.objEnd, a.keyCol, a.keyEnd, a.closeEnd, a.targetLen)
	names := []string{a.objName, a.keyName, a.targetName}
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = 2
	return co, nil
}

// compileSubscriptStore lowers `a[b] = x` where a, b and x are names.
func compileSubscriptStore(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.subAsgn
	bc := bytecode.SubscriptStoreBytecode()
	lt := bytecode.SubscriptStoreLineTable(a.line, a.valCol, a.valEnd, a.objCol, a.objEnd, a.keyCol, a.keyEnd, a.closeEnd)
	names := []string{a.valName, a.objName, a.keyName}
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = 3
	return co, nil
}

// compileAttrLoad lowers `x = a.b` where a is a name.
func compileAttrLoad(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.attrAsgn
	bc := bytecode.AttrLoadBytecode()
	lt := bytecode.AttrLoadLineTable(a.line, a.objCol, a.objEnd, a.attrEnd, a.targetLen)
	names := []string{a.objName, a.attrName, a.targetName}
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = 1
	return co, nil
}

// compileAttrStore lowers `a.b = x` where a and x are names.
func compileAttrStore(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.attrAsgn
	bc := bytecode.AttrStoreBytecode()
	lt := bytecode.AttrStoreLineTable(a.line, a.valCol, a.valEnd, a.objCol, a.objEnd, a.attrEnd)
	names := []string{a.valName, a.objName, a.attrName}
	co := module(filename, bc, lt, []any{nil}, names)
	co.StackSize = 2
	return co, nil
}

// compileIfElse lowers an if/elif/else chain where each branch body is `name = small_int`.
func compileIfElse(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.ifElseAsgn

	// Build co_names: for each branch, add cond name then var name (deduped in order).
	// Then add else var name (if hasElse).
	nameIdx := map[string]byte{}
	names := []string{}
	addName := func(s string) byte {
		if idx, ok := nameIdx[s]; ok {
			return idx
		}
		idx := byte(len(names))
		nameIdx[s] = idx
		names = append(names, s)
		return idx
	}

	bcs := make([]bytecode.IfBranch, len(a.branches))
	lts := make([]bytecode.IfBranchLT, len(a.branches))
	for i, br := range a.branches {
		condIdx := addName(br.condName)
		varIdx := addName(br.varName)
		bcs[i] = bytecode.IfBranch{CondIdx: condIdx, BodyVal: br.bodyVal, VarIdx: varIdx}
		lts[i] = bytecode.IfBranchLT{
			CondLine: br.condLine,
			CondCol:  br.condCol,
			CondEnd:  br.condEnd,
			BodyLine: br.bodyLine,
			ValCol:   br.valCol,
			ValEnd:   br.valEnd,
			VarCol:   br.varCol,
			VarEnd:   br.varEnd,
		}
	}
	elseVarIdx := byte(0)
	if a.hasElse {
		elseVarIdx = addName(a.elseVarName)
	}

	// co_consts: first branch value (phantom) + None.
	firstVal := int64(a.branches[0].bodyVal)
	consts := []any{firstVal, nil}
	noneIdx := byte(1)

	bc := bytecode.IfElseBytecode(bcs, a.hasElse, a.elseVal, elseVarIdx, noneIdx)
	lt := bytecode.IfElseLineTable(lts, a.hasElse, a.elseLine, a.elseValCol, a.elseValEnd, a.elseVarCol, a.elseVarEnd)
	co := module(filename, bc, lt, consts, names)
	co.StackSize = 1
	return co, nil
}

// compileFor lowers `for loopVar in iter: bodyVar = val` (single-assignment body, no break).
func compileFor(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.forAsgn
	// Build co_names: iterName first, then loopVarName, then bodyVarName (deduped).
	nameIdx := map[string]byte{}
	names := []string{}
	addName := func(s string) byte {
		if idx, ok := nameIdx[s]; ok {
			return idx
		}
		idx := byte(len(names))
		nameIdx[s] = idx
		names = append(names, s)
		return idx
	}
	iterIdx := addName(a.iterName)
	loopVarIdx := addName(a.loopVarName)
	bodyVarIdx := addName(a.bodyVarName)
	firstVal := int64(a.bodyVal)
	consts := []any{firstVal, nil}
	noneIdx := byte(1)
	bc := bytecode.ForAssignBytecode(iterIdx, loopVarIdx, a.bodyVal, bodyVarIdx, noneIdx)
	lt := bytecode.ForAssignLineTable(a.forLine, a.bodyLine, a.iterCol, a.iterEnd, a.loopVarCol, a.loopVarEnd, a.valCol, a.valEnd, a.bodyVarCol, a.bodyVarEnd)
	co := module(filename, bc, lt, consts, names)
	co.StackSize = 2
	return co, nil
}

// compileWhile lowers `while cond: name = val` (single-assignment body, no break/continue).
func compileWhile(filename string, cls classification) (*bytecode.CodeObject, error) {
	a := cls.whileAsgn
	condIdx := byte(0)
	varIdx := byte(1)
	names := []string{a.condName, a.varName}
	if a.condName == a.varName {
		varIdx = 0
		names = []string{a.condName}
	}
	firstVal := int64(a.bodyVal)
	consts := []any{firstVal, nil}
	noneIdx := byte(1)
	bc := bytecode.WhileAssignBytecode(condIdx, a.bodyVal, varIdx, noneIdx)
	lt := bytecode.WhileAssignLineTable(a.condLine, a.bodyLine, a.condCol, a.condEnd, a.valCol, a.valEnd, a.varCol, a.varEnd)
	co := module(filename, bc, lt, consts, names)
	co.StackSize = 1
	return co, nil
}

// module returns the canonical Code object for the given body. Only
// bytecode, line table, consts, and names vary across the v0.0.x
// shapes; everything else (locals, filename, qualname, exception
// table) is identical.
//
// Bytes verified against `python3.14 -m py_compile` for empty
// modules, the v0.0.4 N-statement no-op set, and the v0.0.5
// docstring shape.
func module(filename string, bc, lineTable []byte, consts []any, names []string) *bytecode.CodeObject {
	if names == nil {
		names = []string{}
	}
	return &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0,
		Bytecode:        bc,
		Consts:          consts,
		Names:           names,
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        filename,
		Name:            "<module>",
		QualName:        "<module>",
		FirstLineNo:     1,
		LineTable:       lineTable,
		ExcTable:        []byte{},
	}
}

// compileFuncDef lowers `def f(arg): return arg` at module scope where f and
// arg are single identifiers and the body is a single return-arg statement.
func compileFuncDef(filename string, cls classification) (*bytecode.CodeObject, error) {
	fd := cls.funcDefAsgn
	funcCode := &bytecode.CodeObject{
		ArgCount:        1,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0x3,
		Bytecode:        bytecode.FuncReturnArgBytecode(0),
		Consts:          []any{nil},
		Names:           []string{},
		LocalsPlusNames: []string{fd.argName},
		LocalsPlusKinds: []byte{0x26},
		Filename:        filename,
		Name:            fd.funcName,
		QualName:        fd.funcName,
		FirstLineNo:     int32(fd.defLine),
		LineTable:       bytecode.FuncReturnArgLineTable(fd.defLine, fd.bodyLine, fd.argCol, fd.argEnd, fd.retKwCol),
		ExcTable:        []byte{},
	}
	return &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0,
		Bytecode:        bytecode.FuncDefModuleBytecode(0),
		Consts:          []any{funcCode, nil},
		Names:           []string{fd.funcName},
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        filename,
		Name:            "<module>",
		QualName:        "<module>",
		FirstLineNo:     int32(fd.defLine),
		LineTable:       bytecode.FuncDefModuleLineTable(fd.defLine, fd.bodyLine, fd.argEnd),
		ExcTable:        []byte{},
	}, nil
}

// compileClosure lowers `def f(x): def g(): return x; return g` at module scope.
// f has one arg (x, promoted to cell), one inner function g (no args, returns x),
// and returns g. g captures x as a free variable.
func compileClosure(filename string, cls classification) (*bytecode.CodeObject, error) {
	cd := cls.closureAsgn

	// Inner function g: captures outerArg as a free variable.
	innerCode := &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0x13, // CO_OPTIMIZED | CO_NEWLOCALS | CO_NESTED
		Bytecode:        bytecode.ClosureInnerBytecode(),
		Consts:          []any{nil},
		Names:           []string{},
		LocalsPlusNames: []string{cd.argName},
		LocalsPlusKinds: []byte{0x80}, // CO_FAST_FREE
		Filename:        filename,
		Name:            cd.innerFuncName,
		QualName:        cd.outerFuncName + ".<locals>." + cd.innerFuncName,
		FirstLineNo:     int32(cd.innerDefLine),
		LineTable: bytecode.ClosureInnerLineTable(
			cd.innerDefLine, cd.innerRetLine,
			cd.innerFreeArgCol, cd.innerFreeArgEnd, cd.innerRetKwCol),
		ExcTable: []byte{},
	}

	// Outer function f: x is a cell+arg, g is a plain local.
	outerCode := &bytecode.CodeObject{
		ArgCount:        1,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       2,
		Flags:           0x03, // CO_OPTIMIZED | CO_NEWLOCALS
		Bytecode:        bytecode.ClosureOuterBytecode(),
		Consts:          []any{innerCode}, // no None; all paths return explicitly
		Names:           []string{},
		LocalsPlusNames: []string{cd.argName, cd.innerFuncName},
		LocalsPlusKinds: []byte{0x66, 0x20}, // arg+cell, local
		Filename:        filename,
		Name:            cd.outerFuncName,
		QualName:        cd.outerFuncName,
		FirstLineNo:     int32(cd.outerDefLine),
		LineTable: bytecode.ClosureOuterLineTable(
			cd.outerDefLine, cd.innerDefLine, cd.innerRetLine, cd.outerRetLine,
			cd.innerDefCol, cd.innerBodyEndCol,
			cd.outerRetArgCol, cd.outerRetArgEnd, cd.outerRetKwCol),
		ExcTable: []byte{},
	}

	// Module code object: loads outer code, makes function, stores name.
	return &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0,
		Bytecode:        bytecode.FuncDefModuleBytecode(0),
		Consts:          []any{outerCode, nil},
		Names:           []string{cd.outerFuncName},
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        filename,
		Name:            "<module>",
		QualName:        "<module>",
		FirstLineNo:     int32(cd.outerDefLine),
		LineTable: bytecode.FuncDefModuleLineTable(
			cd.outerDefLine, cd.outerRetLine, cd.outerRetArgEnd),
		ExcTable: []byte{},
	}, nil
}

// compileConstLitColl lowers a constant-literal collection assignment:
// `x = ["a","b","c"]` (list) or `x = ("a","b","c")` (tuple).
// Only string elements are supported in v0.4.2.
func compileConstLitColl(filename string, a constLitCollAssign) (*bytecode.CodeObject, error) {
	n := len(a.elts)
	if n == 0 {
		return nil, ErrUnsupportedSource // empty handled by modCollection
	}

	names := []string{a.target}

	if !a.isList {
		// Tuple of any size: LOAD_CONST full tuple.
		// co_consts: (first_elem, None, ConstTuple{all_elems})
		first := a.elts[0].val
		tup := make(bytecode.ConstTuple, n)
		for i, e := range a.elts {
			tup[i] = e.val
		}
		consts := []any{first, nil, tup}
		bc := bytecode.ConstLitTupleBytecode()
		lt := bytecode.ConstLitTupleLineTable(a.line, a.targetLen, a.openCol, a.closeEnd)
		return module(filename, bc, lt, consts, names), nil
	}

	// List.
	switch n {
	case 1:
		// co_consts: (elem, None)
		consts := []any{a.elts[0].val, nil}
		bc := bytecode.ConstLitList1Bytecode()
		lt := bytecode.ConstLitList1LineTable(a.line, a.targetLen, a.openCol, a.closeEnd,
			a.elts[0].col, a.elts[0].endCol)
		return module(filename, bc, lt, consts, names), nil
	case 2:
		// co_consts: (elem0, elem1, None)
		consts := []any{a.elts[0].val, a.elts[1].val, nil}
		bc := bytecode.ConstLitList2Bytecode()
		lt := bytecode.ConstLitList2LineTable(a.line, a.targetLen, a.openCol, a.closeEnd,
			a.elts[0].col, a.elts[0].endCol, a.elts[1].col, a.elts[1].endCol)
		co := module(filename, bc, lt, consts, names)
		co.StackSize = 2
		return co, nil
	default:
		// 3+ elements: BUILD_LIST 0 + LOAD_CONST tuple + LIST_EXTEND 1.
		// co_consts: (first_elem, None, ConstTuple{all_elems})
		first := a.elts[0].val
		tup := make(bytecode.ConstTuple, n)
		for i, e := range a.elts {
			tup[i] = e.val
		}
		consts := []any{first, nil, tup}
		bc := bytecode.ConstLitListExtendBytecode()
		lt := bytecode.ConstLitListExtendLineTable(a.line, a.targetLen, a.openCol, a.closeEnd)
		co := module(filename, bc, lt, consts, names)
		co.StackSize = 2
		return co, nil
	}
}
