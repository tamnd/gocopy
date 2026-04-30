package compiler

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
)

func TestEmptyModule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  []byte
	}{
		{"zero bytes", nil},
		{"only whitespace", []byte("   \n\t\n")},
		{"only comments", []byte("# hello\n# world\n")},
		{"mixed comment and whitespace", []byte("\n# c1\n\n  # c2\n\n")},
		{"trailing whitespace no newline", []byte("   ")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if c == nil {
				t.Fatal("expected CodeObject, got nil")
			}
			if c.Name != "<module>" || c.QualName != "<module>" {
				t.Errorf("name/qualname = %q/%q; want <module>/<module>", c.Name, c.QualName)
			}
			if c.Filename != "x.py" {
				t.Errorf("filename = %q; want x.py", c.Filename)
			}
			if !bytes.Equal(c.Bytecode, bytecode.NoOpBytecode(1)) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, bytecode.NoOpBytecode(1))
			}
			if len(c.Consts) != 1 || c.Consts[0] != nil {
				t.Errorf("consts = %v; want (None,)", c.Consts)
			}
			if c.StackSize != 1 {
				t.Errorf("stacksize = %d; want 1", c.StackSize)
			}
			if c.Flags != 0 {
				t.Errorf("flags = 0x%x; want 0", c.Flags)
			}
			if !bytes.Equal(c.LineTable, bytecode.LineTableEmpty()) {
				t.Errorf("linetable = %x; want empty %x", c.LineTable, bytecode.LineTableEmpty())
			}
		})
	}
}

func TestSingleNoOpStatement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		src    []byte
		endCol byte
	}{
		{"pass", []byte("pass\n"), 4},
		{"None", []byte("None\n"), 4},
		{"True", []byte("True\n"), 4},
		{"False", []byte("False\n"), 5},
		{"ellipsis literal", []byte("...\n"), 3},
		{"int 1", []byte("1\n"), 1},
		{"hex int", []byte("0xff\n"), 4},
		{"float 1.0", []byte("1.0\n"), 3},
		{"float exp", []byte("1e3\n"), 3},
		{"complex 1j", []byte("1j\n"), 2},
		{"pass with trailing comment", []byte("pass # bye\n"), 4},
		{"pass no trailing newline", []byte("pass"), 4},
		{"pass with trailing blank", []byte("pass\n\n"), 4},
		{"pass with trailing comment-only line", []byte("pass\n# done\n"), 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			want := bytecode.LineTableSingleNoOp(tc.endCol)
			if !bytes.Equal(c.LineTable, want) {
				t.Errorf("linetable = %x; want %x", c.LineTable, want)
			}
			if !bytes.Equal(c.Bytecode, bytecode.NoOpBytecode(1)) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, bytecode.NoOpBytecode(1))
			}
			if len(c.Consts) != 1 || c.Consts[0] != nil {
				t.Errorf("consts = %v; want (None,)", c.Consts)
			}
		})
	}
}

