// Package codegen lowers a gocopy AST plus its symbol-table scope to
// a fully-formed *bytecode.CodeObject by emitting an InstrSeq IR and
// running it through the v0.6.5 assembler.
//
// At v0.7.1 the modEmpty/modNoOps/modDocstring shapes — the v0.6.6,
// v0.6.7, and v0.6.8 entries — were promoted to the visitor pipeline
// (compiler/codegen.Generate); they are no longer reachable through
// codegen.Build. At v0.7.2 the simple-constant modAssign,
// modMultiAssign, modChainedAssign, modAugAssign, modBinOpAssign,
// and modUnaryAssign shapes (the v0.6.9-v0.6.13 entries) were
// promoted to the visitor pipeline as well; their classifier files
// are gone and Build no longer dispatches them. v0.7.4 promotes
// modCmpAssign, modBoolOp, and modTernary (the v0.6.14-v0.6.16
// entries) to the visitor pipeline together with the first two
// real optimizer passes (inline_small_exit_blocks and
// resolve_jumps); their classifier files are gone and Build no
// longer dispatches them. v0.7.5 closes phase A by promoting
// modCollection, modSubscriptLoad, modSubscriptStore, modAttrLoad,
// modAttrStore, modCallAssign, and modGenExpr (the v0.6.17-v0.6.21
// entries) to the visitor pipeline as cases of the new central
// `visitExpr` recursive dispatcher; their classifier files and
// Build dispatch arms are gone. v0.6.22 starts band B with
// modIfElse: a single top-level
// `if cond: name = val [elif cond: name = val ...]
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
// ErrUnsupported when the source falls outside the codegen surface.
func Build(mod *ast.Module, scope *symtable.Scope, opts Options) (*bytecode.CodeObject, error) {
	if mod == nil {
		return nil, errors.New("codegen.Build: nil module")
	}
	_ = scope // reserved for the function-codegen release

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
