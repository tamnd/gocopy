package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// visitCallExpr emits IR for an *ast.Call in value context: recurse
// the callee, emit PUSH_NULL (the non-method calling convention),
// recurse every positional argument in order, emit CALL N (the
// assembler appends 3 cache words). Keyword args, *args, and
// **kwargs are not yet supported.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr_call
// (positional-only path).
func visitCallExpr(u *compileUnit, c *ast.Call, line uint32) (uint16, uint16, error) {
	if len(c.Keywords) != 0 {
		return 0, 0, ErrNotImplemented
	}
	funcCol, funcEnd, err := visitExpr(u, c.Func, line)
	if err != nil {
		return 0, 0, err
	}
	funcLoc := bytecode.Loc{Line: line, EndLine: line, Col: funcCol, EndCol: funcEnd}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.PUSH_NULL, Arg: 0, Loc: funcLoc,
	})
	scanFrom := funcEnd
	for _, e := range c.Args {
		_, argEnd, err := visitExpr(u, e, line)
		if err != nil {
			return 0, 0, err
		}
		scanFrom = argEnd
	}
	closeEnd, err := callCloseEnd(u, line, scanFrom)
	if err != nil {
		return 0, 0, err
	}
	loc := bytecode.Loc{Line: line, EndLine: line, Col: funcCol, EndCol: closeEnd}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.CALL, Arg: uint32(len(c.Args)), Loc: loc,
	})
	return funcCol, closeEnd, nil
}

// callCloseEnd returns the exclusive end column of the call's `)` by
// scanning forward from `from` on the call's source line, tracking
// (), [] and {} nesting so an inner bracket pair doesn't terminate
// the outer call. Returns ErrNotImplemented if no matching `)` is
// found on the line.
func callCloseEnd(u *compileUnit, line uint32, from uint16) (uint16, error) {
	lines := splitLines(u.Source)
	if line < 1 || int(line) > len(lines) {
		return 0, ErrNotImplemented
	}
	src := lines[line-1]
	depth := 0
	for i := int(from); i < len(src); i++ {
		switch src[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				if src[i] != ')' {
					return 0, ErrNotImplemented
				}
				return uint16(i + 1), nil
			}
			depth--
		}
	}
	return 0, ErrNotImplemented
}
