package compiler

import (
	"errors"
	"testing"
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
		})
	}
}

func TestNonEmptyModuleRejected(t *testing.T) {
	t.Parallel()
	cases := [][]byte{
		[]byte("x = 1"),
		[]byte("print('hi')"),
		[]byte("# ok\nimport sys\n"),
	}
	for _, src := range cases {
		_, err := Compile(src, Options{Filename: "x.py"})
		if !errors.Is(err, ErrNotEmptyModule) {
			t.Errorf("Compile(%q) err = %v; want ErrNotEmptyModule", src, err)
		}
	}
}