func TestMultiNoOpStatements(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		src   []byte
		stmts []bytecode.NoOpStmt
	}{
		{"two pass", []byte("pass\npass\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 2, EndCol: 4}}},
		{"three pass", []byte("pass\npass\npass\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 2, EndCol: 4}, {Line: 3, EndCol: 4}}},
		{"None False", []byte("None\nFalse\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 2, EndCol: 5}}},
		{"int int", []byte("1\n2\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 1}, {Line: 2, EndCol: 1}}},
		{"five mixed", []byte("pass\nNone\nTrue\nFalse\n...\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 2, EndCol: 4}, {Line: 3, EndCol: 4}, {Line: 4, EndCol: 5}, {Line: 5, EndCol: 3}}},
		{"two pass trailing comments", []byte("pass # a\npass\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 2, EndCol: 4}}},
		{"pass blank pass", []byte("pass\n\npass\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 3, EndCol: 4}}},
		{"pass two blanks pass", []byte("pass\n\n\n\npass\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 5, EndCol: 4}}},
		{"pass comment pass", []byte("pass\n# gap\npass\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 3, EndCol: 4}}},
		{"leading blank single pass", []byte("\n\npass\n"), []bytecode.NoOpStmt{{Line: 3, EndCol: 4}}},
		{"leading comment single None", []byte("# header\nNone\n"), []bytecode.NoOpStmt{{Line: 2, EndCol: 4}}},
		{"mixed gaps three stmts", []byte("pass\n\nNone\nTrue\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 3, EndCol: 4}, {Line: 4, EndCol: 4}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			n := len(tc.stmts)
			if !bytes.Equal(c.Bytecode, bytecode.NoOpBytecode(n)) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, bytecode.NoOpBytecode(n))
			}
			if !bytes.Equal(c.LineTable, bytecode.LineTableNoOps(tc.stmts)) {
				t.Errorf("linetable = %x; want %x", c.LineTable, bytecode.LineTableNoOps(tc.stmts))
			}
		})
	}
}

func TestDocstringModule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		src        []byte
		docLine    int
		docEndLine int
		docEndCol  byte
		docText    string
		tail       []bytecode.NoOpStmt
	}{
		{"plain double", []byte("\"hi\"\n"), 1, 1, 4, "hi", nil},
		{"plain single", []byte("'hi'\n"), 1, 1, 4, "hi", nil},
		{"triple double", []byte("\"\"\"hi\"\"\"\n"), 1, 1, 8, "hi", nil},
		{"triple single", []byte("'''hi'''\n"), 1, 1, 8, "hi", nil},
		{"docstring then pass", []byte("\"hi\"\npass\n"), 1, 1, 4, "hi",
			[]bytecode.NoOpStmt{{Line: 2, EndCol: 4}}},
		{"docstring then None", []byte("\"hi\"\nNone\n"), 1, 1, 4, "hi",
			[]bytecode.NoOpStmt{{Line: 2, EndCol: 4}}},
		{"docstring blank pass", []byte("\"hi\"\n\npass\n"), 1, 1, 4, "hi",
			[]bytecode.NoOpStmt{{Line: 3, EndCol: 4}}},
		{"leading blank docstring", []byte("\n\"hi\"\n"), 2, 2, 4, "hi", nil},
		{"empty docstring", []byte("\"\"\n"), 1, 1, 2, "", nil},
		{"longer ascii", []byte("\"hello world\"\n"), 1, 1, 13, "hello world", nil},
		{"two-line triple", []byte("\"\"\"a\nb\"\"\"\n"), 1, 2, 4, "a\nb", nil},
		{"three-line triple", []byte("\"\"\"a\nb\nc\"\"\"\n"), 1, 3, 4, "a\nb\nc", nil},
		{"two-line triple + tail", []byte("\"\"\"a\nb\"\"\"\npass\n"), 1, 2, 4, "a\nb",
			[]bytecode.NoOpStmt{{Line: 3, EndCol: 4}}},
		{"two-line triple single quotes", []byte("'''a\nb'''\n"), 1, 2, 4, "a\nb", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			wantBC := bytecode.DocstringBytecode(len(tc.tail))
			if !bytes.Equal(c.Bytecode, wantBC) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, wantBC)
			}
			wantLT := bytecode.DocstringLineTable(tc.docLine, tc.docEndLine, tc.docEndCol, tc.tail)
			if !bytes.Equal(c.LineTable, wantLT) {
				t.Errorf("linetable = %x; want %x", c.LineTable, wantLT)
			}
			if len(c.Consts) != 2 || c.Consts[0] != tc.docText || c.Consts[1] != nil {
				t.Errorf("consts = %v; want (%q, None)", c.Consts, tc.docText)
			}
			if len(c.Names) != 1 || c.Names[0] != "__doc__" {
				t.Errorf("names = %v; want (__doc__,)", c.Names)
			}
		})
	}
}

// TestBytesAndNonLeadingStringNoOps covers the cases where a string
// or bytes literal compiles to the no-op bytecode path.
func TestBytesAndNonLeadingStringNoOps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		src   []byte
		stmts []bytecode.NoOpStmt
	}{
		{"bytes only", []byte("b\"hi\"\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 5}}},
		{"bytes single quoted", []byte("b'x'\n"), []bytecode.NoOpStmt{{Line: 1, EndCol: 4}}},
		{"pass then string", []byte("pass\n\"hi\"\n"),
			[]bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 2, EndCol: 4}}},
		{"pass then bytes", []byte("pass\nb\"hi\"\n"),
			[]bytecode.NoOpStmt{{Line: 1, EndCol: 4}, {Line: 2, EndCol: 5}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			n := len(tc.stmts)
			if !bytes.Equal(c.Bytecode, bytecode.NoOpBytecode(n)) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, bytecode.NoOpBytecode(n))
			}
			if !bytes.Equal(c.LineTable, bytecode.LineTableNoOps(tc.stmts)) {
				t.Errorf("linetable = %x; want %x", c.LineTable, bytecode.LineTableNoOps(tc.stmts))
			}
			if len(c.Consts) != 1 || c.Consts[0] != nil {
				t.Errorf("consts = %v; want (None,)", c.Consts)
			}
			if len(c.Names) != 0 {
				t.Errorf("names = %v; want ()", c.Names)
			}
		})
	}
}

func TestAssignModule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		src         []byte
		asgnLine    int
		asgnName    string
		nameLen     byte
		valStartCol byte
		valEndCol   byte
		value       any
		tail        []bytecode.NoOpStmt
	}{
		{"x = None", []byte("x = None\n"), 1, "x", 1, 4, 8, nil, nil},
		{"x = True", []byte("x = True\n"), 1, "x", 1, 4, 8, true, nil},
		{"x = False", []byte("x = False\n"), 1, "x", 1, 4, 9, false, nil},
		{"x = \"hi\"", []byte("x = \"hi\"\n"), 1, "x", 1, 4, 8, "hi", nil},
		{"x = 'hi'", []byte("x = 'hi'\n"), 1, "x", 1, 4, 8, "hi", nil},
		{"xx = None", []byte("xx = None\n"), 1, "xx", 2, 5, 9, nil, nil},
		{"longername = None", []byte("longername = None\n"), 1, "longername", 10, 13, 17, nil, nil},
		{"x = None + pass", []byte("x = None\npass\n"), 1, "x", 1, 4, 8, nil,
			[]bytecode.NoOpStmt{{Line: 2, EndCol: 4}}},
		{"x = True + blank + pass", []byte("x = True\n\npass\n"), 1, "x", 1, 4, 8, true,
			[]bytecode.NoOpStmt{{Line: 3, EndCol: 4}}},
		{"x = None on line 2", []byte("\nx = None\n"), 2, "x", 1, 4, 8, nil, nil},
		{"x = None + comment + pass", []byte("x = None\n# gap\npass\n"), 1, "x", 1, 4, 8, nil,
			[]bytecode.NoOpStmt{{Line: 3, EndCol: 4}}},
		{"x = \"hello world\"", []byte("x = \"hello world\"\n"), 1, "x", 1, 4, 17, "hello world", nil},
		{"x = ...", []byte("x = ...\n"), 1, "x", 1, 4, 7, bytecode.Ellipsis, nil},
		{"x = b\"hi\"", []byte("x = b\"hi\"\n"), 1, "x", 1, 4, 9, []byte("hi"), nil},
		{"x = b\"\"", []byte("x = b\"\"\n"), 1, "x", 1, 4, 7, []byte(""), nil},
		{"x = ... + pass", []byte("x = ...\npass\n"), 1, "x", 1, 4, 7, bytecode.Ellipsis,
			[]bytecode.NoOpStmt{{Line: 2, EndCol: 4}}},
		{"x = b\"hi\" + pass", []byte("x = b\"hi\"\npass\n"), 1, "x", 1, 4, 9, []byte("hi"),
			[]bytecode.NoOpStmt{{Line: 2, EndCol: 4}}},
		{"x = 0", []byte("x = 0\n"), 1, "x", 1, 4, 5, int64(0), nil},
		{"x = 1", []byte("x = 1\n"), 1, "x", 1, 4, 5, int64(1), nil},
		{"x = 42", []byte("x = 42\n"), 1, "x", 1, 4, 6, int64(42), nil},
		{"x = 255", []byte("x = 255\n"), 1, "x", 1, 4, 7, int64(255), nil},
		{"x = 256", []byte("x = 256\n"), 1, "x", 1, 4, 7, int64(256), nil},
		{"x = 1000000", []byte("x = 1000000\n"), 1, "x", 1, 4, 11, int64(1000000), nil},
		{"x = 0xff", []byte("x = 0xff\n"), 1, "x", 1, 4, 8, int64(255), nil},
		{"x = 0x100", []byte("x = 0x100\n"), 1, "x", 1, 4, 9, int64(256), nil},
		{"x = 1 + pass", []byte("x = 1\npass\n"), 1, "x", 1, 4, 5, int64(1),
			[]bytecode.NoOpStmt{{Line: 2, EndCol: 4}}},
		{"x = 1.0", []byte("x = 1.0\n"), 1, "x", 1, 4, 7, float64(1.0), nil},
		{"x = 3.14", []byte("x = 3.14\n"), 1, "x", 1, 4, 8, float64(3.14), nil},
		{"x = 0.0", []byte("x = 0.0\n"), 1, "x", 1, 4, 7, float64(0.0), nil},
		{"x = 1e100", []byte("x = 1e100\n"), 1, "x", 1, 4, 9, float64(1e100), nil},
		{"x = 1.0 + pass", []byte("x = 1.0\npass\n"), 1, "x", 1, 4, 7, float64(1.0),
			[]bytecode.NoOpStmt{{Line: 2, EndCol: 4}}},
		{"x = 1j", []byte("x = 1j\n"), 1, "x", 1, 4, 6, complex(0, 1), nil},
		{"x = 0j", []byte("x = 0j\n"), 1, "x", 1, 4, 6, complex(0, 0), nil},
		{"x = 0.5j", []byte("x = 0.5j\n"), 1, "x", 1, 4, 8, complex(0, 0.5), nil},
		{"x = -1", []byte("x = -1\n"), 1, "x", 1, 4, 6, negLiteral{int64(1), int64(-1)}, nil},
		{"x = -5", []byte("x = -5\n"), 1, "x", 1, 4, 6, negLiteral{int64(5), int64(-5)}, nil},
		{"x = -256", []byte("x = -256\n"), 1, "x", 1, 4, 8, negLiteral{int64(256), int64(-256)}, nil},
		{"x = -3.14", []byte("x = -3.14\n"), 1, "x", 1, 4, 9, negLiteral{float64(3.14), float64(-3.14)}, nil},
		{"x = -1e10", []byte("x = -1e10\n"), 1, "x", 1, 4, 9, negLiteral{float64(1e10), float64(-1e10)}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			var wantConsts []any
			var wantBC []byte
			if nl, ok := tc.value.(negLiteral); ok {
				wantConsts = []any{nl.pos, nil, nl.neg}
				wantBC = bytecode.AssignBytecodeAt(2, 1, len(tc.tail))
			} else {
				noneIdx := byte(1)
				wantConsts = []any{tc.value, nil}
				if tc.value == nil {
					noneIdx = 0
					wantConsts = []any{nil}
				}
				if iv, ok := tc.value.(int64); ok && iv >= 0 && iv <= 255 {
					wantBC = bytecode.AssignSmallIntBytecode(byte(iv), len(tc.tail))
				} else {
					wantBC = bytecode.AssignBytecode(noneIdx, len(tc.tail))
				}
			}
			if !bytes.Equal(c.Bytecode, wantBC) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, wantBC)
			}
			wantLT := bytecode.AssignLineTable(tc.asgnLine, tc.nameLen, tc.valStartCol, tc.valEndCol, tc.tail)
			if !bytes.Equal(c.LineTable, wantLT) {
				t.Errorf("linetable = %x; want %x", c.LineTable, wantLT)
			}
			if len(c.Consts) != len(wantConsts) {
				t.Fatalf("consts len = %d; want %d (%v)", len(c.Consts), len(wantConsts), wantConsts)
			}
			for i := range wantConsts {
				if !reflect.DeepEqual(c.Consts[i], wantConsts[i]) {
					t.Errorf("consts[%d] = %v; want %v", i, c.Consts[i], wantConsts[i])
				}
			}
			if len(c.Names) != 1 || c.Names[0] != tc.asgnName {
				t.Errorf("names = %v; want (%q,)", c.Names, tc.asgnName)
			}
		})
	}
}

func TestMultiAssign(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		src    []byte
		consts []any
		names  []string
		bc     []byte
	}{
		{
			name:   "two small ints",
			src:    []byte("x = 1\ny = 2\n"),
			consts: []any{int64(1), nil},
			names:  []string{"x", "y"},
			bc:     []byte{0x80, 0, 0x5e, 1, 0x74, 0, 0x5e, 2, 0x74, 1, 0x52, 1, 0x23, 0},
		},
		{
			name:   "three small ints",
			src:    []byte("x = 1\ny = 2\nz = 3\n"),
			consts: []any{int64(1), nil},
			names:  []string{"x", "y", "z"},
			bc:     []byte{0x80, 0, 0x5e, 1, 0x74, 0, 0x5e, 2, 0x74, 1, 0x5e, 3, 0x74, 2, 0x52, 1, 0x23, 0},
		},
		{
			name:   "large then small",
			src:    []byte("x = 300\ny = 2\n"),
			consts: []any{int64(300), nil},
			names:  []string{"x", "y"},
			bc:     []byte{0x80, 0, 0x52, 0, 0x74, 0, 0x5e, 2, 0x74, 1, 0x52, 1, 0x23, 0},
		},
		{
			name:   "negative then small",
			src:    []byte("x = -1\ny = 2\n"),
			consts: []any{int64(1), nil, int64(-1)},
			names:  []string{"x", "y"},
			bc:     []byte{0x80, 0, 0x52, 2, 0x74, 0, 0x5e, 2, 0x74, 1, 0x52, 1, 0x23, 0},
		},
		{
			name:   "None then small",
			src:    []byte("x = None\ny = 2\n"),
			consts: []any{nil},
			names:  []string{"x", "y"},
			bc:     []byte{0x80, 0, 0x52, 0, 0x74, 0, 0x5e, 2, 0x74, 1, 0x52, 0, 0x23, 0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if !bytes.Equal(c.Bytecode, tc.bc) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, tc.bc)
			}
			if len(c.Consts) != len(tc.consts) {
				t.Fatalf("consts len = %d; want %d (%v)", len(c.Consts), len(tc.consts), tc.consts)
			}
			for i := range tc.consts {
				if !reflect.DeepEqual(c.Consts[i], tc.consts[i]) {
					t.Errorf("consts[%d] = %v (%T); want %v (%T)", i, c.Consts[i], c.Consts[i], tc.consts[i], tc.consts[i])
				}
			}
			if !reflect.DeepEqual(c.Names, tc.names) {
				t.Errorf("names = %v; want %v", c.Names, tc.names)
			}
		})
	}
}

