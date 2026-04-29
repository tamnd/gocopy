package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// attrLoadModule captures the modAttrLoad shape: a single
// `<target> = <obj>.<attr>` statement where target and obj are Names
// of 1..15 ASCII chars and attr is a 1..15 ASCII char identifier on
// the same source line.
type attrLoadModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	ObjName   string
	ObjCol    uint16
	ObjEnd    uint16
	AttrName  string
	AttrEnd   uint16 // = ObjEnd + 1 + len(AttrName)
}

// classifyAttrLoadModule recognizes a single-statement
// `<target> = <obj>.<attr>` body. To stay byte-identical with the
// classifier's compileAttrLoad (which has no slot for trailing
// no-ops), the codegen path accepts only single-statement modules.
func classifyAttrLoadModule(mod *ast.Module, _ []byte) (attrLoadModule, bool) {
	if len(mod.Body) != 1 {
		return attrLoadModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return attrLoadModule{}, false
	}
	if len(a.Targets) != 1 {
		return attrLoadModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return attrLoadModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return attrLoadModule{}, false
	}
	attr, ok := a.Value.(*ast.Attribute)
	if !ok {
		return attrLoadModule{}, false
	}
	obj, ok := attr.Value.(*ast.Name)
	if !ok {
		return attrLoadModule{}, false
	}
	if n := len(obj.Id); n < 1 || n > 15 {
		return attrLoadModule{}, false
	}
	if n := len(attr.Attr); n < 1 || n > 15 {
		return attrLoadModule{}, false
	}
	if obj.P.Col > 255 {
		return attrLoadModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return attrLoadModule{}, false
	}
	objEnd := uint16(obj.P.Col) + uint16(len(obj.Id))
	attrEnd := objEnd + 1 + uint16(len(attr.Attr)) // +1 for the '.'
	return attrLoadModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		ObjName:   obj.Id,
		ObjCol:    uint16(obj.P.Col),
		ObjEnd:    objEnd,
		AttrName:  attr.Attr,
		AttrEnd:   attrEnd,
	}, true
}

// buildAttrLoadModule emits the bytecode CPython 3.14 generates for
// `<target> = <obj>.<attr>` at module scope. Mirrors
// `bytecode.AttrLoadBytecode` / `bytecode.AttrLoadLineTable`
// byte-for-byte.
//
// Layout (the assembler appends 9 cache words after LOAD_ATTR; the
// 10-code-unit run is split by the line-table encoder into 8+2
// entries with the same Loc):
//
//	RESUME 0
//	LOAD_NAME 0           ; obj
//	LOAD_ATTR 2           ; attr (oparg = attrIdx<<1 = 1<<1)
//	STORE_NAME 2          ; target
//	LOAD_CONST 0          ; None
//	RETURN_VALUE
//
// co_consts = [nil].
// co_names  = [obj, attr, target].
// co.StackSize = 1.
func buildAttrLoadModule(a attrLoadModule, opts Options) (*bytecode.CodeObject, error) {
	if a.TargetLen == 0 || a.TargetLen > 15 {
		return nil, errors.New("codegen.buildAttrLoadModule: target name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	objLoc := bytecode.Loc{
		Line: a.Line, EndLine: a.Line,
		Col: a.ObjCol, EndCol: a.ObjEnd,
	}
	attrLoc := bytecode.Loc{
		Line: a.Line, EndLine: a.Line,
		Col: a.ObjCol, EndCol: a.AttrEnd,
	}
	targetLoc := bytecode.Loc{
		Line: a.Line, EndLine: a.Line,
		Col: 0, EndCol: a.TargetLen,
	}

	// LOAD_ATTR oparg = attrIdx<<1 where attr is at co_names[1].
	const loadAttrOparg uint32 = 1 << 1

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: objLoc},
		ir.Instr{Op: bytecode.LOAD_ATTR, Arg: loadAttrOparg, Loc: attrLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 2, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{a.ObjName, a.AttrName, a.Target},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 1 {
		co.StackSize = 1
	}
	return co, nil
}
