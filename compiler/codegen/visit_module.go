package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
)

// noOpStmt records a no-op module-level statement's source position.
// Line/EndLine are 1-indexed. EndCol is the 0-indexed exclusive end
// column on EndLine, computed from the source text the same way the
// classifier computes it (line trimmed of the trailing comment and
// whitespace).
//
// Used by the v0.6.9+ classifier-shadow build paths
// (visit_assign.go and the multi-/chained-/aug-assign variants) to
// describe trailing no-op statements after the assign(s). The
// modEmpty/modNoOps/modDocstring producers were promoted to the
// visitor pipeline at v0.7.1 and their helpers were removed; this
// type and locOf below remain as the shared description of trailing
// no-op tails.
type noOpStmt struct {
	Line    uint32
	EndLine uint32
	EndCol  uint16
}

func locOf(s noOpStmt) bytecode.Loc {
	return bytecode.Loc{Line: s.Line, EndLine: s.EndLine, Col: 0, EndCol: s.EndCol}
}
