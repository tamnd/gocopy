package bytecode

import "testing"

// Compositions of FastX bytes that compiler/ uses as magic numbers.
// These match the existing literal sites in compiler/func_body.go,
// compiler/compiler.go, and compiler/mixed.go.
func TestFastBytesComposition(t *testing.T) {
	tests := []struct {
		name string
		got  byte
		want byte
	}{
		{"FastLocal|FastArg", FastLocal | FastArg, 0x26},
		{"FastLocal", FastLocal, 0x20},
		{"FastFree", FastFree, 0x80},
		{"FastLocal|FastArg|FastCell", FastLocal | FastArg | FastCell, 0x66},
		{"FastArgPos|FastArgKw==FastArg", FastArgPos | FastArgKw, FastArg},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got 0x%x, want 0x%x", tt.name, tt.got, tt.want)
		}
	}
}

func TestCodeFlagBits(t *testing.T) {
	tests := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"CO_OPTIMIZED", CO_OPTIMIZED, 0x0001},
		{"CO_NEWLOCALS", CO_NEWLOCALS, 0x0002},
		{"CO_HAS_DOCSTRING", CO_HAS_DOCSTRING, 0x04000000},
		{"CO_NO_MONITORING_EVENTS", CO_NO_MONITORING_EVENTS, 0x02000000},
		{"CO_FUTURE_ANNOTATIONS", CO_FUTURE_ANNOTATIONS, 0x01000000},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got 0x%x, want 0x%x", tt.name, tt.got, tt.want)
		}
	}
}
