package compiler

// classify scans Python source and returns the body shape if it falls
// inside the v0.0.2 supported set, plus ok=false otherwise. The
// supported shapes are:
//
//   - modEmpty: file contains only blank lines and comments.
//   - modSingleNoOp: file contains exactly one no-op statement on
//     line 1 starting at column 0, optionally followed by trailing
//     blank or comment-only lines.
//
// Trailing comments on the same line as the statement are allowed and
// excluded from endCol. The statement cannot be indented and cannot
// start past column 0; CPython would raise IndentationError.

type modKind uint8

const (
	modUnsupported modKind = iota
	modEmpty
	modSingleNoOp
)

type classification struct {
	kind   modKind
	endCol int // 0-based column where the statement ends (exclusive); only valid for modSingleNoOp
}

func classify(src []byte) (classification, bool) {
	lines := splitLines(src)

	// Find the first non-blank, non-comment line.
	firstIdx := -1
	for i, ln := range lines {
		if !lineIsBlankOrComment(ln) {
			firstIdx = i
			break
		}
	}
	if firstIdx < 0 {
		return classification{kind: modEmpty}, true
	}

	// v0.0.2 only handles a statement on line 1 (index 0).
	if firstIdx != 0 {
		return classification{}, false
	}

	// The statement must start at column 0 (no indentation).
	stmt := lines[0]
	if len(stmt) > 0 && (stmt[0] == ' ' || stmt[0] == '\t') {
		return classification{}, false
	}

	// Strip a trailing line comment, then trailing whitespace, to get
	// the bare statement text and its end column.
	bare := stripLineComment(stmt)
	bare = trimRight(bare)
	if !isNoOpStatement(bare) {
		return classification{}, false
	}

	// Every subsequent line must be blank or comment-only. A second
	// real statement would be a multi-statement body, which is out of
	// scope.
	for _, ln := range lines[1:] {
		if !lineIsBlankOrComment(ln) {
			return classification{}, false
		}
	}

	return classification{kind: modSingleNoOp, endCol: len(bare)}, true
}

// splitLines splits src on '\n'. A trailing newline does NOT produce an
// empty trailing element; an absent trailing newline still yields the
// last line. Carriage returns inside a line are tolerated by the
// downstream blank/comment check (treated as whitespace).
func splitLines(src []byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	var out [][]byte
	start := 0
	for i := range len(src) {
		if src[i] == '\n' {
			out = append(out, src[start:i])
			start = i + 1
		}
	}
	if start < len(src) {
		out = append(out, src[start:])
	}
	return out
}

// lineIsBlankOrComment returns true iff the line, after trimming
// whitespace and stripping any trailing `# ...` comment, is empty.
func lineIsBlankOrComment(ln []byte) bool {
	for _, c := range ln {
		switch c {
		case ' ', '\t', '\r', '\f', '\v':
			continue
		case '#':
			return true
		default:
			return false
		}
	}
	return true
}

// stripLineComment returns ln with any unquoted `#` and everything
// after it removed. v0.0.2 only sees lines whose statement is one of
// the no-op tokens, none of which contain a `#` themselves, so the
// dumb scan is safe; once string literals are in scope this gets
// upgraded to track quote state.
func stripLineComment(ln []byte) []byte {
	for i := range len(ln) {
		if ln[i] == '#' {
			return ln[:i]
		}
	}
	return ln
}

// trimRight removes trailing ASCII whitespace from b.
func trimRight(b []byte) []byte {
	n := len(b)
	for n > 0 {
		c := b[n-1]
		if c != ' ' && c != '\t' && c != '\r' && c != '\f' && c != '\v' {
			break
		}
		n--
	}
	return b[:n]
}

// isNoOpStatement reports whether s is one of the bare tokens that
// CPython compiles to the same bytecode as an empty module: `pass`, a
// keyword constant (None/True/False), the literal `...`, or an integer
// / float / complex numeric literal.
func isNoOpStatement(s []byte) bool {
	switch string(s) {
	case "pass", "None", "True", "False", "...":
		return true
	}
	return isNumericLiteral(s)
}

// isNumericLiteral reports whether s is a Python numeric literal token
// (int, float, or complex) with no surrounding whitespace and no sign
// prefix. Underscores are allowed as digit separators per PEP 515.
//
// We deliberately keep this on the strict side: anything we do not
// recognise as a clean literal falls back to ErrUnsupportedSource so
// the oracle test never silently mismatches.
func isNumericLiteral(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	switch {
	case isHexLiteral(s), isOctLiteral(s), isBinLiteral(s):
		return true
	}
	return isDecimalOrFloatOrComplex(s)
}

func isHexLiteral(s []byte) bool {
	if len(s) < 3 || s[0] != '0' || (s[1] != 'x' && s[1] != 'X') {
		return false
	}
	return allHexDigitsOrUnderscore(s[2:])
}

func isOctLiteral(s []byte) bool {
	if len(s) < 3 || s[0] != '0' || (s[1] != 'o' && s[1] != 'O') {
		return false
	}
	return allOctDigitsOrUnderscore(s[2:])
}

func isBinLiteral(s []byte) bool {
	if len(s) < 3 || s[0] != '0' || (s[1] != 'b' && s[1] != 'B') {
		return false
	}
	return allBinDigitsOrUnderscore(s[2:])
}

func isDecimalOrFloatOrComplex(s []byte) bool {
	// Strip an optional trailing j/J for complex.
	if c := s[len(s)-1]; c == 'j' || c == 'J' {
		s = s[:len(s)-1]
		if len(s) == 0 {
			return false
		}
	}
	// Walk through optional integer part, optional fractional part,
	// and optional exponent. At least one digit must appear overall.
	i := 0
	hasDigit := false
	for i < len(s) && (isDigit(s[i]) || s[i] == '_') {
		if isDigit(s[i]) {
			hasDigit = true
		}
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && (isDigit(s[i]) || s[i] == '_') {
			if isDigit(s[i]) {
				hasDigit = true
			}
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		expStart := i
		for i < len(s) && (isDigit(s[i]) || s[i] == '_') {
			if isDigit(s[i]) {
				hasDigit = true
			}
			i++
		}
		if i == expStart {
			return false
		}
	}
	return i == len(s) && hasDigit
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func allHexDigitsOrUnderscore(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	hasDigit := false
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			hasDigit = true
		case c == '_':
			// allowed
		default:
			return false
		}
	}
	return hasDigit
}

func allOctDigitsOrUnderscore(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	hasDigit := false
	for _, c := range s {
		switch {
		case c >= '0' && c <= '7':
			hasDigit = true
		case c == '_':
			// allowed
		default:
			return false
		}
	}
	return hasDigit
}

func allBinDigitsOrUnderscore(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	hasDigit := false
	for _, c := range s {
		switch c {
		case '0', '1':
			hasDigit = true
		case '_':
			// allowed
		default:
			return false
		}
	}
	return hasDigit
}
