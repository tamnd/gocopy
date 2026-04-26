package compiler

import "github.com/tamnd/gocopy/v1/bytecode"

// classify scans Python source and returns the body shape if it falls
// inside the v0.0.x supported set, plus ok=false otherwise. The
// supported shapes are:
//
//   - modEmpty: file contains only blank lines and comments.
//   - modNoOps: file contains N >= 1 no-op statements, each at
//     column 0, with arbitrary blank or comment-only lines anywhere
//     (leading, trailing, or between statements). The no-op set is
//     `pass`, the keyword constants None/True/False, the literal
//     `...`, a numeric literal, a non-leading string or bytes
//     literal, or a leading bytes literal (CPython drops the value
//     in all of these cases).
//   - modDocstring: file's first statement is a string literal that
//     CPython binds to `__doc__`, optionally followed by N >= 0
//     no-op statements.
//
// Trailing comments on the same line as a statement are allowed and
// excluded from the recorded end column. Statements cannot be
// indented and cannot start past column 0; CPython would raise
// IndentationError.

type modKind uint8

const (
	modUnsupported modKind = iota
	modEmpty
	modNoOps
	modDocstring
	modAssign
)

type classification struct {
	kind modKind
	// modNoOps: every statement, in source order.
	// modDocstring / modAssign: the no-op tail, after the leading
	// docstring or assignment.
	stmts []bytecode.NoOpStmt
	// modDocstring fields:
	docLine    int
	docEndLine int
	docCol     byte
	docText    string
	// modAssign fields:
	asgnLine     int
	asgnName     string
	asgnNameLen  byte
	asgnValStart byte
	asgnValEnd   byte
	asgnValue    any
}

// rawStmt is the parser's intermediate form: a no-op token, a string
// literal whose contents are captured in `text`, or a `name = literal`
// assignment with its name/value fields populated. Bytes literals
// collapse to stmtNoOp because CPython drops them. For a single-line
// statement endLine == line.
type rawStmt struct {
	line    int
	endLine int
	endCol  byte
	kind    rawStmtKind
	text    string
	// stmtAssign fields:
	asgnNameLen  byte
	asgnValStart byte
	asgnValEnd   byte
	asgnValue    any
}

type rawStmtKind uint8

const (
	stmtNoOp rawStmtKind = iota
	stmtString
	stmtAssign
)

func classify(src []byte) (classification, bool) {
	lines := splitLines(src)

	stmts := make([]rawStmt, 0, len(lines))
	for i := 0; i < len(lines); {
		ln := lines[i]
		if lineIsBlankOrComment(ln) {
			i++
			continue
		}
		if len(ln) > 0 && (ln[0] == ' ' || ln[0] == '\t') {
			return classification{}, false
		}
		// Multi-line triple-quoted string opens on this line and
		// continues across subsequent lines until the matching
		// closing triple. The single-line form (open and close on
		// the same line) falls through to the standard scanner.
		if multi, n, ok := tryConsumeMultilineString(lines, i); ok {
			stmts = append(stmts, multi)
			i = n
			continue
		}
		bare := stripLineComment(ln)
		bare = trimRight(bare)
		if len(bare) > 255 {
			return classification{}, false
		}
		// Assignment is only valid as the first statement in v0.0.7.
		// Subsequent assignments fall through to default (rejected).
		if len(stmts) == 0 {
			if a, ok := tryParseAssign(bare); ok {
				stmts = append(stmts, rawStmt{
					line:         i + 1,
					endLine:      i + 1,
					endCol:       byte(len(bare)),
					kind:         stmtAssign,
					text:         a.name,
					asgnNameLen:  a.nameLen,
					asgnValStart: a.valStart,
					asgnValEnd:   a.valEnd,
					asgnValue:    a.value,
				})
				i++
				continue
			}
		}
		text, isString, isStringOrBytes := parseStringOrBytes(bare)
		switch {
		case isStringOrBytes && isString:
			stmts = append(stmts, rawStmt{line: i + 1, endLine: i + 1, endCol: byte(len(bare)), kind: stmtString, text: text})
		case isStringOrBytes:
			stmts = append(stmts, rawStmt{line: i + 1, endLine: i + 1, endCol: byte(len(bare)), kind: stmtNoOp})
		case isNoOpStatement(bare):
			stmts = append(stmts, rawStmt{line: i + 1, endLine: i + 1, endCol: byte(len(bare)), kind: stmtNoOp})
		default:
			return classification{}, false
		}
		i++
	}
	if len(stmts) == 0 {
		return classification{kind: modEmpty}, true
	}
	if first := stmts[0]; first.kind == stmtString {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:       modDocstring,
			stmts:      tail,
			docLine:    first.line,
			docEndLine: first.endLine,
			docCol:     first.endCol,
			docText:    first.text,
		}, true
	}
	if first := stmts[0]; first.kind == stmtAssign {
		tail := make([]bytecode.NoOpStmt, 0, len(stmts)-1)
		for _, s := range stmts[1:] {
			tail = append(tail, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
		}
		return classification{
			kind:         modAssign,
			stmts:        tail,
			asgnLine:     first.line,
			asgnName:     first.text,
			asgnNameLen:  first.asgnNameLen,
			asgnValStart: first.asgnValStart,
			asgnValEnd:   first.asgnValEnd,
			asgnValue:    first.asgnValue,
		}, true
	}
	out := make([]bytecode.NoOpStmt, 0, len(stmts))
	for _, s := range stmts {
		out = append(out, bytecode.NoOpStmt{Line: s.line, EndCol: s.endCol})
	}
	return classification{kind: modNoOps, stmts: out}, true
}

