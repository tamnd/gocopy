package compiler

import (
	"github.com/tamnd/gocopy/bytecode"
)

// compileMixed lowers a module whose body is:
//
//	[docstring?] [constLitColl?] [foldedBinOp assigns*] [funcBodyExprs+]
//
// This is the shape of CPython's colorsys.py.
//
// co_consts layout: [docstr?, code_0..code_N-1, None, clcTuple?, foldedVals...]
// co_names layout:  [__doc__?, __all__?, assignNames..., funcNames...]
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
		innerCls := classification{kind: modFuncBodyExpr, funcBodyAsgn: f}
		code, endLine, endCol, err := compileFuncBodyCore(filename, innerCls)
		if err != nil {
			return nil, err
		}
		entries[i] = funcEntry{code, endLine, endCol}
	}

	// Build co_consts:
	//   [docstr?]  index 0 if hasDocstring
	//   [code_0 .. code_N-1]  function code objects
	//   [None]
	//   [clcTuple?]  if hasCLC
	//   [foldedVals...]  one per assign
	consts := []any{}

	docConstIdx := byte(0)
	if m.hasDocstring {
		docConstIdx = byte(len(consts))
		consts = append(consts, m.docText)
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

	// One folded result per assign, appended after None (and CLC tuple if present).
	foldedConstBase := byte(len(consts))
	for _, a := range m.assigns {
		fold := a.value.(foldedBinOp)
		consts = append(consts, fold.result)
	}

	// Build co_names:
	//   __doc__ (if hasDocstring)
	//   clc.target (if hasCLC)
	//   assign names in order
	//   func names in order
	names := []string{}
	if m.hasDocstring {
		names = append(names, "__doc__")
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
		names = append(names, f.funcName)
	}

	// Build bytecode.
	bc := make([]byte, 0, 2+4+4+4*nAsgns+6*nFuncs+4)
	bc = append(bc, byte(bytecode.RESUME), 0)

	if m.hasDocstring {
		bc = append(bc, byte(bytecode.LOAD_CONST), docConstIdx)
		bc = append(bc, byte(bytecode.STORE_NAME), 0) // __doc__ is always names[0] when present
	}

	if m.hasCLC {
		bc = append(bc, byte(bytecode.BUILD_LIST), 0)
		bc = append(bc, byte(bytecode.LOAD_CONST), clcConstIdx)
		bc = append(bc, byte(bytecode.LIST_EXTEND), 1)
		bc = append(bc, byte(bytecode.STORE_NAME), clcNameIdx)
	}

	for i := range nAsgns {
		bc = append(bc, byte(bytecode.LOAD_CONST), foldedConstBase+byte(i))
		bc = append(bc, byte(bytecode.STORE_NAME), asgnNameBase+byte(i))
	}

	for i := range nFuncs {
		bc = append(bc, byte(bytecode.LOAD_CONST), funcConstBase+byte(i))
		bc = append(bc, byte(bytecode.MAKE_FUNCTION), 0)
		bc = append(bc, byte(bytecode.STORE_NAME), funcNameBase+byte(i))
	}

	bc = append(bc, byte(bytecode.LOAD_CONST), noneIdx)
	bc = append(bc, byte(bytecode.RETURN_VALUE), 0)

	// Build linetable.
	info := bytecode.MixedModuleInfo{
		HasDocstring: m.hasDocstring,
		DocLine:      m.docLine,
		DocEndLine:   m.docEndLine,
		DocEndCol:    m.docEndCol,
		HasCLC:       m.hasCLC,
		CLCLine:      m.clc.line,
		CLCCloseLine: m.clc.closeLine,
		CLCOpenCol:   m.clc.openCol,
		CLCCloseEnd:  m.clc.closeEnd,
		CLCTargetLen: m.clc.targetLen,
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
		info.Funcs[i] = bytecode.MultiFuncDefEntry{
			DefLine:     f.defLine,
			BodyEndLine: entries[i].bodyEndLine,
			BodyEndCol:  entries[i].bodyEndCol,
		}
	}
	lt := bytecode.MixedModuleLineTable(info)

	co := module(filename, bc, lt, consts, names)
	co.StackSize = 2
	return co, nil
}
