package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// cmpAssignModule captures the modCmpAssign shape: a single
// `<target> = <left> <cmp> <right>` statement where target, left,
// and right are all Names of 1..15 ASCII chars and <cmp> is one of
// the ten comparison operators the classifier supports.
type cmpAssignModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	LeftName  string
	LeftCol   uint16
	LeftLen   uint16
	RightName string
	RightCol  uint16
	RightLen  uint16
	Op        bytecode.Opcode // COMPARE_OP / IS_OP / CONTAINS_OP
	Oparg     uint32
}

// classifyCmpAssignModule recognizes a single-statement
// `<target> = <left> <cmp> <right>` body. To stay byte-identical
// with the classifier's compileCmpAssign (which has no slot for
// trailing no-ops), the codegen path accepts only single-statement
// modules.
func classifyCmpAssignModule(mod *ast.Module, _ []byte) (cmpAssignModule, bool) {
	if len(mod.Body) != 1 {
		return cmpAssignModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return cmpAssignModule{}, false
	}
	if len(a.Targets) != 1 {
		return cmpAssignModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return cmpAssignModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return cmpAssignModule{}, false
	}
	cmp, ok := a.Value.(*ast.Compare)
	if !ok {
		return cmpAssignModule{}, false
	}
	if len(cmp.Ops) != 1 || len(cmp.Comparators) != 1 {
		return cmpAssignModule{}, false
	}
	left, ok := cmp.Left.(*ast.Name)
	if !ok {
		return cmpAssignModule{}, false
	}
	right, ok := cmp.Comparators[0].(*ast.Name)
	if !ok {
		return cmpAssignModule{}, false
	}
	if n := len(left.Id); n < 1 || n > 15 {
		return cmpAssignModule{}, false
	}
	if n := len(right.Id); n < 1 || n > 15 {
		return cmpAssignModule{}, false
	}
	if left.P.Col > 255 || right.P.Col > 255 {
		return cmpAssignModule{}, false
	}
	op, oparg, ok := cmpOpFromAstOp(cmp.Ops[0])
	if !ok {
		return cmpAssignModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return cmpAssignModule{}, false
	}
	return cmpAssignModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		LeftName:  left.Id,
		LeftCol:   uint16(left.P.Col),
		LeftLen:   uint16(len(left.Id)),
		RightName: right.Id,
		RightCol:  uint16(right.P.Col),
		RightLen:  uint16(len(right.Id)),
		Op:        op,
		Oparg:     uint32(oparg),
	}, true
}

// cmpOpFromAstOp maps an *ast.Compare op string to the (opcode,
// oparg) pair CPython 3.14 emits. Mirrors the classifier helper of
// the same shape in compiler/classify_ast.go::cmpOpFromOp.
func cmpOpFromAstOp(op string) (bytecode.Opcode, byte, bool) {
	switch op {
	case "Lt":
		return bytecode.COMPARE_OP, bytecode.CmpLt, true
	case "LtE":
		return bytecode.COMPARE_OP, bytecode.CmpLtE, true
	case "Eq":
		return bytecode.COMPARE_OP, bytecode.CmpEq, true
	case "NotEq":
		return bytecode.COMPARE_OP, bytecode.CmpNotEq, true
	case "Gt":
		return bytecode.COMPARE_OP, bytecode.CmpGt, true
	case "GtE":
		return bytecode.COMPARE_OP, bytecode.CmpGtE, true
	case "Is":
		return bytecode.IS_OP, 0, true
	case "IsNot":
		return bytecode.IS_OP, 1, true
	case "In":
		return bytecode.CONTAINS_OP, 0, true
	case "NotIn":
		return bytecode.CONTAINS_OP, 1, true
	}
	return 0, 0, false
}

// buildCmpAssignModule emits the bytecode CPython generates for
// `<target> = <left> <cmp> <right>` at module scope.
//
// Layout (the IR encoder appends CacheSize[<op>] cache words after
// the comparison instruction — 1 for COMPARE_OP / CONTAINS_OP, 0 for
// IS_OP):
//
//	RESUME 0
//	LOAD_NAME 0          ; left
//	LOAD_NAME 1          ; right
//	<op> <oparg>
//	STORE_NAME 2         ; target
//	LOAD_CONST 0         ; None
//	RETURN_VALUE
//
// co_consts = [nil].
// co_names  = [left, right, target] (insertion order matches the
// classifier).
// co.StackSize = 2.
func buildCmpAssignModule(c cmpAssignModule, opts Options) (*bytecode.CodeObject, error) {
	if c.TargetLen == 0 || c.TargetLen > 15 ||
		c.LeftLen == 0 || c.LeftLen > 15 ||
		c.RightLen == 0 || c.RightLen > 15 {
		return nil, errors.New("codegen.buildCmpAssignModule: name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	leftLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: c.LeftCol, EndCol: c.LeftCol + c.LeftLen,
	}
	rightLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: c.RightCol, EndCol: c.RightCol + c.RightLen,
	}
	cmpLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: c.LeftCol, EndCol: c.RightCol + c.RightLen,
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
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: leftLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 1, Loc: rightLoc},
		ir.Instr{Op: c.Op, Arg: c.Oparg, Loc: cmpLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 2, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{c.LeftName, c.RightName, c.Target},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 2 {
		co.StackSize = 2
	}
	return co, nil
}
