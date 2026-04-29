package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// ternaryAssignModule captures the modTernary shape: a single
// `<target> = <trueVal> if <cond> else <falseVal>` statement where
// target, cond, trueVal, and falseVal are all Names of 1..15 ASCII
// chars.
type ternaryAssignModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	CondName  string
	CondCol   uint16
	CondLen   uint16
	TrueName  string
	TrueCol   uint16
	TrueLen   uint16
	FalseName string
	FalseCol  uint16
	FalseLen  uint16
}

// classifyTernaryAssignModule recognizes a single-statement
// `<target> = <trueVal> if <cond> else <falseVal>` body. To stay
// byte-identical with the classifier's compileTernary (which has no
// slot for trailing no-ops), the codegen path accepts only
// single-statement modules.
func classifyTernaryAssignModule(mod *ast.Module, _ []byte) (ternaryAssignModule, bool) {
	if len(mod.Body) != 1 {
		return ternaryAssignModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return ternaryAssignModule{}, false
	}
	if len(a.Targets) != 1 {
		return ternaryAssignModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return ternaryAssignModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return ternaryAssignModule{}, false
	}
	ifx, ok := a.Value.(*ast.IfExp)
	if !ok {
		return ternaryAssignModule{}, false
	}
	cond, ok := ifx.Test.(*ast.Name)
	if !ok {
		return ternaryAssignModule{}, false
	}
	trueN, ok := ifx.Body.(*ast.Name)
	if !ok {
		return ternaryAssignModule{}, false
	}
	falseN, ok := ifx.OrElse.(*ast.Name)
	if !ok {
		return ternaryAssignModule{}, false
	}
	if n := len(cond.Id); n < 1 || n > 15 {
		return ternaryAssignModule{}, false
	}
	if n := len(trueN.Id); n < 1 || n > 15 {
		return ternaryAssignModule{}, false
	}
	if n := len(falseN.Id); n < 1 || n > 15 {
		return ternaryAssignModule{}, false
	}
	if cond.P.Col > 255 || trueN.P.Col > 255 || falseN.P.Col > 255 {
		return ternaryAssignModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return ternaryAssignModule{}, false
	}
	return ternaryAssignModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		CondName:  cond.Id,
		CondCol:   uint16(cond.P.Col),
		CondLen:   uint16(len(cond.Id)),
		TrueName:  trueN.Id,
		TrueCol:   uint16(trueN.P.Col),
		TrueLen:   uint16(len(trueN.Id)),
		FalseName: falseN.Id,
		FalseCol:  uint16(falseN.P.Col),
		FalseLen:  uint16(len(falseN.Id)),
	}, true
}

// buildTernaryAssignModule emits the bytecode CPython 3.14 generates
// for `<target> = <trueVal> if <cond> else <falseVal>` at module
// scope.
//
// Layout (the IR encoder appends 3 cache words after TO_BOOL and 1
// cache word after the conditional jump):
//
//	RESUME 0
//	LOAD_NAME 0          ; cond
//	TO_BOOL 0            ; encoder appends 3 cache words
//	POP_JUMP_IF_FALSE 5  ; encoder appends 1 cache word
//	NOT_TAKEN 0
//	LOAD_NAME 1          ; trueVal
//	STORE_NAME 3         ; target
//	LOAD_CONST 0         ; None
//	RETURN_VALUE
//	LOAD_NAME 2          ; falseVal
//	STORE_NAME 3         ; target
//	LOAD_CONST 0         ; None
//	RETURN_VALUE
//
// The forward-jump oparg is resolved at codegen time: 5 code units
// past the cache lands on the false-branch LOAD_NAME, mirroring the
// classifier helper byte-for-byte.
//
// co_consts = [nil].
// co_names  = [cond, trueVal, falseVal, target] (insertion order
// matches the classifier).
// co.StackSize = 1.
func buildTernaryAssignModule(t ternaryAssignModule, opts Options) (*bytecode.CodeObject, error) {
	if t.TargetLen == 0 || t.TargetLen > 15 ||
		t.CondLen == 0 || t.CondLen > 15 ||
		t.TrueLen == 0 || t.TrueLen > 15 ||
		t.FalseLen == 0 || t.FalseLen > 15 {
		return nil, errors.New("codegen.buildTernaryAssignModule: name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	condLoc := bytecode.Loc{
		Line: t.Line, EndLine: t.Line,
		Col: t.CondCol, EndCol: t.CondCol + t.CondLen,
	}
	trueLoc := bytecode.Loc{
		Line: t.Line, EndLine: t.Line,
		Col: t.TrueCol, EndCol: t.TrueCol + t.TrueLen,
	}
	falseLoc := bytecode.Loc{
		Line: t.Line, EndLine: t.Line,
		Col: t.FalseCol, EndCol: t.FalseCol + t.FalseLen,
	}
	targetLoc := bytecode.Loc{
		Line: t.Line, EndLine: t.Line,
		Col: 0, EndCol: t.TargetLen,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: condLoc},
		ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: condLoc},
		ir.Instr{Op: bytecode.POP_JUMP_IF_FALSE, Arg: 5, Loc: condLoc},
		ir.Instr{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 1, Loc: trueLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 3, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 2, Loc: falseLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 3, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{t.CondName, t.TrueName, t.FalseName, t.Target},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 1 {
		co.StackSize = 1
	}
	return co, nil
}
