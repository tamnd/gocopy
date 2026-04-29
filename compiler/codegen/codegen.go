// Package codegen lowers a gocopy AST plus its symbol-table scope to
// a fully-formed *bytecode.CodeObject by emitting an InstrSeq IR and
// running it through the v0.6.5 assembler.
//
// At v0.6.6 codegen owned the empty-module shape. v0.6.7 added the
// modNoOps shape (N >= 1 no-op statements). v0.6.8 added the
// modDocstring shape: a leading string literal followed by zero or
// more no-op statements. v0.6.9 added the simple-constant modAssign
// shape: `<name> = <const>` (non-folded value) followed by zero or
// more no-op statements. v0.6.10 adds two natural extensions of
// modAssign: modMultiAssign (N >= 2 independent simple-constant
// assignments) and modChainedAssign (`t0 = t1 = ... = tN-1 =
// <const>`). v0.6.11 adds modAugAssign: `<name> = <initInt>`
// followed by `<name> <op>= <augInt>` for non-negative ints and any
// of the 12 in-place binary operators. v0.6.12 adds modBinOpAssign:
// a single `<target> = <left> <op> <right>` with all operands as
// Names and any of the 13 BinOp operators. v0.6.13 adds
// modUnaryAssign: a single `<target> = -<operand>`,
// `<target> = ~<operand>`, or `<target> = not <operand>` with the
// operand as a Name. v0.6.14 adds modCmpAssign: a single
// `<target> = <left> <cmp> <right>` with all operands as Names and
// any of the 10 comparison operators (six COMPARE_OP, two IS_OP,
// two CONTAINS_OP). v0.6.15 adds modBoolOp: a single
// `<target> = <left> and <right>` or `<target> = <left> or
// <right>` with both operands as Names. This is the first codegen
// path that emits a forward jump (POP_JUMP_IF_FALSE for `and`,
// POP_JUMP_IF_TRUE for `or`). v0.6.16 adds modTernary: a single
// `<target> = <trueVal> if <cond> else <falseVal>` with all three
// operands as Names. This is the second jump-bearing codegen path
// and the first with two return-bearing branches. v0.6.17 adds
// modCollection: a single `<target> = [...]`, `<target> = (...)`,
// `<target> = {...}` (set), or `<target> = {k: v, ...}` (dict)
// where every element is a Name on the same source line, plus the
// empty `[]`, `()`, and `{}` (dict) cases. Anything else returns
// ErrUnsupported and the caller falls back to the classifier.
//
// SOURCE: CPython 3.14 Python/codegen.c.
package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/symtable"
)

// ErrUnsupported indicates the codegen package does not yet handle
// this source shape. Callers should fall back to whatever path
// drove compilation before codegen took over the shape.
var ErrUnsupported = errors.New("codegen: source shape not yet supported")

// Options carries the metadata fields the AST does not encode plus
// the raw source bytes the no-op shape needs for end-of-line column
// calculation (the parser AST does not yet record end positions).
type Options struct {
	Source      []byte
	Filename    string
	Name        string
	QualName    string
	FirstLineNo int32
}

// Build emits a fully-formed CodeObject for the supported shape, or
// ErrUnsupported when the source falls outside the v0.6.7 codegen
// surface.
func Build(mod *ast.Module, scope *symtable.Scope, opts Options) (*bytecode.CodeObject, error) {
	if mod == nil {
		return nil, errors.New("codegen.Build: nil module")
	}
	_ = scope // reserved for the function-codegen release

	if len(mod.Body) == 0 {
		return buildEmptyModule(opts)
	}
	if stmts, ok := classifyNoOpModule(mod, opts.Source); ok {
		return buildNoOpsModule(stmts, opts)
	}
	if d, ok := classifyDocstringModule(mod, opts.Source); ok {
		return buildDocstringModule(d, opts)
	}
	if a, ok := classifyAssignModule(mod, opts.Source); ok {
		return buildAssignModule(a, opts)
	}
	if m, ok := classifyMultiAssignModule(mod, opts.Source); ok {
		return buildMultiAssignModule(m, opts)
	}
	if c, ok := classifyChainedAssignModule(mod, opts.Source); ok {
		return buildChainedAssignModule(c, opts)
	}
	if a, ok := classifyAugAssignModule(mod, opts.Source); ok {
		return buildAugAssignModule(a, opts)
	}
	if b, ok := classifyBinOpAssignModule(mod, opts.Source); ok {
		return buildBinOpAssignModule(b, opts)
	}
	if u, ok := classifyUnaryAssignModule(mod, opts.Source); ok {
		return buildUnaryAssignModule(u, opts)
	}
	if c, ok := classifyCmpAssignModule(mod, opts.Source); ok {
		return buildCmpAssignModule(c, opts)
	}
	if b, ok := classifyBoolOpAssignModule(mod, opts.Source); ok {
		return buildBoolOpAssignModule(b, opts)
	}
	if t, ok := classifyTernaryAssignModule(mod, opts.Source); ok {
		return buildTernaryAssignModule(t, opts)
	}
	if c, ok := classifyCollectionModule(mod, opts.Source); ok {
		return buildCollectionModule(c, opts)
	}
	return nil, ErrUnsupported
}
