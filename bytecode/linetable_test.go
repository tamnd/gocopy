package bytecode

import (
	"bytes"
	"testing"
)

// TestLineTableEmptyGolden locks the empty-module line table against the
// bytes python3.14 -m py_compile produces for an empty source file.
func TestLineTableEmptyGolden(t *testing.T) {
	t.Parallel()
	want := []byte{0xf2, 0x03, 0x01, 0x01, 0x01}
	got := LineTableEmpty()
	if !bytes.Equal(got, want) {
		t.Errorf("LineTableEmpty: want %x got %x", want, got)
	}
}

// TestLineTableSingleNoOpGolden covers the seven single-no-op fixtures
// shipped in v0.0.2 plus complex literals. Each pair was verified by
// running `compile(src, "x.py", "exec").co_linetable.hex()` on Python
// 3.14.4.
func TestLineTableSingleNoOpGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		endCol byte
		want   []byte
	}{
		{"pass / None / True", 4, []byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd9, 0x00, 0x04}},
		{"False", 5, []byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd9, 0x00, 0x05}},
		{"ellipsis literal", 3, []byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd9, 0x00, 0x03}},
		{"int 1", 1, []byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd9, 0x00, 0x01}},
		{"float 1.0", 3, []byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd9, 0x00, 0x03}},
		{"complex 1j", 2, []byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd9, 0x00, 0x02}},
	}
	for _, c := range cases {
		got := LineTableSingleNoOp(c.endCol)
		if !bytes.Equal(got, c.want) {
			t.Errorf("LineTableSingleNoOp(%q endCol=%d): want %x got %x",
				c.name, c.endCol, c.want, got)
		}
	}
}
