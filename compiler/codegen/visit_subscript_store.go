package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// subscriptStoreModule captures the modSubscriptStore shape: a single
// `<obj>[<key>] = <val>` statement where obj, key and val are all
// Names of 1..15 ASCII chars on the same source line.
type subscriptStoreModule struct {
	Line     uint32
	ValName  string
	ValCol   uint16
	ValEnd   uint16
	ObjName  string
	ObjCol   uint16
	ObjEnd   uint16
	KeyName  string
	KeyCol   uint16
	KeyEnd   uint16
	CloseEnd uint16 // col after `]` = KeyEnd + 1
}

// classifySubscriptStoreModule recognizes a single-statement
// `<obj>[<key>] = <val>` body. To stay byte-identical with the
// classifier's compileSubscriptStore (which has no slot for trailing
// no-ops), the codegen path accepts only single-statement modules.
func classifySubscriptStoreModule(mod *ast.Module, _ []byte) (subscriptStoreModule, bool) {
	if len(mod.Body) != 1 {
		return subscriptStoreModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return subscriptStoreModule{}, false
	}
	if len(a.Targets) != 1 {
		return subscriptStoreModule{}, false
	}
	sub, ok := a.Targets[0].(*ast.Subscript)
	if !ok {
		return subscriptStoreModule{}, false
	}
	obj, ok := sub.Value.(*ast.Name)
	if !ok {
		return subscriptStoreModule{}, false
	}
	key, ok := sub.Slice.(*ast.Name)
	if !ok {
		return subscriptStoreModule{}, false
	}
	val, ok := a.Value.(*ast.Name)
	if !ok {
		return subscriptStoreModule{}, false
	}
	if n := len(obj.Id); n < 1 || n > 15 {
		return subscriptStoreModule{}, false
	}
	if n := len(key.Id); n < 1 || n > 15 {
		return subscriptStoreModule{}, false
	}
	if n := len(val.Id); n < 1 || n > 15 {
		return subscriptStoreModule{}, false
	}
	if obj.P.Col > 255 || key.P.Col > 255 || val.P.Col > 255 {
		return subscriptStoreModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return subscriptStoreModule{}, false
	}
	objEnd := uint16(obj.P.Col) + uint16(len(obj.Id))
	keyEnd := uint16(key.P.Col) + uint16(len(key.Id))
	valEnd := uint16(val.P.Col) + uint16(len(val.Id))
	return subscriptStoreModule{
		Line:     uint32(line),
		ValName:  val.Id,
		ValCol:   uint16(val.P.Col),
		ValEnd:   valEnd,
		ObjName:  obj.Id,
		ObjCol:   uint16(obj.P.Col),
		ObjEnd:   objEnd,
		KeyName:  key.Id,
		KeyCol:   uint16(key.P.Col),
		KeyEnd:   keyEnd,
		CloseEnd: keyEnd + 1, // mirror classifier's extractSubscriptStore
	}, true
}

// buildSubscriptStoreModule emits the bytecode CPython 3.14 generates
// for `<obj>[<key>] = <val>` at module scope. Mirrors
// `bytecode.SubscriptStoreBytecode` /
// `bytecode.SubscriptStoreLineTable` byte-for-byte.
//
// Layout (the assembler appends 1 cache word after STORE_SUBSCR):
//
//	RESUME 0
//	LOAD_NAME 0           ; val (right-hand side, loaded first)
//	LOAD_NAME 1           ; obj
//	LOAD_NAME 2           ; key
//	STORE_SUBSCR 0        ; +1 cache word
//	LOAD_CONST 0          ; None
//	RETURN_VALUE
//
// co_consts = [nil].
// co_names  = [val, obj, key].
// co.StackSize = 3.
func buildSubscriptStoreModule(s subscriptStoreModule, opts Options) (*bytecode.CodeObject, error) {
	if s.ValName == "" || s.ObjName == "" || s.KeyName == "" {
		return nil, errors.New("codegen.buildSubscriptStoreModule: empty name")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	valLoc := bytecode.Loc{
		Line: s.Line, EndLine: s.Line,
		Col: s.ValCol, EndCol: s.ValEnd,
	}
	objLoc := bytecode.Loc{
		Line: s.Line, EndLine: s.Line,
		Col: s.ObjCol, EndCol: s.ObjEnd,
	}
	keyLoc := bytecode.Loc{
		Line: s.Line, EndLine: s.Line,
		Col: s.KeyCol, EndCol: s.KeyEnd,
	}
	storeLoc := bytecode.Loc{
		Line: s.Line, EndLine: s.Line,
		Col: s.ObjCol, EndCol: s.CloseEnd,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: valLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 1, Loc: objLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 2, Loc: keyLoc},
		ir.Instr{Op: bytecode.STORE_SUBSCR, Arg: 0, Loc: storeLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: storeLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: storeLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{s.ValName, s.ObjName, s.KeyName},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 3 {
		co.StackSize = 3
	}
	return co, nil
}
