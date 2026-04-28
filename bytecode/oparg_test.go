package bytecode

import "testing"

func TestLoadGlobalArg(t *testing.T) {
	tests := []struct {
		idx      byte
		pushNull bool
		want     byte
	}{
		{0, false, 0},
		{0, true, 1},
		{1, false, 2},
		{1, true, 3},
		{5, true, 11},  // 5<<1 | 1 = 11
		{42, false, 84}, // 42<<1 = 84
		{127, true, 255},
	}
	for _, tt := range tests {
		got := LoadGlobalArg(tt.idx, tt.pushNull)
		if got != tt.want {
			t.Errorf("LoadGlobalArg(%d,%v)=%d want %d", tt.idx, tt.pushNull, got, tt.want)
		}
	}
}

func TestLoadGlobalArgEquivalentToMagic(t *testing.T) {
	// Replicate the exact magic-number sites in compiler/func_body.go.
	for idx := byte(0); idx < 128; idx++ {
		callForm := LoadGlobalArg(idx, true)
		magic := byte((idx << 1) | 1)
		if callForm != magic {
			t.Errorf("idx=%d: helper=%d magic=%d", idx, callForm, magic)
		}
		valueForm := LoadGlobalArg(idx, false)
		valueMagic := byte(idx << 1)
		if valueForm != valueMagic {
			t.Errorf("idx=%d (no-push): helper=%d magic=%d", idx, valueForm, valueMagic)
		}
	}
}

func TestLoadGlobalArgOutOfRangePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nameIdx>=128")
		}
	}()
	LoadGlobalArg(128, false)
}

func TestLoadAttrArg(t *testing.T) {
	if LoadAttrArg(3, true) != 7 {
		t.Errorf("LoadAttrArg(3,true)=%d want 7", LoadAttrArg(3, true))
	}
	if LoadAttrArg(3, false) != 6 {
		t.Errorf("LoadAttrArg(3,false)=%d want 6", LoadAttrArg(3, false))
	}
	for idx := byte(0); idx < 128; idx++ {
		want := byte(idx << 1)
		if got := LoadAttrArg(idx, false); got != want {
			t.Errorf("LoadAttrArg(%d,false)=%d want %d", idx, got, want)
		}
	}
}

func TestLoadAttrArgOutOfRangePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nameIdx>=128")
		}
	}()
	LoadAttrArg(200, false)
}

func TestLflblflbArg(t *testing.T) {
	for l := byte(0); l < 16; l++ {
		for r := byte(0); r < 16; r++ {
			got := LflblflbArg(l, r)
			want := (l << 4) | r
			if got != want {
				t.Errorf("LflblflbArg(%d,%d)=%d want %d", l, r, got, want)
			}
		}
	}
}

func TestLflblflbArgOutOfRangePanics(t *testing.T) {
	cases := [][2]byte{{16, 0}, {0, 16}, {255, 0}}
	for _, c := range cases {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("LflblflbArg(%d,%d): expected panic", c[0], c[1])
				}
			}()
			LflblflbArg(c[0], c[1])
		}()
	}
}

func TestCompareCondArg(t *testing.T) {
	tests := []struct {
		base byte
		want byte
	}{
		{CmpLt, CmpLt + 16},
		{CmpEq, CmpEq + 16},
		{CmpGt, CmpGt + 16},
	}
	for _, tt := range tests {
		got := CompareCondArg(tt.base)
		if got != tt.want {
			t.Errorf("CompareCondArg(%d)=%d want %d", tt.base, got, tt.want)
		}
	}
}

func TestLocalsKindArgConstants(t *testing.T) {
	tests := []struct {
		name string
		got  byte
		want byte
	}{
		{"LocalsKindLocal", LocalsKindLocal, 0x20},
		{"LocalsKindArg", LocalsKindArg, 0x26},
		{"LocalsKindArgCell", LocalsKindArgCell, 0x66},
		{"LocalsKindFree", LocalsKindFree, 0x80},
		{"LocalsKindCell", LocalsKindCell, 0x40},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s=0x%x want 0x%x", tt.name, tt.got, tt.want)
		}
	}
}
