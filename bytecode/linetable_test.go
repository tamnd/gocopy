package bytecode

import (
	"bytes"
	"testing"
)

func TestLineTableEmptyGolden(t *testing.T) {
	t.Parallel()
	want := []byte{0xf2, 0x03, 0x01, 0x01, 0x01}
	got := LineTableEmpty()
	if !bytes.Equal(got, want) {
		t.Errorf("LineTableEmpty: want %x got %x", want, got)
	}
}

// TestLineTableSingleNoOpGolden covers the seven single-no-op fixtures
// shipped in v0.0.2 plus complex literals. Every expectation came from
// `compile(src, "x.py", "exec").co_linetable.hex()` on Python 3.14.4.
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

// TestLineTableNoOpsGolden covers consecutive (gap=0 → ONE_LINE1) and
// gapped (ONE_LINE2 / LONG) bodies. Every expectation comes from
// `compile(src, "x.py", "exec").co_linetable.hex()` on Python 3.14.4.
func TestLineTableNoOpsGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		stmts []NoOpStmt
		want  []byte
	}{
		{
			"pass\\npass (consec)",
			[]NoOpStmt{{1, 4}, {2, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x00, 0x04, 0xd9, 0x00, 0x04},
		},
		{
			"three pass consec",
			[]NoOpStmt{{1, 4}, {2, 4}, {3, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x00, 0x04, 0xd8, 0x00, 0x04, 0xd9, 0x00, 0x04},
		},
		{
			"pass blank pass (gap=1, ONE_LINE2)",
			[]NoOpStmt{{1, 4}, {3, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x00, 0x04, 0xe1, 0x00, 0x04},
		},
		{
			"pass _ _ pass (gap=2, LONG line_delta=+3)",
			[]NoOpStmt{{1, 4}, {4, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x00, 0x04, 0xf1, 0x06, 0x00, 0x01, 0x05},
		},
		{
			"pass _ _ _ pass (gap=3, LONG line_delta=+4)",
			[]NoOpStmt{{1, 4}, {5, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x00, 0x04, 0xf1, 0x08, 0x00, 0x01, 0x05},
		},
		{
			"pass _ _ _ _ _ _ _ _ _ _ pass (gap=10, LONG line_delta=+11)",
			[]NoOpStmt{{1, 4}, {12, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x00, 0x04, 0xf1, 0x16, 0x00, 0x01, 0x05},
		},
		{
			"leading blank: pass on line 3 (only one stmt, LONG line_delta=+3)",
			[]NoOpStmt{{3, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xf1, 0x06, 0x00, 0x01, 0x05},
		},
		{
			"three stmts mixed gaps: 1, 3 (gap1 ONE_LINE2), 4 (gap0 ONE_LINE1)",
			[]NoOpStmt{{1, 4}, {3, 4}, {4, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x00, 0x04, 0xe0, 0x00, 0x04, 0xd9, 0x00, 0x04},
		},
		{
			"three stmts: 1, 2, 4 (consec then gap1)",
			[]NoOpStmt{{1, 4}, {2, 4}, {4, 4}},
			[]byte{0xf0, 0x03, 0x01, 0x01, 0x01, 0xd8, 0x00, 0x04, 0xd8, 0x00, 0x04, 0xe1, 0x00, 0x04},
		},
	}
	for _, c := range cases {
		got := LineTableNoOps(c.stmts)
		if !bytes.Equal(got, c.want) {
			t.Errorf("LineTableNoOps(%s): want %x got %x", c.name, c.want, got)
		}
	}
}

func TestNoOpBytecodeGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		n    int
		want []byte
	}{
		{1, []byte{0x80, 0, 0x52, 0, 0x23, 0}},
		{2, []byte{0x80, 0, 0x1b, 0, 0x52, 0, 0x23, 0}},
		{3, []byte{0x80, 0, 0x1b, 0, 0x1b, 0, 0x52, 0, 0x23, 0}},
		{5, []byte{0x80, 0, 0x1b, 0, 0x1b, 0, 0x1b, 0, 0x1b, 0, 0x52, 0, 0x23, 0}},
	}
	for _, c := range cases {
		got := NoOpBytecode(c.n)
		if !bytes.Equal(got, c.want) {
			t.Errorf("NoOpBytecode(%d): want %x got %x", c.n, c.want, got)
		}
	}
}

func TestVarintRoundTrips(t *testing.T) {
	t.Parallel()
	cases := []struct {
		x    uint
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{63, []byte{0x3f}},
		{64, []byte{0x40, 0x01}},
		{200, []byte{0x48, 0x03}},
	}
	for _, c := range cases {
		got := appendVarint(nil, c.x)
		if !bytes.Equal(got, c.want) {
			t.Errorf("appendVarint(%d): want %x got %x", c.x, c.want, got)
		}
	}
}

func TestSignedVarintRoundTrips(t *testing.T) {
	t.Parallel()
	cases := []struct {
		x    int
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x02}},
		{-1, []byte{0x03}},
		{3, []byte{0x06}},
		{-2, []byte{0x05}},
		{11, []byte{0x16}},
	}
	for _, c := range cases {
		got := appendSignedVarint(nil, c.x)
		if !bytes.Equal(got, c.want) {
			t.Errorf("appendSignedVarint(%d): want %x got %x", c.x, c.want, got)
		}
	}
}
