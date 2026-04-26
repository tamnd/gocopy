package compiler

import (
	"bytes"
	"errors"
	"testing"

	"github.com/tamnd/gocopy/v1/bytecode"
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
			if len(c.Bytecode) != 6 {
				t.Errorf("bytecode len = %d; want 6", len(c.Bytecode))
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
			if len(c.Bytecode) != 6 {
				t.Errorf("bytecode len = %d; want 6", len(c.Bytecode))
			}
			if len(c.Consts) != 1 || c.Consts[0] != nil {
				t.Errorf("consts = %v; want (None,)", c.Consts)
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
		{"assignment", []byte("x = 1\n")},
		{"call", []byte("print('hi')\n")},
		{"import", []byte("import sys\n")},
		{"docstring", []byte("\"hi\"\n")},
		{"bytes literal", []byte("b'x'\n")},
		{"two passes", []byte("pass\npass\n")},
		{"pass on line 2 (leading blank)", []byte("\npass\n")},
		{"pass on line 2 (leading comment)", []byte("# c\npass\n")},
		{"indented pass", []byte("  pass\n")},
		{"name Ellipsis", []byte("Ellipsis\n")},
		{"unary negative", []byte("-1\n")},
		{"binary op", []byte("1 + 2\n")},
		{"trailing comma", []byte("1,\n")},
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
