package codegen

import (
	"strconv"

	"github.com/tamnd/gocopy/v1/ast"
)

// Source-text scanner helpers used by the v0.7.10 function-body
// visitor for per-instruction Loc resolution. They are pure
// functions over a pre-split source-line slice; the visitor is
// expected to call splitLines once at unit construction time and
// pass the resulting [][]byte into every call.
//
// These helpers were duplicated from compiler/func_body.go (the
// 1,516-line legacy classifier) so the codegen package gains its
// own end-column source of truth without depending on the funcState
// abstraction. The legacy func_body.go retains its own copies until
// v0.7.10 step 17 deletes the file; the duplication is short-lived
// and intentional.
//
// SOURCE: CPython 3.14's tokenizer-driven column tracking — the
// Python compiler doesn't have to scan source for end columns
// because the AST already carries end_col_offset. gocopy's gopapy
// AST does not, so the visitor recovers spans by scanning.

// scanLineByte returns the byte at column col on the 1-indexed
// source line, or (0, false) if the line or column is out of
// range. lines is the pre-split source from splitLines.
func scanLineByte(lines [][]byte, line int, col int) (byte, bool) {
	if line < 1 || line > len(lines) {
		return 0, false
	}
	ln := lines[line-1]
	if col < 0 || col >= len(ln) {
		return 0, false
	}
	return ln[col], true
}

// scanCharForward scans line forward from startCol looking for ch.
// Returns col+1 past the first occurrence, or startCol if not
// found. Mirrors compiler/func_body.go::funcState.scanChar.
func scanCharForward(lines [][]byte, line int, startCol byte, ch byte) byte {
	if line < 1 || line > len(lines) {
		return startCol
	}
	src := lines[line-1]
	for c := int(startCol); c < len(src); c++ {
		if src[c] == ch {
			return byte(c + 1)
		}
	}
	return startCol
}

// scanCallEnd finds the closing ')' of a call starting from
// startCol. Mirrors compiler/func_body.go::funcState.scanCallEnd.
func scanCallEnd(lines [][]byte, line int, startCol byte) byte {
	return scanCharForward(lines, line, startCol, ')')
}

// scanSubscriptEnd finds the closing ']' of a subscript.
// Mirrors compiler/func_body.go::funcState.scanSubscriptEnd.
func scanSubscriptEnd(lines [][]byte, line int, startCol byte) byte {
	return scanCharForward(lines, line, startCol, ']')
}

// scanBackOpen checks whether the byte immediately before col on
// the source line is '('. If so, returns col-1; otherwise returns
// col unchanged. Mirrors func_body.go::funcState.scanBackOpen.
func scanBackOpen(lines [][]byte, line int, col byte) byte {
	if col == 0 || line < 1 || line > len(lines) {
		return col
	}
	src := lines[line-1]
	if int(col) <= len(src) && src[col-1] == '(' {
		return col - 1
	}
	return col
}

// scanMatchingClose finds the closing ')' that matches the '(' at
// openCol. Returns the column just past the ')'. If no match is
// found, returns openCol.
// Mirrors func_body.go::funcState.scanMatchingClose.
func scanMatchingClose(lines [][]byte, line int, openCol byte) byte {
	if line < 1 || line > len(lines) {
		return openCol
	}
	src := lines[line-1]
	depth := 0
	for c := int(openCol); c < len(src); c++ {
		switch src[c] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return byte(c + 1)
			}
		}
	}
	return openCol
}

// scanWhitespaceClose scans line forward from col, skipping spaces
// and tabs. If the next non-whitespace byte is ')', returns the
// column past it; otherwise returns col unchanged. Used to extend
// linetable spans past the closing ')' of expressions like
// `not (a < b)`. Mirrors func_body.go::funcState.scanEndCol.
func scanWhitespaceClose(lines [][]byte, line int, col byte) byte {
	if line < 1 || line > len(lines) {
		return col
	}
	src := lines[line-1]
	c := int(col)
	for c < len(src) && (src[c] == ' ' || src[c] == '\t') {
		c++
	}
	if c < len(src) && src[c] == ')' {
		return byte(c + 1)
	}
	return col
}

