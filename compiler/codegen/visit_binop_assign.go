package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// binOpAssignModule captures the modBinOpAssign shape: a single
// `<target> = <left> <op> <right>` statement where target, left,
// and right are all Names of 1..15 ASCII chars and <op> is one of
// the 13 BinOp operators the classifier supports.
type binOpAssignModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	LeftName  string
	LeftCol   uint16
	LeftLen   uint16
	RightName string
	RightCol  uint16
	RightLen  uint16
	Oparg     uint32
}

// classifyBinOpAssignModule recognizes a single-statement
// `<target> = <left> <op> <right>` body. To stay byte-identical
// with the classifier's compileBinOpAssign (which ignores any
// trailing no-op tail), the codegen path accepts only the
// single-statement case and falls through to the classifier
// otherwise.
func classifyBinOpAssignModule(mod *ast.Module, _ []byte) (binOpAssignModule, bool) {
	if len(mod.Body) != 1 {
		return binOpAssignModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return binOpAssignModule{}, false
	}
	if len(a.Targets) != 1 {
		return binOpAssignModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return binOpAssignModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return binOpAssignModule{}, false
	}
	bin, ok := a.Value.(*ast.BinOp)
	if !ok {
		return binOpAssignModule{}, false
	}
	left, ok := bin.Left.(*ast.Name)
	if !ok {
		return binOpAssignModule{}, false
	}
	right, ok := bin.Right.(*ast.Name)
	if !ok {
		return binOpAssignModule{}, false
	}
	if n := len(left.Id); n < 1 || n > 15 {
		return binOpAssignModule{}, false
	}
	if n := len(right.Id); n < 1 || n > 15 {
		return binOpAssignModule{}, false
	}
	if left.P.Col > 255 || right.P.Col > 255 {
		return binOpAssignModule{}, false
	}
	oparg, ok := binOpargFromOp(bin.Op)
	if !ok {
		return binOpAssignModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return binOpAssignModule{}, false
	}
	return binOpAssignModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		LeftName:  left.Id,
		LeftCol:   uint16(left.P.Col),
		LeftLen:   uint16(len(left.Id)),
		RightName: right.Id,
		RightCol:  uint16(right.P.Col),
		RightLen:  uint16(len(right.Id)),
		Oparg:     uint32(oparg),
	}, true
}

// binOpargFromOp maps an *ast.BinOp Op string to its CPython
// NB_* enum value for BINARY_OP. Mirrors compiler/classify_ast.go's
// helper of the same name.
func binOpargFromOp(op string) (byte, bool) {
	switch op {
	case "Add":
		return bytecode.NbAdd, true
	case "Sub":
		return bytecode.NbSubtract, true
	case "Mult":
		return bytecode.NbMultiply, true
	case "Div":
		return bytecode.NbTrueDivide, true
	case "FloorDiv":
		return bytecode.NbFloorDivide, true
	case "Mod":
		return bytecode.NbRemainder, true
	case "Pow":
		return bytecode.NbPower, true
	case "BitAnd":
		return bytecode.NbAnd, true
	case "BitOr":
		return bytecode.NbOr, true
	case "BitXor":
		return bytecode.NbXor, true
	case "LShift":
		return bytecode.NbLshift, true
	case "RShift":
		return bytecode.NbRshift, true
	case "MatMult":
		return bytecode.NbMatrixMultiply, true
	}
	return 0, false
}

// buildBinOpAssignModule emits the bytecode CPython generates for
// `<target> = <left> <op> <right>` at module scope.
//
// Layout:
//
//	RESUME 0
//	LOAD_NAME 0          ; left
//	LOAD_NAME 1          ; right
//	BINARY_OP <oparg>    ; encoder appends 5 cache words
//	STORE_NAME 2         ; target
//	LOAD_CONST 0         ; None
//	RETURN_VALUE
//
// co_consts = [nil].
// co_names  = [left, right, target] (insertion order).
// co.StackSize = 2 (left + right on stack at BINARY_OP).
func buildBinOpAssignModule(b binOpAssignModule, opts Options) (*bytecode.CodeObject, error) {
	if b.TargetLen == 0 || b.TargetLen > 15 ||
		b.LeftLen == 0 || b.LeftLen > 15 ||
		b.RightLen == 0 || b.RightLen > 15 {
		return nil, errors.New("codegen.buildBinOpAssignModule: name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	leftLoc := bytecode.Loc{
		Line: b.Line, EndLine: b.Line,
		Col: b.LeftCol, EndCol: b.LeftCol + b.LeftLen,
	}
	rightLoc := bytecode.Loc{
		Line: b.Line, EndLine: b.Line,
		Col: b.RightCol, EndCol: b.RightCol + b.RightLen,
	}
	binOpLoc := bytecode.Loc{
		Line: b.Line, EndLine: b.Line,
		Col: b.LeftCol, EndCol: b.RightCol + b.RightLen,
	}
	targetLoc := bytecode.Loc{
		Line: b.Line, EndLine: b.Line,
		Col: 0, EndCol: b.TargetLen,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: leftLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 1, Loc: rightLoc},
		ir.Instr{Op: bytecode.BINARY_OP, Arg: b.Oparg, Loc: binOpLoc},
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
