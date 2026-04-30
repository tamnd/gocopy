package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
)

// codegenIf compiles an *ast.If statement inside a function
// body. It is the single entry point for *ast.If as of
// v0.7.10.2, replacing the per-shape emitFuncBodyIf and
// emitFuncBodyTerminatingIf helpers introduced in v0.7.10.
//
// MIRRORS: Python/codegen.c:2042 codegen_if (CPython 3.14).
//
// Deviation: CPython emits the test as
// `<test>; TO_BOOL; POP_JUMP_IF_FALSE next` and a flowgraph
// peephole later fuses TO_BOOL into the preceding COMPARE_OP
// by setting bit 4 of its oparg. gocopy does not yet have
// that fusion pass (lands in v0.7.10.6 with optimize_basic_block);
// codegenJumpIf therefore pre-fuses by emitting
// `COMPARE_OP (cmp<<5)|mask|16; POP_JUMP_IF_FALSE next` directly.
// Final bytecode is identical post-fusion.
//
// The terminating return reports whether every branch of this
// If chain (body + every elif + final else, when present) ends
// with an unconditional Return. When terminating=true the
// chain fully replaces the trailing function-body Return and
// lastEndCol/lastEndLine carry the deepest Return value's end
// for the outer LOAD_CONST/MAKE_FUNCTION/STORE_NAME Loc the
// caller emits. terminating=false means at least one branch
// falls through and the caller's trailing Return supplies the
// merge.
func codegenIf(u *compileUnit, ifs *ast.If, lines [][]byte) (lastEndCol uint16, lastEndLine int, terminating bool, err error) {
	cmp := ifs.Test.(*ast.Compare)
	falseLabel, condLoc, err := codegenJumpIf(u, cmp, lines)
	if err != nil {
		return 0, 0, false, err
	}

	thenBlock := u.Seq.AddBlock()
	thenBlock.Instrs = append(thenBlock.Instrs, ir.Instr{
		Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc,
	})

	bodyTerminates := false
	switch body := ifs.Body[0].(type) {
	case *ast.Return:
		if _, err := emitFuncBodyReturn(u, body, lines); err != nil {
			return 0, 0, false, err
		}
		bodyTerminates = true
	case *ast.Assign:
		if err := emitFuncBodyAssign(u, body, lines); err != nil {
			return 0, 0, false, err
		}
	default:
		return 0, 0, false, ErrNotImplemented
	}

	elseBlock := u.Seq.AddBlock()
	u.Seq.BindLabel(falseLabel, elseBlock)

	if len(ifs.Orelse) == 0 {
		return 0, 0, false, nil
	}

	switch first := ifs.Orelse[0].(type) {
	case *ast.If:
		c, line, elifTerminates, err := codegenIf(u, first, lines)
		if err != nil {
			return 0, 0, false, err
		}
		if bodyTerminates && elifTerminates {
			return c, line, true, nil
		}
		return 0, 0, false, nil
	case *ast.Return:
		c, err := emitFuncBodyReturn(u, first, lines)
		if err != nil {
			return 0, 0, false, err
		}
		if bodyTerminates {
			return c, first.P.Line, true, nil
		}
		return 0, 0, false, nil
	case *ast.Assign:
		if err := emitFuncBodyAssign(u, first, lines); err != nil {
			return 0, 0, false, err
		}
		return 0, 0, false, nil
	}
	return 0, 0, false, ErrNotImplemented
}

// codegenJumpIf emits the test-expression compilation for an
// If/While/Assert test. v0.7.10.2 ports the Compare-arm of
// CPython's codegen_jump_if (the only test shape the v0.7.10
// visitor surface accepts; BoolOp / NamedExpr / IfExp / UnaryOp
// arms land in v0.7.10.6 / v0.7.10.9 / v0.7.10.10).
//
// MIRRORS: Python/codegen.c:1884 codegen_jump_if — Compare arm
// (single-op single-comparator path).
//
// Returns the false-branch label that the caller binds to the
// else block, and condLoc, the source span of the comparison
// (left-start, right-end) used as the Loc of the NOT_TAKEN
// pseudo-op the caller emits at the head of the then block.
func codegenJumpIf(u *compileUnit, cmp *ast.Compare, lines [][]byte) (ir.LabelID, bytecode.Loc, error) {
	op, base, _ := cmpOpFromAstOp(cmp.Ops[0])
	lc, _, err := visitFuncExpr(u, cmp.Left, lines)
	if err != nil {
		return 0, bytecode.Loc{}, err
	}
	_, re, err := visitFuncExpr(u, cmp.Comparators[0], lines)
	if err != nil {
		return 0, bytecode.Loc{}, err
	}
	condLoc := bytecode.Loc{
		Line: uint32(cmp.P.Line), EndLine: uint32(cmp.P.Line),
		Col: lc, EndCol: re,
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: op, Arg: uint32(base + 16), Loc: condLoc,
	})
	falseLabel := u.Seq.AllocLabel()
	block.AddJump(bytecode.POP_JUMP_IF_FALSE, falseLabel, condLoc)
	return falseLabel, condLoc, nil
}

// validateIfStmt reports whether ifs is an If-statement shape
// codegenIf accepts. The predicate consolidates the v0.7.10
// validateFuncBodyIf (non-last position) and
// validateFuncBodyTerminatingIf (last position) into a single
// recursive checker.
//
// isLast=true allows the "terminating" form where Orelse is a
// non-empty single Return or a terminating elif chain.
// isLast=false restricts Orelse to empty or an elif chain
// (which itself must validate as non-last).
func validateIfStmt(ifs *ast.If, isLast bool) bool {
	if ifs.P.Col > 255 || len(ifs.Body) != 1 {
		return false
	}
	cmp, ok := ifs.Test.(*ast.Compare)
	if !ok || cmp.P.Col > 255 || len(cmp.Ops) != 1 || len(cmp.Comparators) != 1 {
		return false
	}
	op, _, ok := cmpOpFromAstOp(cmp.Ops[0])
	if !ok || op == bytecode.IS_OP {
		return false
	}
	if !validateFuncBodyAssignRHS(cmp.Left) || !validateFuncBodyAssignRHS(cmp.Comparators[0]) {
		return false
	}
	switch body := ifs.Body[0].(type) {
	case *ast.Return:
		if body.Value == nil || body.P.Col > 255 || !validateFuncBodyReturnValue(body.Value) {
			return false
		}
	case *ast.Assign:
		if !validateFuncBodyAssign(body) {
			return false
		}
		if isLast {
			return false
		}
	default:
		return false
	}
	switch len(ifs.Orelse) {
	case 0:
		return !isLast
	case 1:
		switch first := ifs.Orelse[0].(type) {
		case *ast.If:
			return validateIfStmt(first, isLast)
		case *ast.Return:
			if !isLast {
				return false
			}
			return first.Value != nil && first.P.Col <= 255 && validateFuncBodyReturnValue(first.Value)
		}
		return false
	}
	return false
}
