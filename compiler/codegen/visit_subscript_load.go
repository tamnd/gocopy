package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// subscriptLoadModule captures the modSubscriptLoad shape: a single
// `<target> = <obj>[<key>]` statement where target, obj and key are
// all Names of 1..15 ASCII chars on the same source line.
type subscriptLoadModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	ObjName   string
	ObjCol    uint16
	ObjEnd    uint16
	KeyName   string
	KeyCol    uint16
	KeyEnd    uint16
	CloseEnd  uint16 // col after `]` = KeyEnd + 1 for `a[b]`
}

// classifySubscriptLoadModule recognizes a single-statement
// `<target> = <obj>[<key>]` body. To stay byte-identical with the
// classifier's compileSubscriptLoad (which has no slot for trailing
// no-ops), the codegen path accepts only single-statement modules.
func classifySubscriptLoadModule(mod *ast.Module, _ []byte) (subscriptLoadModule, bool) {
	if len(mod.Body) != 1 {
		return subscriptLoadModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return subscriptLoadModule{}, false
	}
	if len(a.Targets) != 1 {
		return subscriptLoadModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return subscriptLoadModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return subscriptLoadModule{}, false
	}
	sub, ok := a.Value.(*ast.Subscript)
	if !ok {
		return subscriptLoadModule{}, false
	}
	obj, ok := sub.Value.(*ast.Name)
	if !ok {
		return subscriptLoadModule{}, false
	}
	key, ok := sub.Slice.(*ast.Name)
	if !ok {
		return subscriptLoadModule{}, false
	}
	if n := len(obj.Id); n < 1 || n > 15 {
		return subscriptLoadModule{}, false
	}
	if n := len(key.Id); n < 1 || n > 15 {
		return subscriptLoadModule{}, false
	}
	if obj.P.Col > 255 || key.P.Col > 255 {
		return subscriptLoadModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return subscriptLoadModule{}, false
	}
	objEnd := uint16(obj.P.Col) + uint16(len(obj.Id))
	keyEnd := uint16(key.P.Col) + uint16(len(key.Id))
	return subscriptLoadModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		ObjName:   obj.Id,
		ObjCol:    uint16(obj.P.Col),
		ObjEnd:    objEnd,
		KeyName:   key.Id,
		KeyCol:    uint16(key.P.Col),
		KeyEnd:    keyEnd,
		CloseEnd:  keyEnd + 1, // mirror classifier's extractSubscriptLoad
	}, true
}

// buildSubscriptLoadModule emits the bytecode CPython 3.14 generates
// for `<target> = <obj>[<key>]` at module scope. Mirrors
// `bytecode.SubscriptLoadBytecode` /
// `bytecode.SubscriptLoadLineTable` byte-for-byte.
//
// Layout (the assembler appends 5 cache words after BINARY_OP):
//
//	RESUME 0
//	LOAD_NAME 0           ; obj
//	LOAD_NAME 1           ; key
//	BINARY_OP NbGetItem   ; +5 cache words
//	STORE_NAME 2          ; target
//	LOAD_CONST 0          ; None
//	RETURN_VALUE
//
// co_consts = [nil].
// co_names  = [obj, key, target].
// co.StackSize = 2.
func buildSubscriptLoadModule(s subscriptLoadModule, opts Options) (*bytecode.CodeObject, error) {
	if s.TargetLen == 0 || s.TargetLen > 15 {
		return nil, errors.New("codegen.buildSubscriptLoadModule: target name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	objLoc := bytecode.Loc{
		Line: s.Line, EndLine: s.Line,
		Col: s.ObjCol, EndCol: s.ObjEnd,
	}
	keyLoc := bytecode.Loc{
		Line: s.Line, EndLine: s.Line,
		Col: s.KeyCol, EndCol: s.KeyEnd,
	}
	binOpLoc := bytecode.Loc{
		Line: s.Line, EndLine: s.Line,
		Col: s.ObjCol, EndCol: s.CloseEnd,
	}
	targetLoc := bytecode.Loc{
		Line: s.Line, EndLine: s.Line,
		Col: 0, EndCol: s.TargetLen,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: objLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 1, Loc: keyLoc},
		ir.Instr{Op: bytecode.BINARY_OP, Arg: bytecode.NbGetItem, Loc: binOpLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 2, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{s.ObjName, s.KeyName, s.Target},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 2 {
		co.StackSize = 2
	}
	return co, nil
}
