// Package compiler lowers a Python source file to a bytecode.CodeObject.
//
// v0.0.2 supports two body shapes:
//
//  1. Empty module (file is empty or contains only whitespace, blank
//     lines, and comments).
//  2. Exactly one no-op statement on line 1 starting at column 0,
//     followed only by blank or comment-only lines. The no-op set is:
//     `pass`, `None`, `True`, `False`, `...`, an integer literal, a
//     float literal, or a complex literal.
//
// Both shapes compile to the same bytecode + consts as the empty
// module; only the line table differs (see bytecode/linetable.go).
//
// Anything else returns ErrUnsupportedSource. Wiring github.com/tamnd/
// gopapy as the parser, which would replace this hand-rolled scanner,
// is deferred until gopapy cuts a v1.0.0 (its current latest tag is
// v0.1.x and the /v1 module path is not yet consumable).
package compiler

import (
	"errors"

	"github.com/tamnd/gocopy/v1/bytecode"
)

// ErrUnsupportedSource is returned for any module body the v0.0.x
// rungs have not yet learned to compile.
var ErrUnsupportedSource = errors.New("compiler: source not yet supported")

// Options configures the compiler. Filename ends up in CodeObject.Filename
// (a.k.a. CPython's co_filename).
type Options struct {
	Filename string
}

// Compile returns the CodeObject for the given Python source bytes.
func Compile(source []byte, opts Options) (*bytecode.CodeObject, error) {
	cls, ok := classify(source)
	if !ok {
		return nil, ErrUnsupportedSource
	}
	switch cls.kind {
	case modEmpty:
		return module(opts.Filename, bytecode.LineTableEmpty()), nil
	case modSingleNoOp:
		return module(opts.Filename, bytecode.LineTableSingleNoOp(byte(cls.endCol))), nil
	}
	return nil, ErrUnsupportedSource
}

// module returns the canonical Code object for any module body that
// compiles to "implicit return None" with no real local state. The
// caller supplies the line table appropriate to the body shape.
//
// Bytes verified against `python3.14 -m py_compile` for empty modules
// and the v0.0.2 no-op statement set. The bytecode and consts are
// identical across all in-scope shapes; only the line table differs.
func module(filename string, lineTable []byte) *bytecode.CodeObject {
	return &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1, // implicit None on the return path
		Flags:           0, // module scope: not optimized, not new-locals
		Bytecode: []byte{
			byte(bytecode.RESUME), 0,
			byte(bytecode.LOAD_CONST), 0,
			byte(bytecode.RETURN_VALUE), 0,
		},
		Consts:          []any{nil}, // (None,)
		Names:           []string{},
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        filename,
		Name:            "<module>",
		QualName:        "<module>",
		FirstLineNo:     1,
		LineTable:       lineTable,
		ExcTable:        []byte{},
	}
}
