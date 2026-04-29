package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// boolOpAssignModule captures the modBoolOp shape: a single
// `<target> = <left> and <right>` or `<target> = <left> or <right>`
// statement where target, left, and right are all Names of 1..15
// ASCII chars.
type boolOpAssignModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	LeftName  string
	LeftCol   uint16
	LeftLen   uint16
	RightName string
	RightCol  uint16
	RightLen  uint16
	IsOr      bool
}

// classifyBoolOpAssignModule recognizes a single-statement
// `<target> = <left> <bool_op> <right>` body. To stay byte-identical
// with the classifier's compileBoolOp (which has no slot for
// trailing no-ops), the codegen path accepts only single-statement
// modules.
func classifyBoolOpAssignModule(mod *ast.Module, _ []byte) (boolOpAssignModule, bool) {
	if len(mod.Body) != 1 {
		return boolOpAssignModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return boolOpAssignModule{}, false
	}
	if len(a.Targets) != 1 {
		return boolOpAssignModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return boolOpAssignModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return boolOpAssignModule{}, false
	}
	bop, ok := a.Value.(*ast.BoolOp)
	if !ok {
		return boolOpAssignModule{}, false
	}
	if len(bop.Values) != 2 {
		return boolOpAssignModule{}, false
	}
	left, ok := bop.Values[0].(*ast.Name)
	if !ok {
		return boolOpAssignModule{}, false
	}
	right, ok := bop.Values[1].(*ast.Name)
	if !ok {
		return boolOpAssignModule{}, false
	}
	if n := len(left.Id); n < 1 || n > 15 {
		return boolOpAssignModule{}, false
	}
	if n := len(right.Id); n < 1 || n > 15 {
		return boolOpAssignModule{}, false
	}
	if left.P.Col > 255 || right.P.Col > 255 {
		return boolOpAssignModule{}, false
	}
	var isOr bool
	switch bop.Op {
	case "And":
		isOr = false
	case "Or":
		isOr = true
	default:
		return boolOpAssignModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return boolOpAssignModule{}, false
	}
	return boolOpAssignModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		LeftName:  left.Id,
		LeftCol:   uint16(left.P.Col),
		LeftLen:   uint16(len(left.Id)),
		RightName: right.Id,
		RightCol:  uint16(right.P.Col),
		RightLen:  uint16(len(right.Id)),
		IsOr:      isOr,
	}, true
}

// buildBoolOpAssignModule emits the bytecode CPython 3.14 generates
// for `<target> = <left> <bool_op> <right>` at module scope.
//
// Layout (the IR encoder appends 3 cache words after TO_BOOL and 1
// cache word after the conditional jump):
//
//	RESUME 0
//	LOAD_NAME 0          ; left
//	COPY 1
//	TO_BOOL 0            ; encoder appends 3 cache words
//	POP_JUMP_IF_FALSE 3  ; (POP_JUMP_IF_TRUE for `or`); encoder appends 1 cache word
//	NOT_TAKEN 0
//	POP_TOP 0
//	LOAD_NAME 1          ; right
//	STORE_NAME 2         ; target
//	LOAD_CONST 0         ; None
//	RETURN_VALUE
//
// The forward-jump oparg is resolved at codegen time: 3 code units
// past the cache lands on STORE_NAME, mirroring the classifier
// helper byte-for-byte.
//
// co_consts = [nil].
// co_names  = [left, right, target] (insertion order matches the
// classifier).
// co.StackSize = 2 (COPY pushes a duplicate of left).
func buildBoolOpAssignModule(b boolOpAssignModule, opts Options) (*bytecode.CodeObject, error) {
	if b.TargetLen == 0 || b.TargetLen > 15 ||
		b.LeftLen == 0 || b.LeftLen > 15 ||
		b.RightLen == 0 || b.RightLen > 15 {
		return nil, errors.New("codegen.buildBoolOpAssignModule: name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	leftLoc := bytecode.Loc{
		Line: b.Line, EndLine: b.Line,
		Col: b.LeftCol, EndCol: b.LeftCol + b.LeftLen,
	}
	spanLoc := bytecode.Loc{
		Line: b.Line, EndLine: b.Line,
		Col: b.LeftCol, EndCol: b.RightCol + b.RightLen,
	}
	rightLoc := bytecode.Loc{
		Line: b.Line, EndLine: b.Line,
		Col: b.RightCol, EndCol: b.RightCol + b.RightLen,
	}
	targetLoc := bytecode.Loc{
		Line: b.Line, EndLine: b.Line,
		Col: 0, EndCol: b.TargetLen,
	}

	jumpOp := bytecode.POP_JUMP_IF_FALSE
	if b.IsOr {
		jumpOp = bytecode.POP_JUMP_IF_TRUE
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: leftLoc},
		ir.Instr{Op: bytecode.COPY, Arg: 1, Loc: spanLoc},
		ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: spanLoc},
		ir.Instr{Op: jumpOp, Arg: 3, Loc: spanLoc},
		ir.Instr{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: spanLoc},
		ir.Instr{Op: bytecode.POP_TOP, Arg: 0, Loc: spanLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 1, Loc: rightLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 2, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{b.LeftName, b.RightName, b.Target},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 2 {
		co.StackSize = 2
	}
	return co, nil
}
