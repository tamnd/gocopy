package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// callAssignModule captures the modCallAssign shape: a single
// `<target> = <func>(<arg0>, ..., <argN-1>)` statement where target,
// func and every positional arg are Names of 1..15 ASCII chars on
// the same source line. Keyword arguments and unpacking are not
// supported.
type callAssignModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	FuncName  string
	FuncCol   uint16
	FuncEnd   uint16
	Args      []bytecode.CallArg
	CloseEnd  uint16 // exclusive end column of `)` = line tail
}

// classifyCallAssignModule recognizes a single-statement
// `<target> = <func>(<args...>)` body. To stay byte-identical with
// the classifier's compileCallAssign (which has no slot for trailing
// no-ops), the codegen path accepts only single-statement modules.
func classifyCallAssignModule(mod *ast.Module, src []byte) (callAssignModule, bool) {
	if len(mod.Body) != 1 {
		return callAssignModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return callAssignModule{}, false
	}
	if len(a.Targets) != 1 {
		return callAssignModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return callAssignModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return callAssignModule{}, false
	}
	call, ok := a.Value.(*ast.Call)
	if !ok {
		return callAssignModule{}, false
	}
	if len(call.Keywords) != 0 {
		return callAssignModule{}, false
	}
	fn, ok := call.Func.(*ast.Name)
	if !ok {
		return callAssignModule{}, false
	}
	if n := len(fn.Id); n < 1 || n > 15 {
		return callAssignModule{}, false
	}
	if fn.P.Col > 255 {
		return callAssignModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return callAssignModule{}, false
	}
	args := make([]bytecode.CallArg, 0, len(call.Args))
	for _, e := range call.Args {
		n, isName := e.(*ast.Name)
		if !isName {
			return callAssignModule{}, false
		}
		if n.P.Col > 255 || len(n.Id) > 15 || n.P.Line != line {
			return callAssignModule{}, false
		}
		args = append(args, bytecode.CallArg{
			Name:    n.Id,
			Col:     byte(n.P.Col),
			NameLen: byte(len(n.Id)),
		})
	}
	lines := splitLines(src)
	closeEnd, ok := lineEndCol(lines, line)
	if !ok {
		return callAssignModule{}, false
	}
	funcEnd := uint16(fn.P.Col) + uint16(len(fn.Id))
	return callAssignModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		FuncName:  fn.Id,
		FuncCol:   uint16(fn.P.Col),
		FuncEnd:   funcEnd,
		Args:      args,
		CloseEnd:  uint16(closeEnd),
	}, true
}

// buildCallAssignModule emits the bytecode CPython 3.14 generates
// for `<target> = <func>(<args...>)` at module scope. Mirrors
// `bytecode.CallAssignBytecode` / `bytecode.CallAssignLineTable`
// byte-for-byte.
//
// Layout (the assembler appends 3 cache words after CALL):
//
//	RESUME 0
//	LOAD_NAME 0           ; func
//	PUSH_NULL 0           ; CPython non-method calling convention
//	LOAD_NAME 1           ; arg0       (repeated N times)
//	  ...
//	CALL N                ; +3 cache words
//	STORE_NAME N+1        ; target
//	LOAD_CONST 0          ; None
//	RETURN_VALUE
//
// co_consts = [nil].
// co_names  = [func, arg0, ..., argN-1, target].
// co.StackSize = 2 + N.
func buildCallAssignModule(c callAssignModule, opts Options) (*bytecode.CodeObject, error) {
	if c.TargetLen == 0 || c.TargetLen > 15 {
		return nil, errors.New("codegen.buildCallAssignModule: target name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	funcLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: c.FuncCol, EndCol: c.FuncEnd,
	}
	callLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: c.FuncCol, EndCol: c.CloseEnd,
	}
	targetLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: 0, EndCol: c.TargetLen,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: funcLoc},
		ir.Instr{Op: bytecode.PUSH_NULL, Arg: 0, Loc: funcLoc},
	)
	for i, a := range c.Args {
		argLoc := bytecode.Loc{
			Line: c.Line, EndLine: c.Line,
			Col: uint16(a.Col), EndCol: uint16(a.Col) + uint16(a.NameLen),
		}
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_NAME, Arg: uint32(i + 1), Loc: argLoc},
		)
	}
	n := uint32(len(c.Args))
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.CALL, Arg: n, Loc: callLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: n + 1, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	names := make([]string, 0, len(c.Args)+2)
	names = append(names, c.FuncName)
	for _, a := range c.Args {
		names = append(names, a.Name)
	}
	names = append(names, c.Target)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    names,
	})
	if err != nil {
		return nil, err
	}
	want := int32(2) + int32(n)
	if co.StackSize < want {
		co.StackSize = want
	}
	return co, nil
}
