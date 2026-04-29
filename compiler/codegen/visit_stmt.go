package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
)

// docstringInfo carries the resolved Loc and string payload of a
// leading module docstring.
type docstringInfo struct {
	Loc  bytecode.Loc
	Text string
}

// isModuleDocstring reports whether stmt is the leading
// module-level docstring: an *ast.ExprStmt wrapping a string
// *ast.Constant whose value is plain printable ASCII (the slice the
// classifier and the v0.6 build*Module helpers accept). Returns the
// docstring's Loc and string payload on success.
//
// Mirrors compiler/codegen/visit_module.go::classifyDocstringModule's
// gating predicate at v0.6, kept structurally identical so the
// promotion at v0.7.1 is byte-equivalent.
func isModuleDocstring(stmt ast.Stmt, source []byte) (docstringInfo, bool) {
	es, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return docstringInfo{}, false
	}
	c, ok := es.Value.(*ast.Constant)
	if !ok || c.Kind != "str" {
		return docstringInfo{}, false
	}
	text, ok := c.Value.(string)
	if !ok {
		return docstringInfo{}, false
	}
	lines := splitLines(source)
	docLine := c.P.Line
	docEndLine, docEndCol, tripleOK := findTripleQuoteEnd(lines, docLine)
	if !tripleOK {
		// Plain "..." string. Embedded newlines must consume source
		// lines; if not, reject.
		n := 0
		for i := 0; i < len(text); i++ {
			if text[i] == '\n' {
				n++
			}
		}
		docEndLine = docLine + n
		if docEndLine > len(lines) {
			return docstringInfo{}, false
		}
		ec, ok := lineEndCol(lines, docEndLine)
		if !ok {
			return docstringInfo{}, false
		}
		docEndCol = ec
	}
	for _, seg := range splitOnNewline(text) {
		if !isPlainAsciiNoEscape(seg) {
			return docstringInfo{}, false
		}
	}
	return docstringInfo{
		Loc: bytecode.Loc{
			Line:    uint32(docLine),
			EndLine: uint32(docEndLine),
			Col:     0,
			EndCol:  uint16(docEndCol),
		},
		Text: text,
	}, true
}

// stmtNopLoc returns the Loc to attach to a single-instruction NOP
// emitted for a no-op-shaped statement. Supported shapes:
//
//   - *ast.Pass — emits NOP at the keyword's line.
//   - *ast.ExprStmt whose value is a non-string *ast.Constant of a
//     simple kind (None, True, False, Ellipsis, int, float, complex,
//     bytes). String constants in non-leading position would need
//     full literal parsing the v0.7.1 visitor doesn't have, so they
//     return ErrNotImplemented and bubble up to the classifier.
//
// Mirrors CPython 3.14 Python/codegen.c::codegen_visit_stmt_pass +
// the constant branch of codegen_visit_stmt_expr.
func stmtNopLoc(stmt ast.Stmt, source []byte) (bytecode.Loc, error) {
	var line int
	switch s := stmt.(type) {
	case *ast.Pass:
		line = s.P.Line
	case *ast.ExprStmt:
		c, ok := s.Value.(*ast.Constant)
		if !ok {
			return bytecode.Loc{}, ErrNotImplemented
		}
		switch c.Kind {
		case "None", "True", "False", "Ellipsis",
			"int", "float", "complex", "bytes":
			line = s.P.Line
		default:
			return bytecode.Loc{}, ErrNotImplemented
		}
	default:
		return bytecode.Loc{}, ErrNotImplemented
	}
	lines := splitLines(source)
	ec, ok := lineEndCol(lines, line)
	if !ok {
		return bytecode.Loc{}, ErrNotImplemented
	}
	return bytecode.Loc{
		Line:    uint32(line),
		EndLine: uint32(line),
		Col:     0,
		EndCol:  uint16(ec),
	}, nil
}
