package bytecode

import (
	"bytes"
	"testing"
)

// TestAssignBytecodeGolden mirrors `compile(src, "x.py", "exec").co_code`
// for `name = literal` modules with and without no-op tails on Python
// 3.14.4.
func TestAssignBytecodeGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		noneIdx   byte
		tailStmts int
		want      []byte
	}{
		{
			"x = None (consts (None,))",
			0, 0,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x52, 0, 0x23, 0},
		},
		{
			"x = True (consts (True, None))",
			1, 0,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x52, 1, 0x23, 0},
		},
		{
			"x = None + one tail (no NOP, last absorbs LC/RET)",
			0, 1,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x52, 0, 0x23, 0},
		},
		{
			"x = None + two tail (one NOP)",
			0, 2,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x1b, 0, 0x52, 0, 0x23, 0},
		},
		{
			"x = True + three tail (two NOPs)",
			1, 3,
			[]byte{0x80, 0, 0x52, 0, 0x74, 0, 0x1b, 0, 0x1b, 0, 0x52, 1, 0x23, 0},
		},
	}
	for _, c := range cases {
		got := AssignBytecode(c.noneIdx, c.tailStmts)
		if !bytes.Equal(got, c.want) {
			t.Errorf("AssignBytecode(%s): want %x got %x", c.name, c.want, got)
		}
	}
}

// TestAssignLineTableGolden covers the entries CPython emits for a
// `name = literal` module: prologue + ONE_LINE1 LOAD_CONST entry +
// SHORT0 STORE_NAME entry, plus the v0.0.4 no-op tail rule.
func TestAssignLineTableGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		line        int
		nameLen     byte
		valStartCol byte
		valEndCol   byte
		tail        []NoOpStmt
		want        []byte
	}{
		{
			"x = None on line 1",
			1, 1, 4, 8, nil,
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x04, 0x08, 0x82, 0x01},
		},
		{
			"xx = None on line 1",
			1, 2, 5, 9, nil,
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x05, 0x09, 0x82, 0x02},
		},
		{
			"longername = None on line 1",
			1, 10, 13, 17, nil,
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x0d, 0x11, 0x82, 0x0a},
		},
		{
			"x = True on line 1",
			1, 1, 4, 8, nil,
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x04, 0x08, 0x82, 0x01},
		},
		{
			"x = None + pass on line 2",
			1, 1, 4, 8, []NoOpStmt{{Line: 2, EndCol: 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x04, 0x08, 0x80, 0x01, 0xd9, 0x00, 0x04},
		},
		{
			"x = None + blank + pass on line 3",
			1, 1, 4, 8, []NoOpStmt{{Line: 3, EndCol: 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x04, 0x08, 0x80, 0x01, 0xe1, 0x00, 0x04},
		},
		{
			"x = None + two pass (line 2, line 3)",
			1, 1, 4, 8, []NoOpStmt{{Line: 2, EndCol: 4}, {Line: 3, EndCol: 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x04, 0x08, 0x80, 0x01, 0xd8, 0x00, 0x04, 0xd9, 0x00, 0x04},
		},
		{
			"x = None on line 2 (leading blank)",
			2, 1, 4, 8, nil,
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xe0, 0x04, 0x08, 0x82, 0x01},
		},
	}
	for _, c := range cases {
		got := AssignLineTable(c.line, c.nameLen, c.valStartCol, c.valEndCol, c.tail)
		if !bytes.Equal(got, c.want) {
			t.Errorf("AssignLineTable(%s): want %x got %x", c.name, c.want, got)
		}
	}
}