func TestChainedAssign(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		src     []byte
		consts  []any
		names   []string
		bc      []byte
		targets []bytecode.ChainedTarget
		line    int
		vStart  byte
		vEnd    byte
		tail    []bytecode.NoOpStmt
	}{
		{
			name:    "x = y = 1",
			src:     []byte("x = y = 1\n"),
			consts:  []any{int64(1), nil},
			names:   []string{"x", "y"},
			bc:      []byte{0x80, 0, 0x5e, 1, 0x3b, 1, 0x74, 0, 0x74, 1, 0x52, 1, 0x23, 0},
			targets: []bytecode.ChainedTarget{{NameStart: 0, NameLen: 1}, {NameStart: 4, NameLen: 1}},
			line:    1, vStart: 8, vEnd: 9,
		},
		{
			name:    "x = y = z = 1",
			src:     []byte("x = y = z = 1\n"),
			consts:  []any{int64(1), nil},
			names:   []string{"x", "y", "z"},
			bc:      []byte{0x80, 0, 0x5e, 1, 0x3b, 1, 0x74, 0, 0x3b, 1, 0x74, 1, 0x74, 2, 0x52, 1, 0x23, 0},
			targets: []bytecode.ChainedTarget{{NameStart: 0, NameLen: 1}, {NameStart: 4, NameLen: 1}, {NameStart: 8, NameLen: 1}},
			line:    1, vStart: 12, vEnd: 13,
		},
		{
			name:    "x = y = 300",
			src:     []byte("x = y = 300\n"),
			consts:  []any{int64(300), nil},
			names:   []string{"x", "y"},
			bc:      []byte{0x80, 0, 0x52, 0, 0x3b, 1, 0x74, 0, 0x74, 1, 0x52, 1, 0x23, 0},
			targets: []bytecode.ChainedTarget{{NameStart: 0, NameLen: 1}, {NameStart: 4, NameLen: 1}},
			line:    1, vStart: 8, vEnd: 11,
		},
		{
			name:    "x = y = None",
			src:     []byte("x = y = None\n"),
			consts:  []any{nil},
			names:   []string{"x", "y"},
			bc:      []byte{0x80, 0, 0x52, 0, 0x3b, 1, 0x74, 0, 0x74, 1, 0x52, 0, 0x23, 0},
			targets: []bytecode.ChainedTarget{{NameStart: 0, NameLen: 1}, {NameStart: 4, NameLen: 1}},
			line:    1, vStart: 8, vEnd: 12,
		},
		{
			name:    "x = y = -1",
			src:     []byte("x = y = -1\n"),
			consts:  []any{int64(1), nil, int64(-1)},
			names:   []string{"x", "y"},
			bc:      []byte{0x80, 0, 0x52, 2, 0x3b, 1, 0x74, 0, 0x74, 1, 0x52, 1, 0x23, 0},
			targets: []bytecode.ChainedTarget{{NameStart: 0, NameLen: 1}, {NameStart: 4, NameLen: 1}},
			line:    1, vStart: 8, vEnd: 10,
		},
		{
			name:    "a = b = c = d = 5",
			src:     []byte("a = b = c = d = 5\n"),
			consts:  []any{int64(5), nil},
			names:   []string{"a", "b", "c", "d"},
			bc:      []byte{0x80, 0, 0x5e, 5, 0x3b, 1, 0x74, 0, 0x3b, 1, 0x74, 1, 0x3b, 1, 0x74, 2, 0x74, 3, 0x52, 1, 0x23, 0},
			targets: []bytecode.ChainedTarget{{NameStart: 0, NameLen: 1}, {NameStart: 4, NameLen: 1}, {NameStart: 8, NameLen: 1}, {NameStart: 12, NameLen: 1}},
			line:    1, vStart: 16, vEnd: 17,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if !bytes.Equal(c.Bytecode, tc.bc) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, tc.bc)
			}
			if len(c.Consts) != len(tc.consts) {
				t.Fatalf("consts len = %d; want %d (%v)", len(c.Consts), len(tc.consts), tc.consts)
			}
			for i := range tc.consts {
				if !reflect.DeepEqual(c.Consts[i], tc.consts[i]) {
					t.Errorf("consts[%d] = %v (%T); want %v (%T)", i, c.Consts[i], c.Consts[i], tc.consts[i], tc.consts[i])
				}
			}
			if !reflect.DeepEqual(c.Names, tc.names) {
				t.Errorf("names = %v; want %v", c.Names, tc.names)
			}
			wantLT := bytecode.ChainedAssignLineTable(tc.line, tc.targets, tc.vStart, tc.vEnd, tc.tail)
			if !bytes.Equal(c.LineTable, wantLT) {
				t.Errorf("linetable = %x; want %x", c.LineTable, wantLT)
			}
		})
	}
}

