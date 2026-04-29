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
