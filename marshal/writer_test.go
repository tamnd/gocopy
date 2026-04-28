package marshal

import (
	"bytes"
	"testing"

	"github.com/tamnd/gocopy/bytecode"
)

// TestEmptyModuleGolden locks the marshal stream of the canonical empty
// module Code object against the bytes python3.14 -m py_compile produces.
//
// Source verified manually on Python 3.14.4:
//
//	echo -n > /tmp/empty.py
//	python3.14 -m py_compile /tmp/empty.py
//	xxd /tmp/__pycache__/empty.cpython-314.pyc | tail +2  # everything past the 16-byte header
//
// The Filename field below is the path the compiler bakes into the Code
// object, not the path on disk; the oracle harness uses
// tests/fixtures/001_empty.py.
//
// If this golden ever breaks under a Python 3.14.x patch bump, the bump is
// either: (a) a marshal-format fix on CPython side that we should mirror,
// or (b) a CPython bug. Investigate before regenerating.
func TestEmptyModuleGolden(t *testing.T) {
	t.Parallel()
	c := &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0,
		Bytecode: []byte{
			0x80, 0, 0x52, 0, 0x23, 0,
		},
		Consts:          []any{nil},
		Names:           []string{},
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        "tests/fixtures/empty.py",
		Name:            "<module>",
		QualName:        "<module>",
		FirstLineNo:     1,
		LineTable:       []byte{0xf2, 0x03, 0x01, 0x01, 0x01},
		ExcTable:        []byte{},
	}

	want := []byte{
		// TYPE_CODE | FLAG_REF, then 5 i32 fields
		0xe3,
		0, 0, 0, 0, // argcount
		0, 0, 0, 0, // posonlyargcount
		0, 0, 0, 0, // kwonlyargcount
		1, 0, 0, 0, // stacksize
		0, 0, 0, 0, // flags
		// bytecode: TYPE_STRING|FLAG_REF, len=6, payload
		0xf3, 6, 0, 0, 0,
		0x80, 0, 0x52, 0, 0x23, 0,
		// consts: TYPE_SMALL_TUPLE, count=1, [None]
		0x29, 1, 0x4e,
		// names: TYPE_SMALL_TUPLE|FLAG_REF, count=0
		0xa9, 0,
		// localsplusnames: TYPE_REF index=2 (the empty names tuple)
		0x72, 2, 0, 0, 0,
		// localspluskinds: TYPE_STRING|FLAG_REF, len=0
		0xf3, 0, 0, 0, 0,
		// filename: TYPE_SHORT_ASCII_INTERNED|FLAG_REF, len=23, "tests/fixtures/empty.py"
		0xda, 23,
		't', 'e', 's', 't', 's', '/', 'f', 'i', 'x', 't', 'u', 'r', 'e', 's', '/',
		'e', 'm', 'p', 't', 'y', '.', 'p', 'y',
		// name: TYPE_SHORT_ASCII_INTERNED|FLAG_REF, len=8, "<module>"
		0xda, 8,
		'<', 'm', 'o', 'd', 'u', 'l', 'e', '>',
		// qualname: TYPE_REF index=5 (the name slot)
		0x72, 5, 0, 0, 0,
		// firstlineno
		1, 0, 0, 0,
		// linetable: TYPE_STRING (no FLAG_REF), len=5
		0x73, 5, 0, 0, 0,
		0xf2, 0x03, 0x01, 0x01, 0x01,
		// exctable: TYPE_REF index=3 (the empty localspluskinds slot)
		0x72, 3, 0, 0, 0,
	}

	got, err := Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("marshal byte mismatch\nwant: %x\n got: %x", want, got)
	}
}

// TestStringConstInterningRule pins the all-name-chars test that decides
// whether a string const is emitted as TYPE_SHORT_ASCII_INTERNED|FLAG_REF
// (0xda) or plain TYPE_SHORT_ASCII (0x7a). CPython's
// intern_string_constants only interns when every byte is ASCII
// alphanumeric or underscore; multi-line docstrings, strings with
// spaces, and strings with punctuation must come out non-interned.
func TestStringConstInterningRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		s       string
		wantTag byte
	}{
		{"identifier-like", "hi", TYPE_SHORT_ASCII_INTERNED | FlagRef},
		{"underscore", "abc_def", TYPE_SHORT_ASCII_INTERNED | FlagRef},
		{"leading digit", "1abc", TYPE_SHORT_ASCII_INTERNED | FlagRef},
		{"with space", "a b", TYPE_SHORT_ASCII},
		{"with newline", "a\nb", TYPE_SHORT_ASCII},
		{"with hyphen", "a-b", TYPE_SHORT_ASCII},
	}
	for _, tc := range cases {
		c := &bytecode.CodeObject{
			Bytecode:        []byte{0x80, 0},
			Consts:          []any{tc.s, nil},
			Names:           []string{},
			LocalsPlusNames: []string{},
			LocalsPlusKinds: []byte{},
			Filename:        "x.py",
			Name:            "<module>",
			QualName:        "<module>",
			FirstLineNo:     1,
			LineTable:       []byte{0xf0},
			ExcTable:        []byte{},
		}
		got, err := Marshal(c)
		if err != nil {
			t.Fatalf("%s: Marshal: %v", tc.name, err)
		}
		i := bytes.Index(got, []byte(tc.s))
		if i < 2 {
			t.Fatalf("%s: payload not found in output", tc.name)
		}
		tag := got[i-2]
		if tag != tc.wantTag {
			t.Errorf("%s: type byte = 0x%02x, want 0x%02x", tc.name, tag, tc.wantTag)
		}
	}
}

// TestRefDedupsRepeatedString covers the FLAG_REF / TYPE_REF roundtrip for
// an interned string used twice in the walk (Name and QualName both
// "<module>"). The qualname slot must come out as a back-reference.
func TestRefDedupsRepeatedString(t *testing.T) {
	t.Parallel()
	c := &bytecode.CodeObject{
		Bytecode:        []byte{0x80, 0},
		Consts:          []any{nil},
		Names:           []string{},
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        "x.py",
		Name:            "abc",
		QualName:        "abc",
		FirstLineNo:     1,
		LineTable:       []byte{0xf0},
		ExcTable:        []byte{},
	}
	got, err := Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Look for a TYPE_REF (0x72) byte after "abc" appears the first time.
	first := bytes.Index(got, []byte("abc"))
	if first < 0 {
		t.Fatalf("Marshal output missing first abc")
	}
	rest := got[first+3:]
	if i := bytes.Index(rest, []byte("abc")); i >= 0 {
		t.Errorf("expected only one literal 'abc' (qualname should be TYPE_REF), found a second at offset %d", i)
	}
	if !bytes.Contains(rest, []byte{TYPE_REF}) {
		t.Errorf("expected a TYPE_REF (0x72) after first abc")
	}
}