// scanNumericTokenEnd scans line forward from startCol, consuming
// numeric-literal characters (digits, '.', '_', 'e', 'E', and
// '+'/'-' only as part of an exponent). Returns the column past
// the last consumed byte. Used to find float literal end columns.
// Mirrors func_body.go::funcState.scanTokenEnd.
func scanNumericTokenEnd(lines [][]byte, line int, startCol byte) byte {
	if line < 1 || line > len(lines) {
		return startCol
	}
	src := lines[line-1]
	c := int(startCol)
	for c < len(src) {
		ch := src[c]
		switch {
		case (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == 'e' || ch == 'E':
			c++
		case (ch == '+' || ch == '-') && c > 0 && (src[c-1] == 'e' || src[c-1] == 'E'):
			c++
		default:
			return byte(c)
		}
	}
	return byte(c)
}

// scanStringEnd returns the source column past the closing quote
// of the string literal that begins at startCol on the given source
// line. Handles single-, double-, and triple-quoted literals,
// including backslash escape sequences. Falls back to end-of-line
// on any parse error.
// Mirrors func_body.go::funcState.scanStringEnd.
func scanStringEnd(lines [][]byte, line int, startCol byte) byte {
	if line < 1 || line > len(lines) {
		return startCol
	}
	src := lines[line-1]
	c := int(startCol)
	if c >= len(src) {
		return startCol
	}
	q := src[c]
	if q != '"' && q != '\'' {
		return startCol
	}
	c++
	if c+1 < len(src) && src[c] == q && src[c+1] == q {
		c += 2
		for c < len(src)-2 {
			if src[c] == q && src[c+1] == q && src[c+2] == q {
				return byte(c + 3)
			}
			if src[c] == '\\' {
				c++
			}
			c++
		}
		return byte(len(src))
	}
	for c < len(src) {
		if src[c] == '\\' {
			c += 2
			continue
		}
		if src[c] == q {
			return byte(c + 1)
		}
		c++
	}
	return byte(len(src))
}

// astExprCol returns the start column of an expression node.
// Mirrors func_body.go::exprCol — kept under a longer name to
// reserve the short `exprCol` identifier for future use.
func astExprCol(e ast.Expr) byte {
	switch n := e.(type) {
	case *ast.Name:
		return byte(n.P.Col)
	case *ast.Constant:
		return byte(n.P.Col)
	case *ast.BinOp:
		return byte(n.P.Col)
	case *ast.UnaryOp:
		return byte(n.P.Col)
	case *ast.Compare:
		return byte(n.P.Col)
	case *ast.Call:
		return byte(n.P.Col)
	case *ast.Attribute:
		return byte(n.P.Col)
	case *ast.Subscript:
		return byte(n.P.Col)
	case *ast.Tuple:
		return byte(n.P.Col)
	case *ast.BoolOp:
		return byte(n.P.Col)
	}
	return 0
}

// astExprEndCol returns the end column of expression e by walking
// the AST and falling back to source scanning where the AST does
// not carry end-column information (string and float literals,
// calls). lines is the pre-split source.
// Mirrors func_body.go::funcState.exprEndCol.
func astExprEndCol(lines [][]byte, line int, e ast.Expr) byte {
	switch n := e.(type) {
	case *ast.Name:
		return byte(n.P.Col) + byte(len(n.Id))
	case *ast.Constant:
		switch n.Kind {
		case "int":
			return byte(n.P.Col) + byte(len(strconv.Itoa(int(n.Value.(int64)))))
		case "None":
			return byte(n.P.Col) + 4
		case "True":
			return byte(n.P.Col) + 4
		case "False":
			return byte(n.P.Col) + 5
		case "float":
			return scanNumericTokenEnd(lines, line, byte(n.P.Col))
		case "str":
			return scanStringEnd(lines, line, byte(n.P.Col))
		}
	case *ast.BinOp:
		return astExprEndCol(lines, line, n.Right)
	case *ast.UnaryOp:
		return astExprEndCol(lines, line, n.Operand)
	case *ast.Compare:
		return astExprEndCol(lines, line, n.Comparators[0])
	case *ast.Tuple:
		if len(n.Elts) > 0 {
			return astExprEndCol(lines, line, n.Elts[len(n.Elts)-1])
		}
	case *ast.Attribute:
		obj, ok := n.Value.(*ast.Name)
		if !ok {
			return 0
		}
		return byte(obj.P.Col) + byte(len(obj.Id)) + 1 + byte(len(n.Attr))
	case *ast.Subscript:
		return scanSubscriptEnd(lines, line, astExprEndCol(lines, line, n.Slice))
	case *ast.BoolOp:
		if len(n.Values) > 0 {
			return astExprEndCol(lines, line, n.Values[len(n.Values)-1])
		}
	case *ast.Call:
		var lastEnd byte
		if len(n.Args) > 0 {
			lastEnd = astExprEndCol(lines, line, n.Args[len(n.Args)-1])
		} else {
			switch fn := n.Func.(type) {
			case *ast.Name:
				lastEnd = byte(fn.P.Col) + byte(len(fn.Id))
			case *ast.Attribute:
				obj, ok := fn.Value.(*ast.Name)
				if !ok {
					return 0
				}
				lastEnd = byte(obj.P.Col) + byte(len(obj.Id)) + 1 + byte(len(fn.Attr))
			}
		}
		return scanCallEnd(lines, line, lastEnd)
	}
	return 0
}
