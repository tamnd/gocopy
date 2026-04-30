package compiler

import (
	"github.com/tamnd/gocopy/bytecode"
)

// compileMixed lowers a module whose body is:
//
//	[docstring?] [starImport?] [constLitColl?] [assigns*] [funcBodyExprs+]
//
// co_consts layout: [docstr?, ('*',)?, code_0..code_N-1, None, clcTuple?, foldedVals...]
// co_names layout:  [__doc__?, module?, __all__?, assignNames..., funcNames...]
func compileMixed(filename string, cls classification) (*bytecode.CodeObject, error) {
	m := cls.mixedModuleAsgn
	nFuncs := len(m.funcs)
	nAsgns := len(m.assigns)

	// Compile each function body.
	type funcEntry struct {
		code        *bytecode.CodeObject
		bodyEndLine int
		bodyEndCol  byte
	}
	entries := make([]funcEntry, nFuncs)
	for i, f := range m.funcs {
		var code *bytecode.CodeObject
		var endLine int
		var endCol byte
		var err error
		if f.isFuncBodyExpr {
			innerCls := classification{kind: modFuncBodyExpr, funcBodyAsgn: f.funcBody}
			code, endLine, endCol, err = compileFuncBodyCore(filename, innerCls)
		} else {
			code, endLine, endCol, err = compileFuncDefInner(filename, f.funcDef)
		}
		if err != nil {
			return nil, err
		}
		entries[i] = funcEntry{code, endLine, endCol}
	}

	// Build co_consts:
	//   [docstr?]          index 0 if hasDocstring
	//   [('*',)?]          right after docstring if hasStarImport
	//   [code_0..code_N-1] function code objects
	//   [None]
	//   [clcTuple?]        if hasCLC
	//   [foldedVals...]    one per foldedBinOp assign
	consts := []any{}

	docConstIdx := byte(0)
	if m.hasDocstring {
		docConstIdx = byte(len(consts))
		consts = append(consts, m.docText)
	}

	starImportConstIdx := byte(0)
	if m.hasStarImport {
		starImportConstIdx = byte(len(consts))
		consts = append(consts, bytecode.ConstTuple{"*"})
	}

	funcConstBase := byte(len(consts))
	for _, e := range entries {
		consts = append(consts, e.code)
	}

	noneIdx := byte(len(consts))
	consts = append(consts, nil)

	clcConstIdx := byte(0)
	if m.hasCLC {
		clcConstIdx = byte(len(consts))
		tup := make(bytecode.ConstTuple, len(m.clc.elts))
		for i, e := range m.clc.elts {
			tup[i] = e.val
		}
		consts = append(consts, tup)
	}

	// FoldedBinOp results go to co_consts; small-int assigns use LOAD_SMALL_INT and add nothing.
	foldedConstBase := byte(len(consts))
	for _, a := range m.assigns {
		if fold, ok := a.value.(foldedBinOp); ok {
			consts = append(consts, fold.result)
		}
	}

	// Build co_names:
	//   __doc__ (if hasDocstring)
	//   module name (if hasStarImport)
	//   clc.target (if hasCLC)
	//   assign names in order
	//   func names in order
	names := []string{}
	if m.hasDocstring {
		names = append(names, "__doc__")
	}
	starImportNameIdx := byte(0)
	if m.hasStarImport {
		starImportNameIdx = byte(len(names))
		names = append(names, m.starImportModule)
	}
	clcNameIdx := byte(0)
	if m.hasCLC {
		clcNameIdx = byte(len(names))
		names = append(names, m.clc.target)
	}
	asgnNameBase := byte(len(names))
	for _, a := range m.assigns {
		names = append(names, a.name)
	}
	funcNameBase := byte(len(names))
	for _, f := range m.funcs {
		names = append(names, mixedFuncName(f))
	}

	// Build a name→co_names index map for LOAD_NAME lookups.
	nameIdx := map[string]byte{}
	for i, n := range names {
		nameIdx[n] = byte(i)
	}

	// Build bytecode.
	bc := make([]byte, 0, 2+4+10+4*nAsgns+6*nFuncs+4)
	bc = append(bc, byte(bytecode.RESUME), 0)

	if m.hasDocstring {
		bc = append(bc, byte(bytecode.LOAD_CONST), docConstIdx)
		bc = append(bc, byte(bytecode.STORE_NAME), 0) // __doc__ is always names[0] when present
	}

	if m.hasStarImport {
		bc = append(bc,
			byte(bytecode.LOAD_SMALL_INT), 0,
			byte(bytecode.LOAD_CONST), starImportConstIdx,
			byte(bytecode.IMPORT_NAME), starImportNameIdx,
			byte(bytecode.CALL_INTRINSIC_1), 2,
			byte(bytecode.POP_TOP), 0,
		)
	}

	if m.hasCLC {
		bc = append(bc, byte(bytecode.BUILD_LIST), 0)
		bc = append(bc, byte(bytecode.LOAD_CONST), clcConstIdx)
		bc = append(bc, byte(bytecode.LIST_EXTEND), 1)
		bc = append(bc, byte(bytecode.STORE_NAME), clcNameIdx)
	}

	foldedIdx := foldedConstBase
	for i, a := range m.assigns {
		switch v := a.value.(type) {
		case int64:
			bc = append(bc, byte(bytecode.LOAD_SMALL_INT), byte(v))
		case foldedBinOp:
			bc = append(bc, byte(bytecode.LOAD_CONST), foldedIdx)
			foldedIdx++
		}
		bc = append(bc, byte(bytecode.STORE_NAME), asgnNameBase+byte(i))
	}

	for i, f := range m.funcs {
		defs := mixedFuncDefaults(f)
		if len(defs) > 0 {
			for _, d := range defs {
				idx := nameIdx[d.name]
				bc = append(bc, byte(bytecode.LOAD_NAME), idx)
			}
			bc = append(bc, byte(bytecode.BUILD_TUPLE), byte(len(defs)))
			bc = append(bc, byte(bytecode.LOAD_CONST), funcConstBase+byte(i))
			bc = append(bc, byte(bytecode.MAKE_FUNCTION), 0)
			bc = append(bc, byte(bytecode.SET_FUNCTION_ATTRIBUTE), 1)
		} else {
			bc = append(bc, byte(bytecode.LOAD_CONST), funcConstBase+byte(i))
			bc = append(bc, byte(bytecode.MAKE_FUNCTION), 0)
		}
		bc = append(bc, byte(bytecode.STORE_NAME), funcNameBase+byte(i))
	}

	bc = append(bc, byte(bytecode.LOAD_CONST), noneIdx)
	bc = append(bc, byte(bytecode.RETURN_VALUE), 0)

	// Build linetable.
	info := bytecode.MixedModuleInfo{
		HasDocstring:     m.hasDocstring,
		DocLine:          m.docLine,
		DocEndLine:       m.docEndLine,
		DocEndCol:        m.docEndCol,
		HasStarImport:    m.hasStarImport,
		StarImportLine:   m.starImportLine,
		StarImportEndCol: m.starImportEndCol,
		HasCLC:           m.hasCLC,
		CLCLine:          m.clc.line,
		CLCCloseLine:     m.clc.closeLine,
		CLCOpenCol:       m.clc.openCol,
		CLCCloseEnd:      m.clc.closeEnd,
		CLCTargetLen:     m.clc.targetLen,
	}
	info.Assigns = make([]bytecode.AssignInfo, nAsgns)
	for i, a := range m.assigns {
		info.Assigns[i] = bytecode.AssignInfo{
			Line:     a.line,
			NameLen:  a.nameLen,
			ValStart: a.valStart,
			ValEnd:   a.valEnd,
		}
	}
	info.Funcs = make([]bytecode.MultiFuncDefEntry, nFuncs)
	for i, f := range m.funcs {
		entry := bytecode.MultiFuncDefEntry{
			DefLine:     mixedFuncDefLine(f),
			BodyEndLine: entries[i].bodyEndLine,
			BodyEndCol:  entries[i].bodyEndCol,
		}
		defs := mixedFuncDefaults(f)
		if len(defs) > 0 {
			entry.Defaults = make([]bytecode.DefaultInfo, len(defs))
			for j, d := range defs {
				entry.Defaults[j] = bytecode.DefaultInfo{
					Line:     d.line,
					ColStart: d.colStart,
					ColEnd:   d.colEnd,
				}
			}
		}
		info.Funcs[i] = entry
	}
	lt := bytecode.MixedModuleLineTable(info)

	// Stack size: LOAD_CONST+MAKE_FUNCTION paths peak at 1; star import and
	// CLC (BUILD_LIST + LOAD_CONST) peak at 2; K Name-defaults push K values
	// before BUILD_TUPLE then LOAD_CONST code adds 1 more (peak = max(2, K)).
	stackSize := int32(1)
	if m.hasStarImport || m.hasCLC {
		stackSize = 2
	}
	for _, f := range m.funcs {
		k := int32(len(mixedFuncDefaults(f)))
		if k > 0 {
			need := max(int32(2), k)
			if need > stackSize {
				stackSize = need
			}
		}
	}

	co := module(filename, bc, lt, consts, names)
	co.StackSize = stackSize
	return co, nil
}

