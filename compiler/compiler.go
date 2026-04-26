// Package compiler lowers a Python source file to a bytecode.CodeObject.
//
// v0.1.0 supports empty modules only (file is empty or contains only
// blank lines and comments). Non-empty modules return ErrNotEmptyModule;
// v0.1.1 lifts that restriction by depending on github.com/tamnd/gopapy
// for real parsing.
package compiler

import (
	"errors"

	"github.com/tamnd/gocopy/v1/bytecode"
)

// ErrNotEmptyModule is returned when v0.1.0 sees a non-empty module body.
// The roadmap (notes/Spec/1100/1158_gocopy_roadmap.md) lifts this in
// v0.1.1 by wiring in gopapy and emitting per-AST-node opcodes.
var ErrNotEmptyModule = errors.New("compiler: v0.1.0 only supports empty modules")

// Options configures the compiler. Filename ends up in CodeObject.Filename
// (a.k.a. CPython's co_filename).
type Options struct {
	Filename string
}

// Compile returns the CodeObject for the given Python source bytes.
// In v0.1.0 the source must be empty or contain only whitespace and
// comments.
func Compile(source []byte, opts Options) (*bytecode.CodeObject, error) {
	if !isEmptyModule(source) {
		return nil, ErrNotEmptyModule
	}
	return emptyModule(opts.Filename), nil
}

// isEmptyModule reports whether the source compiles to an empty AST module.
// A line is empty iff its non-whitespace content (with `#` comments
// stripped to end of line) is empty. Tracks whether we're inside a
// triple-quoted string so docstring-only top-level files compile too.
// CPython's compiler treats a single string-literal expression statement
// as a docstring and discards it from the body, but v0.1.0 deliberately
// punts on that; a bare docstring counts as non-empty here and falls
// out to ErrNotEmptyModule. The real empty-vs-docstring distinction
// lands once gopapy is wired in (v0.1.1).
func isEmptyModule(src []byte) bool {
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '#':
			// Skip to end of line.
			for i < len(src) && src[i] != '\n' {
				i++
			}
		default:
			return false
		}
	}
	return true
}

// emptyModule returns the canonical empty-module Code object. The exact
// byte layout matches what CPython 3.14 emits for an empty .py source
// file (verified against python3.14 -m py_compile on Python 3.14.4).
func emptyModule(filename string) *bytecode.CodeObject {
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
		// PEP 626 compact line table: one entry covering the three
		// instruction words at line 0 (synthetic RESUME). Bytes verified
		// against python3.14 -m py_compile output for an empty file.
		LineTable: []byte{0xf2, 0x03, 0x01, 0x01, 0x01},
		ExcTable:  []byte{},
	}
}
