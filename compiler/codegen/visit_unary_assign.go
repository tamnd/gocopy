package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// unaryKindCG identifies which unary operator a modUnaryAssign uses.
type unaryKindCG uint8

const (
	unaryKindNeg unaryKindCG = iota
	unaryKindInvert
	unaryKindNot
)

// unaryAssignModule captures the modUnaryAssign shape: a single
// `<target> = <op> <operand>` statement where target and operand
// are both Names of 1..15 ASCII chars.
type unaryAssignModule struct {
	Line       uint32
	Target     string
	TargetLen  uint16
	Operand    string
	OperandCol uint16
	OperandLen uint16
	OpCol      uint16
	Kind       unaryKindCG
}

// classifyUnaryAssignModule recognizes a single-statement
// `<target> = <op> <operand>` body. To stay byte-identical with
// the classifier's compileUnaryAssign (which has no slot for
// trailing no-ops), the codegen path accepts only single-statement
// modules.
func classifyUnaryAssignModule(mod *ast.Module, _ []byte) (unaryAssignModule, bool) {
	if len(mod.Body) != 1 {
		return unaryAssignModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return unaryAssignModule{}, false
	}
	if len(a.Targets) != 1 {
		return unaryAssignModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return unaryAssignModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return unaryAssignModule{}, false
	}
	un, ok := a.Value.(*ast.UnaryOp)
	if !ok {
		return unaryAssignModule{}, false
	}
	operand, ok := un.Operand.(*ast.Name)
	if !ok {
		return unaryAssignModule{}, false
	}
	if n := len(operand.Id); n < 1 || n > 15 {
		return unaryAssignModule{}, false
	}
	if un.P.Col > 255 || operand.P.Col > 255 {
		return unaryAssignModule{}, false
	}
	var kind unaryKindCG
	switch un.Op {
	case "USub":
		kind = unaryKindNeg
	case "Invert":
		kind = unaryKindInvert
	case "Not":
		kind = unaryKindNot
	default:
		return unaryAssignModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return unaryAssignModule{}, false
	}
	return unaryAssignModule{
		Line:       uint32(line),
		Target:     target.Id,
		TargetLen:  uint16(len(target.Id)),
		Operand:    operand.Id,
		OperandCol: uint16(operand.P.Col),
		OperandLen: uint16(len(operand.Id)),
		OpCol:      uint16(un.P.Col),
		Kind:       kind,
	}, true
}

// buildUnaryAssignModule emits the bytecode CPython generates for
// `<target> = -<operand>`, `<target> = ~<operand>`, or
// `<target> = not <operand>`.
//
// For Neg / Invert:
//
//	RESUME 0
//	LOAD_NAME 0          ; operand
//	UNARY_NEGATIVE | UNARY_INVERT
//	STORE_NAME 1         ; target
//	LOAD_CONST 0         ; None
//	RETURN_VALUE
//
// For Not (TO_BOOL has 3 cache words appended by the IR encoder):
//
//	RESUME 0
//	LOAD_NAME 0          ; operand
//	TO_BOOL              ; encoder appends 3 cache words
//	UNARY_NOT
//	STORE_NAME 1         ; target
//	LOAD_CONST 0         ; None
//	RETURN_VALUE
//
// co_consts = [nil].
// co_names  = [operand, target].
// co.StackSize = 1.
func buildUnaryAssignModule(u unaryAssignModule, opts Options) (*bytecode.CodeObject, error) {
	if u.TargetLen == 0 || u.TargetLen > 15 ||
		u.OperandLen == 0 || u.OperandLen > 15 {
		return nil, errors.New("codegen.buildUnaryAssignModule: name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	operandLoc := bytecode.Loc{
		Line: u.Line, EndLine: u.Line,
		Col: u.OperandCol, EndCol: u.OperandCol + u.OperandLen,
	}
	opLoc := bytecode.Loc{
		Line: u.Line, EndLine: u.Line,
		Col: u.OpCol, EndCol: u.OperandCol + u.OperandLen,
	}
	targetLoc := bytecode.Loc{
		Line: u.Line, EndLine: u.Line,
		Col: 0, EndCol: u.TargetLen,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: operandLoc},
	)
	switch u.Kind {
	case unaryKindNeg:
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.UNARY_NEGATIVE, Arg: 0, Loc: opLoc},
		)
	case unaryKindInvert:
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.UNARY_INVERT, Arg: 0, Loc: opLoc},
		)
	case unaryKindNot:
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: opLoc},
			ir.Instr{Op: bytecode.UNARY_NOT, Arg: 0, Loc: opLoc},
		)
	default:
		return nil, errors.New("codegen.buildUnaryAssignModule: unknown unary kind")
	}
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 1, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{u.Operand, u.Target},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 1 {
		co.StackSize = 1
	}
	return co, nil
}