// tryConsumeMultilineString reports whether `lines[i]` opens a
// triple-quoted string at column 0 that closes on a later line. On
// success it returns the assembled rawStmt (line range, end column
// on the close line, body text) and the next line index to resume
// from. The body must be plain ASCII with no backslashes and no
// occurrence of the matching quote character. A bytes prefix is
// honoured (the result becomes a no-op stmt). Single-line triple
// quotes return ok=false so the standard scanner picks them up.
func tryConsumeMultilineString(lines [][]byte, i int) (rawStmt, int, bool) {
	ln := lines[i]
	s := ln
	isBytes := false
	if len(s) > 0 && (s[0] == 'b' || s[0] == 'B') {
		isBytes = true
		s = s[1:]
	}
	if len(s) < 3 {
		return rawStmt{}, 0, false
	}
	q := s[0]
	if q != '"' && q != '\'' {
		return rawStmt{}, 0, false
	}
	if s[1] != q || s[2] != q {
		return rawStmt{}, 0, false
	}
	body := s[3:]
	// If the closing triple is on this same line, defer to the
	// single-line scanner.
	if hasTriple(body, q) {
		return rawStmt{}, 0, false
	}
	if !isPlainAscii(body, q) {
		return rawStmt{}, 0, false
	}
	parts := []string{string(body)}
	for j := i + 1; j < len(lines); j++ {
		next := lines[j]
		idx := indexTriple(next, q)
		if idx < 0 {
			if !isPlainAscii(next, q) {
				return rawStmt{}, 0, false
			}
			parts = append(parts, string(next))
			continue
		}
		head := next[:idx]
		if !isPlainAscii(head, q) {
			return rawStmt{}, 0, false
		}
		// The closing triple must end the line (no trailing junk
		// other than a comment). Anything past the close becomes a
		// new statement which we don't support today.
		tail := next[idx+3:]
		tail = stripLineComment(tail)
		tail = trimRight(tail)
		if len(tail) != 0 {
			return rawStmt{}, 0, false
		}
		parts = append(parts, string(head))
		text := joinNL(parts)
		if len(text) > 255 {
			return rawStmt{}, 0, false
		}
		endCol := idx + 3
		if endCol > 255 {
			return rawStmt{}, 0, false
		}
		kind := stmtString
		if isBytes {
			kind = stmtNoOp
			text = ""
		}
		return rawStmt{
			line:    i + 1,
			endLine: j + 1,
			endCol:  byte(endCol),
			kind:    kind,
			text:    text,
		}, j + 1, true
	}
	return rawStmt{}, 0, false
}

// hasTriple reports whether b contains three consecutive bytes equal
// to q anywhere.
func hasTriple(b []byte, q byte) bool {
	return indexTriple(b, q) >= 0
}

// indexTriple returns the index of the first qqq run in b, or -1.
func indexTriple(b []byte, q byte) int {
	for i := 0; i+2 < len(b); i++ {
		if b[i] == q && b[i+1] == q && b[i+2] == q {
			return i
		}
	}
	return -1
}

// joinNL joins parts with '\n'. Used to reconstruct a multi-line
// string body from per-source-line fragments.
func joinNL(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	n := len(parts) - 1
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			out = append(out, '\n')
		}
		out = append(out, p...)
	}
	return string(out)
}

