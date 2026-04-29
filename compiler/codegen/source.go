package codegen

// Source-text helpers used by module-level no-op classification.
// Mirrors compiler/classify.go's helpers verbatim — kept local here
// so the codegen package has no compiler-package dependency. When
// every shape moves into codegen the classifier copies retire.

// splitLines splits src on '\n'. A trailing newline does not produce
// an empty trailing element; an absent trailing newline still yields
// the last line.
func splitLines(src []byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	var out [][]byte
	start := 0
	for i := range src {
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

// stripLineComment returns ln with any unquoted `#` and everything
// after it removed.
func stripLineComment(ln []byte) []byte {
	for i := range ln {
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

// lineEndCol returns the 0-indexed exclusive end column for the
// given 1-indexed source line, after stripping trailing comments and
// whitespace. Lines longer than 255 bytes (the no-op encoder cap)
// fail the lookup.
func lineEndCol(lines [][]byte, line int) (int, bool) {
	if line < 1 || line > len(lines) {
		return 0, false
	}
	ln := trimRight(stripLineComment(lines[line-1]))
	if len(ln) > 255 {
		return 0, false
	}
	return len(ln), true
}

// isPlainAsciiNoEscape reports whether body is printable ASCII
// (0x20..0x7e) with no backslash. Mirrors compiler/classify.go's
// isPlainAscii with quote==0.
func isPlainAsciiNoEscape(body string) bool {
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c < 0x20 || c > 0x7e || c == '\\' {
			return false
		}
	}
	return true
}

// splitOnNewline splits s into segments at '\n'. A trailing newline
// produces an empty trailing segment, matching strings.Split(s,"\n").
func splitOnNewline(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// findTripleQuoteEnd scans raw source lines for the closing triple-
// quote of a string literal that opens on startLine (1-indexed).
// Handles backslash-escaped characters including \<newline>
// continuations. Returns the 1-indexed end line and exclusive end
// column of the closing delimiter, or (0, 0, false) if no triple
// quote opens on startLine. Mirrors compiler/classify_ast.go's
// helper of the same name.
func findTripleQuoteEnd(lines [][]byte, startLine int) (endLine int, endCol int, ok bool) {
	if startLine < 1 || startLine > len(lines) {
		return 0, 0, false
	}
	ln := lines[startLine-1]
	var q byte
	scanCol := -1
	for i := 0; i+2 < len(ln); i++ {
		if (ln[i] == '"' || ln[i] == '\'') && ln[i] == ln[i+1] && ln[i] == ln[i+2] {
			q = ln[i]
			scanCol = i + 3
			break
		}
	}
	if scanCol < 0 {
		return 0, 0, false
	}
	curLine := startLine
	col := scanCol
	for curLine <= len(lines) {
		b := lines[curLine-1]
		for col < len(b) {
			ch := b[col]
			if ch == '\\' {
				if col+1 < len(b) {
					col += 2
				} else {
					col = len(b)
				}
				continue
			}
			if col+2 < len(b) && b[col] == q && b[col+1] == q && b[col+2] == q {
				end := col + 3
				if end > 255 {
					return 0, 0, false
				}
				return curLine, end, true
			}
			col++
		}
		curLine++
		col = 0
	}
	return 0, 0, false
}