func TestAugAssign(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		src         []byte
		consts      []any
		names       []string
		bc          []byte
		initLine    int
		nameLen     byte
		initVStart  byte
		initVEnd    byte
		augLine     int
		augVStart   byte
		augVEnd     byte
		tail        []bytecode.NoOpStmt
	}{
		{
			name:       "x = 0 then x += 1",
			src:        []byte("x = 0\nx += 1\n"),
			consts:     []any{int64(0), nil},
			names:      []string{"x"},
			bc:         []byte{0x80, 0, 0x5e, 0, 0x74, 0, 0x5d, 0, 0x5e, 1, 0x2c, 13, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
		{
			name:       "x = 5 then x += 10",
			src:        []byte("x = 5\nx += 10\n"),
			consts:     []any{int64(5), nil},
			names:      []string{"x"},
			bc:         []byte{0x80, 0, 0x5e, 5, 0x74, 0, 0x5d, 0, 0x5e, 10, 0x2c, 13, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 7,
		},
		{
			name:       "x = 0 then x += 300",
			src:        []byte("x = 0\nx += 300\n"),
			consts:     []any{int64(0), int64(300), nil},
			names:      []string{"x"},
			bc:         []byte{0x80, 0, 0x5e, 0, 0x74, 0, 0x5d, 0, 0x52, 1, 0x2c, 13, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 2, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 8,
		},
		{
			name:       "x = 300 then x += 1",
			src:        []byte("x = 300\nx += 1\n"),
			consts:     []any{int64(300), nil},
			names:      []string{"x"},
			bc:         []byte{0x80, 0, 0x52, 0, 0x74, 0, 0x5d, 0, 0x5e, 1, 0x2c, 13, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 7,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
		// v0.0.16: remaining inplace operators
		{
			name: "x = 5 then x -= 3",
			src:  []byte("x = 5\nx -= 3\n"), consts: []any{int64(5), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 5, 0x74, 0, 0x5d, 0, 0x5e, 3, 0x2c, 23, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
		{
			name: "x = 5 then x *= 3",
			src:  []byte("x = 5\nx *= 3\n"), consts: []any{int64(5), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 5, 0x74, 0, 0x5d, 0, 0x5e, 3, 0x2c, 18, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
		{
			name: "x = 6 then x //= 2",
			src:  []byte("x = 6\nx //= 2\n"), consts: []any{int64(6), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 6, 0x74, 0, 0x5d, 0, 0x5e, 2, 0x2c, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 6, augVEnd: 7,
		},
		{
			name: "x = 7 then x %= 3",
			src:  []byte("x = 7\nx %= 3\n"), consts: []any{int64(7), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 7, 0x74, 0, 0x5d, 0, 0x5e, 3, 0x2c, 19, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
		{
			name: "x = 2 then x **= 3",
			src:  []byte("x = 2\nx **= 3\n"), consts: []any{int64(2), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 2, 0x74, 0, 0x5d, 0, 0x5e, 3, 0x2c, 21, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 6, augVEnd: 7,
		},
		{
			name: "x = 7 then x &= 3",
			src:  []byte("x = 7\nx &= 3\n"), consts: []any{int64(7), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 7, 0x74, 0, 0x5d, 0, 0x5e, 3, 0x2c, 14, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
		{
			name: "x = 5 then x |= 3",
			src:  []byte("x = 5\nx |= 3\n"), consts: []any{int64(5), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 5, 0x74, 0, 0x5d, 0, 0x5e, 3, 0x2c, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
		{
			name: "x = 5 then x ^= 3",
			src:  []byte("x = 5\nx ^= 3\n"), consts: []any{int64(5), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 5, 0x74, 0, 0x5d, 0, 0x5e, 3, 0x2c, 25, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
		{
			name: "x = 8 then x >>= 1",
			src:  []byte("x = 8\nx >>= 1\n"), consts: []any{int64(8), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 8, 0x74, 0, 0x5d, 0, 0x5e, 1, 0x2c, 22, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 6, augVEnd: 7,
		},
		{
			name: "x = 2 then x <<= 3",
			src:  []byte("x = 2\nx <<= 3\n"), consts: []any{int64(2), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 2, 0x74, 0, 0x5d, 0, 0x5e, 3, 0x2c, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 6, augVEnd: 7,
		},
		{
			name: "x = 6 then x /= 2",
			src:  []byte("x = 6\nx /= 2\n"), consts: []any{int64(6), nil}, names: []string{"x"},
			bc:       []byte{0x80, 0, 0x5e, 6, 0x74, 0, 0x5d, 0, 0x5e, 2, 0x2c, 24, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x74, 0, 0x52, 1, 0x23, 0},
			initLine: 1, nameLen: 1, initVStart: 4, initVEnd: 5,
			augLine: 2, augVStart: 5, augVEnd: 6,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if !bytes.Equal(c.Bytecode, tc.bc) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, tc.bc)
			}
			if len(c.Consts) != len(tc.consts) {
				t.Fatalf("consts len = %d; want %d (%v)", len(c.Consts), len(tc.consts), tc.consts)
			}
			for i := range tc.consts {
				if !reflect.DeepEqual(c.Consts[i], tc.consts[i]) {
					t.Errorf("consts[%d] = %v (%T); want %v (%T)", i, c.Consts[i], c.Consts[i], tc.consts[i], tc.consts[i])
				}
			}
			if !reflect.DeepEqual(c.Names, tc.names) {
				t.Errorf("names = %v; want %v", c.Names, tc.names)
			}
			if c.StackSize != 2 {
				t.Errorf("stacksize = %d; want 2", c.StackSize)
			}
			wantLT := bytecode.AugAssignLineTable(
				tc.initLine, tc.nameLen, tc.initVStart, tc.initVEnd,
				tc.augLine, tc.augVStart, tc.augVEnd,
				tc.tail,
			)
			if !bytes.Equal(c.LineTable, wantLT) {
				t.Errorf("linetable = %x; want %x", c.LineTable, wantLT)
			}
		})
	}
}

func TestBinOpAssign(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		src       []byte
		oparg     byte
		leftName  string
		leftCol   byte
		leftLen   byte
		rightName string
		rightCol  byte
		rightLen  byte
		target    string
		targetLen byte
	}{
		{
			name: "x = a + b", src: []byte("x = a + b\n"),
			oparg: bytecode.NbAdd, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 8, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a - b", src: []byte("x = a - b\n"),
			oparg: bytecode.NbSubtract, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 8, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a * b", src: []byte("x = a * b\n"),
			oparg: bytecode.NbMultiply, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 8, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a / b", src: []byte("x = a / b\n"),
			oparg: bytecode.NbTrueDivide, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 8, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a // b", src: []byte("x = a // b\n"),
			oparg: bytecode.NbFloorDivide, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 9, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a ** b", src: []byte("x = a ** b\n"),
			oparg: bytecode.NbPower, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 9, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a & b", src: []byte("x = a & b\n"),
			oparg: bytecode.NbAnd, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 8, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a | b", src: []byte("x = a | b\n"),
			oparg: bytecode.NbOr, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 8, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a ^ b", src: []byte("x = a ^ b\n"),
			oparg: bytecode.NbXor, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 8, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a << b", src: []byte("x = a << b\n"),
			oparg: bytecode.NbLshift, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 9, rightLen: 1, target: "x", targetLen: 1,
		},
		{
			name: "x = a >> b", src: []byte("x = a >> b\n"),
			oparg: bytecode.NbRshift, leftName: "a", leftCol: 4, leftLen: 1,
			rightName: "b", rightCol: 9, rightLen: 1, target: "x", targetLen: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			wantNames := []string{tc.leftName, tc.rightName, tc.target}
			if !reflect.DeepEqual(c.Names, wantNames) {
				t.Errorf("names = %v; want %v", c.Names, wantNames)
			}
			if len(c.Consts) != 1 || c.Consts[0] != nil {
				t.Errorf("consts = %v; want (None,)", c.Consts)
			}
			if c.StackSize != 2 {
				t.Errorf("stacksize = %d; want 2", c.StackSize)
			}
			wantBC := bytecode.BinOpAssignBytecode(tc.oparg)
			if !bytes.Equal(c.Bytecode, wantBC) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, wantBC)
			}
			wantLT := bytecode.BinOpAssignLineTable(1, tc.leftCol, tc.leftLen, tc.rightCol, tc.rightLen, tc.targetLen)
			if !bytes.Equal(c.LineTable, wantLT) {
				t.Errorf("linetable = %x; want %x", c.LineTable, wantLT)
			}
		})
	}
}

func TestUnaryAssign(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		src         []byte
		kind        unaryKind
		opCol       byte
		operandCol  byte
		operandLen  byte
		operandName string
		targetLen   byte
	}{
		{
			name: "x = -a", src: []byte("x = -a\n"),
			kind: unaryNeg, opCol: 4, operandCol: 5, operandLen: 1, operandName: "a", targetLen: 1,
		},
		{
			name: "x = ~a", src: []byte("x = ~a\n"),
			kind: unaryInvert, opCol: 4, operandCol: 5, operandLen: 1, operandName: "a", targetLen: 1,
		},
		{
			name: "x = not a", src: []byte("x = not a\n"),
			kind: unaryNot, opCol: 4, operandCol: 8, operandLen: 1, operandName: "a", targetLen: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Compile(tc.src, Options{Filename: "x.py"})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			wantNames := []string{tc.operandName, "x"}
			if !reflect.DeepEqual(c.Names, wantNames) {
				t.Errorf("names = %v; want %v", c.Names, wantNames)
			}
			if len(c.Consts) != 1 || c.Consts[0] != nil {
				t.Errorf("consts = %v; want (None,)", c.Consts)
			}
			if c.StackSize != 1 {
				t.Errorf("stacksize = %d; want 1", c.StackSize)
			}
			var wantBC []byte
			var wantLT []byte
			switch tc.kind {
			case unaryNeg:
				wantBC = bytecode.UnaryNegInvertBytecode(bytecode.UNARY_NEGATIVE)
				wantLT = bytecode.UnaryNegInvertLineTable(1, tc.opCol, tc.operandCol, tc.operandLen, tc.targetLen)
			case unaryInvert:
				wantBC = bytecode.UnaryNegInvertBytecode(bytecode.UNARY_INVERT)
				wantLT = bytecode.UnaryNegInvertLineTable(1, tc.opCol, tc.operandCol, tc.operandLen, tc.targetLen)
			case unaryNot:
				wantBC = bytecode.UnaryNotBytecode()
				wantLT = bytecode.UnaryNotLineTable(1, tc.opCol, tc.operandCol, tc.operandLen, tc.targetLen)
			}
			if !bytes.Equal(c.Bytecode, wantBC) {
				t.Errorf("bytecode = %x; want %x", c.Bytecode, wantBC)
			}
			if !bytes.Equal(c.LineTable, wantLT) {
				t.Errorf("linetable = %x; want %x", c.LineTable, wantLT)
			}
		})
	}
}

func TestUnsupportedSourceRejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  []byte
	}{
		{"assignment to reserved name", []byte("None = 1\n")},
		{"augmented assignment", []byte("x += 1\n")},
		{"call", []byte("print('hi')\n")},
		{"import dotted alias", []byte("import os.path as p\n")},
		{"docstring with backslash escape", []byte("\"hi\\nthere\"\n")},
		{"non-ascii docstring", []byte("\"héllo\"\n")},
		{"f-string", []byte("f\"hi\"\n")},
		{"indented pass", []byte("  pass\n")},
		{"indented pass after blank", []byte("\n  pass\n")},
		{"name Ellipsis", []byte("Ellipsis\n")},
		{"unary negative", []byte("-1\n")},
		{"binary op", []byte("1 + 2\n")},
		{"trailing comma", []byte("1,\n")},
		{"pass then assignment", []byte("pass\nx = None\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compile(tc.src, Options{Filename: "x.py"})
			if !errors.Is(err, ErrUnsupportedSource) {
				t.Errorf("Compile(%q) err = %v; want ErrUnsupportedSource", tc.src, err)
			}
		})
	}
}