// asgn is the parsed form of a single `name = literal` assignment line.
type asgn struct {
	name     string
	nameLen  byte
	valStart byte
	valEnd   byte
	value    any
}

// tryParseAssign recognises the v0.0.7 assignment grammar:
// `<identifier> = <literal>` at column 0, where literal is one of
// None, True, False, or a single- or triple-quoted plain-ASCII string
// literal. The identifier must be a Python name (letter/underscore
// followed by name chars) of length 1..15 (the SHORT0 entry's
// end_col-start_col field caps at 15). Any whitespace around `=` is
// allowed; trailing comments have already been stripped by the caller.
// Returns ok=false when the line does not match this grammar.
func tryParseAssign(s []byte) (asgn, bool) {
	if len(s) == 0 || !isIdentStart(s[0]) {
		return asgn{}, false
	}
	nameEnd := 1
	for nameEnd < len(s) && isIdentCont(s[nameEnd]) {
		nameEnd++
	}
	if nameEnd > 15 {
		return asgn{}, false
	}
	name := string(s[:nameEnd])
	if isReservedName(name) {
		return asgn{}, false
	}
	i := nameEnd
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i >= len(s) || s[i] != '=' {
		return asgn{}, false
	}
	if i+1 < len(s) && s[i+1] == '=' {
		// `==` is a comparison, not an assignment.
		return asgn{}, false
	}
	i++
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i >= len(s) {
		return asgn{}, false
	}
	valStart := i
	rhs := s[valStart:]
	var value any
	switch string(rhs) {
	case "None":
		value = nil
	case "True":
		value = true
	case "False":
		value = false
	default:
		text, isString, ok := parseStringOrBytes(rhs)
		if !ok || !isString {
			return asgn{}, false
		}
		value = text
	}
	return asgn{
		name:     name,
		nameLen:  byte(nameEnd),
		valStart: byte(valStart),
		valEnd:   byte(len(s)),
		value:    value,
	}, true
}

// isIdentStart reports whether b can start a Python identifier
// (ASCII letters and underscore only; non-ASCII identifiers are out
// of scope for v0.0.7).
func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isIdentCont reports whether b can continue a Python identifier
// (ASCII alphanumeric and underscore).
func isIdentCont(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

// isReservedName rejects identifiers that would parse as a constant
// or keyword. CPython raises SyntaxError on `None = 1` etc.; we want
// to fall through to the standard scanner (which then rejects them
// too) instead of producing a bogus assignment.
func isReservedName(s string) bool {
	switch s {
	case "None", "True", "False":
		return true
	}
	return false
}

// parseStringOrBytes recognises the v0.0.5 string-literal grammar:
// pure ASCII contents, no backslash escapes, no embedded matching
// quote, single or triple `"`/`'` quoting, optional `b`/`B` prefix
// for bytes. Returns (text, isString, ok). isString is true for
// string literals (docstring candidate) and false for bytes
// literals (always a no-op). ok is false when the line is not a
// recognised string or bytes literal.
func parseStringOrBytes(s []byte) (text string, isString bool, ok bool) {
	if len(s) == 0 {
		return "", false, false
	}
	isBytes := false
	if s[0] == 'b' || s[0] == 'B' {
		isBytes = true
		s = s[1:]
		if len(s) == 0 {
			return "", false, false
		}
	}
	q := s[0]
	if q != '"' && q != '\'' {
		return "", false, false
	}
	if len(s) >= 6 && s[1] == q && s[2] == q {
		if s[len(s)-1] != q || s[len(s)-2] != q || s[len(s)-3] != q {
			return "", false, false
		}
		body := s[3 : len(s)-3]
		if !isPlainAscii(body, q) {
			return "", false, false
		}
		return string(body), !isBytes, true
	}
	if len(s) < 2 || s[len(s)-1] != q {
		return "", false, false
	}
	body := s[1 : len(s)-1]
	if !isPlainAscii(body, q) {
		return "", false, false
	}
	return string(body), !isBytes, true
}

// isPlainAscii reports whether body is printable ASCII (0x20..0x7e)
// with no backslashes and no copies of the quote byte. This is the
// strict subset v0.0.5 will marshal as TYPE_SHORT_ASCII_INTERNED
// without needing to interpret escape sequences.
func isPlainAscii(body []byte, quote byte) bool {
	for _, c := range body {
		if c < 0x20 || c > 0x7e {
			return false
		}
		if c == '\\' || c == quote {
			return false
		}
	}
	return true
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
