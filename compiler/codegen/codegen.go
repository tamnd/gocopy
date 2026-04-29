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
// empty `[]`, `()`, and `{}` (dict) cases. v0.6.18 adds
// modSubscriptLoad and modAttrLoad: a single `<target> = <obj>[<key>]`
// or `<target> = <obj>.<attr>` where every operand is a Name (and
// `attr` is an identifier) of 1..15 ASCII chars on the same source
// line. modSubscriptLoad is the first codegen path to emit
// `BINARY_OP NbGetItem` and modAttrLoad is the first to emit
// `LOAD_ATTR` with a 10-code-unit run split 8+2 in the line table.
// v0.6.19 adds modSubscriptStore and modAttrStore: a single
// `<obj>[<key>] = <val>` or `<obj>.<attr> = <val>` where every
// operand is a Name (and `attr` is an identifier) of 1..15 ASCII
// chars on the same source line. These are the first codegen paths
// to emit `STORE_SUBSCR` (1 cache word) and `STORE_ATTR` (4 cache
// words). v0.6.20 adds modCallAssign: a single
// `<target> = <func>(<arg0>, ..., <argN-1>)` where target, func and
// every positional arg are Names of 1..15 ASCII chars on the same
// source line. N >= 0; keyword args and unpacking are out of scope.
// First codegen path to emit `PUSH_NULL` and `CALL` (3 cache words).
// v0.6.21 closes band A with modGenExpr: a single
// `<target> = <expr>` where the right-hand side is recursively
// composed of Name, small-int Constant (0..255), BinOp (any of the
// 13 supported operators), or UnaryOp (USub or Invert) — all on
// the same source line. First codegen path that is itself a
// recursive walker rather than a closed-form template, and the
// first to emit `LOAD_SMALL_INT`, `UNARY_NEGATIVE`, and
// `UNARY_INVERT`. v0.6.22 starts band B with modIfElse: a single
// top-level `if cond: name = val [elif cond: name = val ...]
// [else: name = val]` chain where every condition is a 1..15-char
// Name and every body is a single `name = small_int` (0..255)
// assignment. First codegen path that emits a multi-branch
// forward-jump pattern; the no-else implicit-return-None tail
// produces a LONG line-table entry pointing back to the first
// condition's source position. v0.6.23 adds modWhile: a single
// top-level `while cond: name = val` loop with no break/continue/
// else, where cond is a 1..15-char Name and val is a small int
// (0..255). First codegen path that emits a backward jump
// (`JUMP_BACKWARD 12`); the trailing implicit-return-None run is
// attributed back to the condition line, mirroring modIfElse.
// v0.6.24 closes band B with modFor: a single top-level
// `for loopVar in iter: bodyVar = val` loop with no break/continue/
// else, where iter and loopVar are 1..15-char Names and val is a
// small int (0..255). Adds the FOR_ITER + END_FOR + POP_ITER opcode
// trio; the trailing 4-cu loop-exit run is attributed back to the
// for line via a LONG line-table entry.
// v0.6.25 opens band C with modFuncDef: a single top-level
// `def f(arg): return arg` definition where f and arg are 1..15-char
// identifiers, the body is a single `return <argName>` statement,
// and there are no decorators/annotations/type-params/defaults/
// vararg/kwarg. First codegen path that emits a nested code object;
// the inner function CodeObject is hand-built mirroring the
// classifier's `compileFuncDef` byte-for-byte rather than routed
// through assemble.Assemble.
// v0.6.26 adds modClosureDef: a single top-level
// `def f(x): def g(): return x; return g` definition where f, x, and
// g are 1..15-char identifiers and the inner function captures x as
// a free variable. First codegen path that emits three nested code
// objects (module → outer → inner) and the first to exercise the
// cell + free closure-variable machinery (LocalsKindArgCell on the
// outer arg slot, LocalsKindFree on the inner). Hand-built mirroring
// the classifier's `compileClosure` byte-for-byte.
// Anything else returns ErrUnsupported and the caller falls back to
// the classifier.
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
	if s, ok := classifySubscriptLoadModule(mod, opts.Source); ok {
		return buildSubscriptLoadModule(s, opts)
	}
	if a, ok := classifyAttrLoadModule(mod, opts.Source); ok {
		return buildAttrLoadModule(a, opts)
	}
	if s, ok := classifySubscriptStoreModule(mod, opts.Source); ok {
		return buildSubscriptStoreModule(s, opts)
	}
	if a, ok := classifyAttrStoreModule(mod, opts.Source); ok {
		return buildAttrStoreModule(a, opts)
	}
	if c, ok := classifyCallAssignModule(mod, opts.Source); ok {
		return buildCallAssignModule(c, opts)
	}
	if g, ok := classifyGenExprModule(mod, opts.Source); ok {
		return buildGenExprModule(g, opts)
	}
	if ie, ok := classifyIfElseModule(mod, opts.Source); ok {
		return buildIfElseModule(ie, opts)
	}
	if w, ok := classifyWhileModule(mod, opts.Source); ok {
		return buildWhileModule(w, opts)
	}
	if f, ok := classifyForModule(mod, opts.Source); ok {
		return buildForModule(f, opts)
	}
	if f, ok := classifyFuncDefModule(mod, opts.Source); ok {
		return buildFuncDefModule(f, opts)
	}
	if c, ok := classifyClosureDefModule(mod, opts.Source); ok {
		return buildClosureDefModule(c, opts)
	}
	return nil, ErrUnsupported
}
