package bytecode

import (
	"bytes"
	"testing"
)

// TestDocstringBytecodeGolden mirrors `compile(src, "x.py", "exec").co_code`
// on Python 3.14.4.
func TestDocstringBytecodeGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		tailStmts int
		want      []byte
	}{
		{
			"docstring only",
			0,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x52, 1, 0x23, 0},
		},
		{
			"docstring + one tail (no NOP, last absorbs LC/RET)",
			1,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x52, 1, 0x23, 0},
		},
		{
			"docstring + two tail (one NOP)",
			2,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x1b, 0, 0x52, 1, 0x23, 0},
		},
		{
			"docstring + three tail (two NOPs)",
			3,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x1b, 0, 0x1b, 0, 0x52, 1, 0x23, 0},
		},
	}
	for _, c := range cases {
		got := DocstringBytecode(c.tailStmts)
		if !bytes.Equal(got, c.want) {
			t.Errorf("DocstringBytecode(%d): want %x got %x", c.tailStmts, c.want, got)
		}
	}
}

// TestDocstringLineTableGolden covers the entries CPython emits for a
// docstring on its own, a docstring followed by a single-line tail,
// and a docstring on line 2 (leading blank).
func TestDocstringLineTableGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		docLine   int
		docEndCol byte
		tail      []NoOpStmt
		want      []byte
	}{
		{
			"docstring only line 1",
			1, 4, nil,
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xdb, 0x00, 0x04},
		},
		{
			"triple-quoted docstring only line 1",
			1, 8, nil,
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xdb, 0x00, 0x08},
		},
		{
			"docstring + pass",
			1, 4, []NoOpStmt{{Line: 2, EndCol: 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd9, 0x00, 0x04, 0xd9, 0x00, 0x04},
		},
		{
			"docstring + blank + pass",
			1, 4, []NoOpStmt{{Line: 3, EndCol: 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd9, 0x00, 0x04, 0xe1, 0x00, 0x04},
		},
		{
			"docstring on line 2 only (leading blank)",
			2, 4, nil,
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xe3, 0x00, 0x04},
		},
	}
	for _, c := range cases {
		got := DocstringLineTable(c.docLine, c.docEndCol, c.tail)
		if !bytes.Equal(got, c.want) {
			t.Errorf("DocstringLineTable(%s): want %x got %x", c.name, c.want, got)
		}
	}
}
