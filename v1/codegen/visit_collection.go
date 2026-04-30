package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
)

// visitListExpr emits IR for an *ast.List in value context. Empty
// `[]` emits BUILD_LIST 0; non-empty recurses every element via
// visitExpr and emits BUILD_LIST N. Returns the list's column span
// (open bracket → end-of-line) used by callers to anchor wrapping
// op Locs.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr_list.
func visitListExpr(u *compileUnit, l *ast.List, line uint32) (uint16, uint16, error) {
	col, end, err := collectionSpan(u, l.P.Col, line)
	if err != nil {
		return 0, 0, err
	}
	loc := bytecode.Loc{Line: line, EndLine: line, Col: col, EndCol: end}
	for _, e := range l.Elts {
		if _, _, err := visitExpr(u, e, line); err != nil {
			return 0, 0, err
		}
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BUILD_LIST, Arg: uint32(len(l.Elts)), Loc: loc,
	})
	return col, end, nil
}

// visitTupleExpr emits IR for an *ast.Tuple in value context. Empty
// `()` emits a deferred LOAD_CONST of bytecode.ConstTuple{}; the
// trailing addConst(nil) consumes co_consts[0] and finalizeDeferred
// rewrites the placeholder to the post-None slot. Non-empty recurses
// every element via visitExpr and emits BUILD_TUPLE N.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr_tuple.
func visitTupleExpr(u *compileUnit, t *ast.Tuple, line uint32) (uint16, uint16, error) {
	col, end, err := collectionSpan(u, t.P.Col, line)
	if err != nil {
		return 0, 0, err
	}
	loc := bytecode.Loc{Line: line, EndLine: line, Col: col, EndCol: end}
	if len(t.Elts) == 0 {
		emitDeferredConstLoad(u, bytecode.ConstTuple{}, loc)
		return col, end, nil
	}
	for _, e := range t.Elts {
		if _, _, err := visitExpr(u, e, line); err != nil {
			return 0, 0, err
		}
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BUILD_TUPLE, Arg: uint32(len(t.Elts)), Loc: loc,
	})
	return col, end, nil
}

// visitSetExpr emits IR for an *ast.Set in value context. Empty set
// has no literal form (`{}` is a dict) and is rejected. Non-empty
// recurses every element via visitExpr and emits BUILD_SET N.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr_set.
func visitSetExpr(u *compileUnit, s *ast.Set, line uint32) (uint16, uint16, error) {
	if len(s.Elts) == 0 {
		return 0, 0, ErrNotImplemented
	}
	col, end, err := collectionSpan(u, s.P.Col, line)
	if err != nil {
		return 0, 0, err
	}
	loc := bytecode.Loc{Line: line, EndLine: line, Col: col, EndCol: end}
	for _, e := range s.Elts {
		if _, _, err := visitExpr(u, e, line); err != nil {
			return 0, 0, err
		}
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BUILD_SET, Arg: uint32(len(s.Elts)), Loc: loc,
	})
	return col, end, nil
}

// visitDictExpr emits IR for an *ast.Dict in value context. Empty
// `{}` emits BUILD_MAP 0. Non-empty recurses every (key, value) pair
// via visitExpr and emits BUILD_MAP N (where N is the number of
// pairs). `**other` unpacking (a nil Key) is not supported.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_expr_dict.
func visitDictExpr(u *compileUnit, d *ast.Dict, line uint32) (uint16, uint16, error) {
	if len(d.Keys) != len(d.Values) {
		return 0, 0, ErrNotImplemented
	}
	col, end, err := collectionSpan(u, d.P.Col, line)
	if err != nil {
		return 0, 0, err
	}
	loc := bytecode.Loc{Line: line, EndLine: line, Col: col, EndCol: end}
	for i, k := range d.Keys {
		if k == nil {
			return 0, 0, ErrNotImplemented
		}
		if _, _, err := visitExpr(u, k, line); err != nil {
			return 0, 0, err
		}
		if _, _, err := visitExpr(u, d.Values[i], line); err != nil {
			return 0, 0, err
		}
	}
	u.currentBlock().Instrs = append(u.currentBlock().Instrs, ir.Instr{
		Op: bytecode.BUILD_MAP, Arg: uint32(len(d.Keys)), Loc: loc,
	})
	return col, end, nil
}

// collectionSpan returns the (openCol, lineEnd) span used as the
// build op's Loc. Mirrors the v0.6 classifier's openCol→closeEnd
// recipe: openCol is the literal's open bracket / paren / brace
// column and closeEnd is the source line's trimmed end column.
func collectionSpan(u *compileUnit, openCol int, line uint32) (uint16, uint16, error) {
	if line < 1 || openCol > 255 {
		return 0, 0, ErrNotImplemented
	}
	lines := splitLines(u.Source)
	end, ok := lineEndCol(lines, int(line))
	if !ok {
		return 0, 0, ErrNotImplemented
	}
	return uint16(openCol), uint16(end), nil
}
