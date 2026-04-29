package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// attrStoreModule captures the modAttrStore shape: a single
// `<obj>.<attr> = <val>` statement where obj and val are Names of
// 1..15 ASCII chars and attr is a 1..15 ASCII char identifier on the
// same source line.
type attrStoreModule struct {
	Line     uint32
	ValName  string
	ValCol   uint16
	ValEnd   uint16
	ObjName  string
	ObjCol   uint16
	ObjEnd   uint16
	AttrName string
	AttrEnd  uint16 // = ObjEnd + 1 + len(AttrName)
}

// classifyAttrStoreModule recognizes a single-statement
// `<obj>.<attr> = <val>` body. To stay byte-identical with the
// classifier's compileAttrStore (which has no slot for trailing
// no-ops), the codegen path accepts only single-statement modules.
func classifyAttrStoreModule(mod *ast.Module, _ []byte) (attrStoreModule, bool) {
	if len(mod.Body) != 1 {
		return attrStoreModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return attrStoreModule{}, false
	}
	if len(a.Targets) != 1 {
		return attrStoreModule{}, false
	}
	attr, ok := a.Targets[0].(*ast.Attribute)
	if !ok {
		return attrStoreModule{}, false
	}
	obj, ok := attr.Value.(*ast.Name)
	if !ok {
		return attrStoreModule{}, false
	}
	val, ok := a.Value.(*ast.Name)
	if !ok {
		return attrStoreModule{}, false
	}
	if n := len(obj.Id); n < 1 || n > 15 {
		return attrStoreModule{}, false
	}
	if n := len(attr.Attr); n < 1 || n > 15 {
		return attrStoreModule{}, false
	}
	if n := len(val.Id); n < 1 || n > 15 {
		return attrStoreModule{}, false
	}
	if obj.P.Col > 255 || val.P.Col > 255 {
		return attrStoreModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return attrStoreModule{}, false
	}
	objEnd := uint16(obj.P.Col) + uint16(len(obj.Id))
	valEnd := uint16(val.P.Col) + uint16(len(val.Id))
	attrEnd := objEnd + 1 + uint16(len(attr.Attr)) // +1 for the '.'
	return attrStoreModule{
		Line:     uint32(line),
		ValName:  val.Id,
		ValCol:   uint16(val.P.Col),
		ValEnd:   valEnd,
		ObjName:  obj.Id,
		ObjCol:   uint16(obj.P.Col),
		ObjEnd:   objEnd,
		AttrName: attr.Attr,
		AttrEnd:  attrEnd,
	}, true
}

// buildAttrStoreModule emits the bytecode CPython 3.14 generates for
// `<obj>.<attr> = <val>` at module scope. Mirrors
// `bytecode.AttrStoreBytecode` / `bytecode.AttrStoreLineTable`
// byte-for-byte.
//
// Layout (the assembler appends 4 cache words after STORE_ATTR):
//
//	RESUME 0
//	LOAD_NAME 0           ; val (right-hand side, loaded first)
//	LOAD_NAME 1           ; obj
//	STORE_ATTR 2          ; attr (oparg = attrIdx in co_names)
//	LOAD_CONST 0          ; None
//	RETURN_VALUE
//
// co_consts = [nil].
// co_names  = [val, obj, attr].
// co.StackSize = 2.
func buildAttrStoreModule(a attrStoreModule, opts Options) (*bytecode.CodeObject, error) {
	if a.ValName == "" || a.ObjName == "" || a.AttrName == "" {
		return nil, errors.New("codegen.buildAttrStoreModule: empty name")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	valLoc := bytecode.Loc{
		Line: a.Line, EndLine: a.Line,
		Col: a.ValCol, EndCol: a.ValEnd,
	}
	objLoc := bytecode.Loc{
		Line: a.Line, EndLine: a.Line,
		Col: a.ObjCol, EndCol: a.ObjEnd,
	}
	storeLoc := bytecode.Loc{
		Line: a.Line, EndLine: a.Line,
		Col: a.ObjCol, EndCol: a.AttrEnd,
	}

	// STORE_ATTR oparg = attrIdx where attr is at co_names[2].
	const storeAttrOparg uint32 = 2

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: valLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 1, Loc: objLoc},
		ir.Instr{Op: bytecode.STORE_ATTR, Arg: storeAttrOparg, Loc: storeLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: storeLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: storeLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{a.ValName, a.ObjName, a.AttrName},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 2 {
		co.StackSize = 2
	}
	return co, nil
}
