package pyc

import (
	"encoding/hex"
	"testing"
)

// TestSourceHashVectors locks SipHash-1-3 against vectors observed from
// CPython 3.14.4's _imp.source_hash(_imp.pyc_magic_number_token, src).
//
//	python3.14 -c "import _imp; k=_imp.pyc_magic_number_token; \
//	    print(_imp.source_hash(k, b'').hex())"  → 61f5b7cb653a6aa4
//	... b'a'                                      → 9713ded0c3324373
//	... b'abc'                                    → 6e461bf6a5e97b4a
//	... b'x'*64                                   → 96ee4ddf98d8ded8
func TestSourceHashVectors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []byte
		want string
	}{
		{[]byte{}, "61f5b7cb653a6aa4"},
		{[]byte("a"), "9713ded0c3324373"},
		{[]byte("abc"), "6e461bf6a5e97b4a"},
		{bytesRepeat('x', 64), "96ee4ddf98d8ded8"},
	}
	for _, tc := range cases {
		got := SourceHash(tc.in)
		if hex.EncodeToString(got[:]) != tc.want {
			t.Errorf("SourceHash(%q) = %x; want %s", tc.in, got, tc.want)
		}
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
