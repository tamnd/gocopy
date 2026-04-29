package optimize

import (
	"testing"

	"github.com/tamnd/gocopy/compiler/ir"
)

func TestRunIdentityNil(t *testing.T) {
	if got := Run(nil); got != nil {
		t.Fatalf("Run(nil) = %v, want nil", got)
	}
}

func TestRunIdentityNonNil(t *testing.T) {
	seq := ir.NewInstrSeq()
	if got := Run(seq); got != seq {
		t.Fatalf("Run(seq) = %p, want same pointer %p", got, seq)
	}
}

func TestRunPreservesBlocks(t *testing.T) {
	seq := ir.NewInstrSeq()
	b := seq.AddBlock()
	if b == nil {
		t.Fatal("AddBlock returned nil")
	}
	if got := Run(seq); got != seq || len(got.Blocks) != 1 {
		t.Fatalf("Run dropped blocks: blocks=%d, want 1", len(got.Blocks))
	}
}
