// Package compiler lowers a Python source file to a bytecode.CodeObject.
//
// v0.0.6 supports three body shapes:
//
//  1. Empty module (file is empty or contains only whitespace, blank
//     lines, and comments).
//  2. N >= 1 no-op statements, each at column 0, with arbitrary blank
//     or comment-only lines anywhere (leading, trailing, or between
//     statements). The no-op set is: `pass`, `None`, `True`, `False`,
//     `...`, a numeric literal, a non-leading string or bytes
//     literal, or a leading bytes literal.
//  3. A leading ASCII string literal (the docstring), single-line or
//     triple-quoted across multiple lines, optionally followed by
//     N >= 0 no-op statements. Compiles to `LOAD_CONST docstring;
//     STORE_NAME __doc__` after the synthetic RESUME, then the no-op
//     tail. Multi-line docstrings emit a LONG line-table entry whose
//     end_line_delta covers the closing triple quote's source line.
//
// The first two shapes share the consts tuple `(None,)` and an empty
// names tuple. The docstring shape uses `(docstring, None)` and
// `('__doc__',)`.
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
		return module(opts.Filename,
			bytecode.NoOpBytecode(1),
			bytecode.LineTableEmpty(),
			[]any{nil}, nil,
		), nil
	case modNoOps:
		return module(opts.Filename,
			bytecode.NoOpBytecode(len(cls.stmts)),
			bytecode.LineTableNoOps(cls.stmts),
			[]any{nil}, nil,
		), nil
	case modDocstring:
		return module(opts.Filename,
			bytecode.DocstringBytecode(len(cls.stmts)),
			bytecode.DocstringLineTable(cls.docLine, cls.docEndLine, cls.docCol, cls.stmts),
			[]any{cls.docText, nil},
			[]string{"__doc__"},
		), nil
	}
	return nil, ErrUnsupportedSource
}

// module returns the canonical Code object for the given body. Only
// bytecode, line table, consts, and names vary across the v0.0.x
// shapes; everything else (locals, filename, qualname, exception
// table) is identical.
//
// Bytes verified against `python3.14 -m py_compile` for empty
// modules, the v0.0.4 N-statement no-op set, and the v0.0.5
// docstring shape.
func module(filename string, bc, lineTable []byte, consts []any, names []string) *bytecode.CodeObject {
	if names == nil {
		names = []string{}
	}
	return &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0,
		Bytecode:        bc,
		Consts:          consts,
		Names:           names,
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
