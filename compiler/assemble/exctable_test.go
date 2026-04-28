package assemble

import "testing"

func TestEncodeExcTableEmpty(t *testing.T) {
	if got := EncodeExcTable(nil); len(got) != 0 {
		t.Fatalf("EncodeExcTable(nil) = %x, want empty", got)
	}
	if got := EncodeExcTable([]ExcRegion{}); len(got) != 0 {
		t.Fatalf("EncodeExcTable([]) = %x, want empty", got)
	}
}

func TestEncodeExcTableNonEmptyPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("EncodeExcTable(non-empty) did not panic; v0.6.10 should fill in the encoder")
		}
	}()
	EncodeExcTable([]ExcRegion{{StartUnit: 0, EndUnit: 1}})
}