// mixedFuncName returns the function name for a mixedFunc.
func mixedFuncName(f mixedFunc) string {
	if f.isFuncBodyExpr {
		return f.funcBody.funcName
	}
	return f.funcDef.funcName
}

// mixedFuncDefLine returns the def-line for a mixedFunc.
func mixedFuncDefLine(f mixedFunc) int {
	if f.isFuncBodyExpr {
		return f.funcBody.defLine
	}
	return f.funcDef.defLine
}

// mixedFuncDefaults returns the Name-expression defaults for a mixedFunc.
// stmtFuncDef functions have no defaults.
func mixedFuncDefaults(f mixedFunc) []fbDefault {
	if f.isFuncBodyExpr {
		return f.funcBody.defaults
	}
	return nil
}

// compileFuncDefInner compiles the inner code object for a stmtFuncDef
// (def f(arg): return arg) and returns it alongside line/col info for the
// mixed module linetable.
func compileFuncDefInner(filename string, fd funcDefClassify) (*bytecode.CodeObject, int, byte, error) {
	code := &bytecode.CodeObject{
		ArgCount:        1,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0x3,
		Bytecode:        bytecode.FuncReturnArgBytecode(0),
		Consts:          []any{nil},
		Names:           []string{},
		LocalsPlusNames: []string{fd.argName},
		LocalsPlusKinds: []byte{bytecode.LocalsKindArg},
		Filename:        filename,
		Name:            fd.funcName,
		QualName:        fd.funcName,
		FirstLineNo:     int32(fd.defLine),
		LineTable:       bytecode.FuncReturnArgLineTable(fd.defLine, fd.bodyLine, fd.argCol, fd.argEnd, fd.retKwCol),
		ExcTable:        []byte{},
	}
	return code, fd.bodyLine, fd.argEnd, nil
}
