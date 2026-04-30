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
// Build dispatch arms are gone. v0.7.6 opens phase B by promoting
// modIfElse — a single top-level
// `if cond: name = val [elif cond: name = val ...]
// [else: name = val]` chain — to the visitor pipeline as
// visitIfStmt; the v0.6.22 codegen classifier file
// (visit_if_else.go) and Build dispatch arm are gone. visitIfStmt
// is the first visitor that emits a multi-block CFG (one entry
// block, one body block per branch, optional kept-merge end block)
// and the first to drive the optimize pipeline through a real
// branching shape (eliminate_empty_blocks +
// inline_small_exit_blocks). v0.7.7 promotes modWhile — a single
// top-level `while cond: name = val` loop with no break/continue/
// else, where cond is a 1..15-char Name and val is a small int
// (0..255) — to the visitor pipeline as visitWhileStmt; the
// v0.6.23 codegen classifier file (visit_while.go) and Build
// dispatch arm are gone. visitWhileStmt is the first visitor that
// emits a backward jump (JUMP_BACKWARD), and v0.7.7 lifts the
// resolveJumps tripwire that previously panicked on backward
// targets. v0.7.8 closes phase B by promoting modFor — a single
// top-level `for loopVar in iter: bodyVar = val` loop with no
// break/continue/else, where iter and loopVar are 1..15-char Names
// and val is a small int (0..255) — to the visitor pipeline as
// visitForStmt; the v0.6.24 codegen classifier file (visit_for.go)
// and Build dispatch arm are gone. visitForStmt is the first visitor
// that emits FOR_ITER + GET_ITER + END_FOR + POP_ITER (the iterator
// protocol opcodes) and the first whose CFG has both a forward
// (FOR_ITER → exit) and a backward (JUMP_BACKWARD → top) labelled
// edge sharing one body block.
// v0.7.8 opens band C by promoting modFuncDef — a single top-level
// `def f(arg): return arg` definition where f and arg are 1..15-char
// identifiers, the body is a single `return <argName>` statement,
// and there are no decorators/annotations/type-params/defaults/
// vararg/kwarg — to the visitor pipeline as visitFunctionDef; the
// v0.6.25 codegen classifier file (visit_funcdef.go) and Build
// dispatch arm are gone. visitFunctionDef is the first visitor that
// pushes a nested compileUnit (compiler_enter_scope /
// compiler_exit_scope) and the first to route a non-module unit
// through optimize.Run + assemble.Assemble. It is also the first
// visitor that emits LOAD_FAST_BORROW + MAKE_FUNCTION.
// v0.7.9 closes phase B by promoting modClosureDef — a single
// top-level `def f(x): def g(): return x; return g` closure — to
// the visitor pipeline as visitClosureDef. The v0.6.26 codegen
// classifier file (visit_closuredef.go) is gone; Build's dispatch
// is now empty and returns ErrUnsupported unconditionally.
// visitClosureDef is the first visitor that exercises a doubly-
// nested compileUnit (module → outer → inner) and the first to
// emit MAKE_CELL, COPY_FREE_VARS, LOAD_DEREF, BUILD_TUPLE,
// SET_FUNCTION_ATTRIBUTE, and STORE_FAST.
// v0.7.10 opens the general-function-body surface as
// visitFuncBodyDef (visit_funcbody_stmt.go) plus its statement
// and expression dispatchers visitFuncStmt / visitFuncExpr
// (visit_func_stmt.go) and the scope-driven name-resolution
// helpers in scope_ops.go. The FunctionDef arm of visitStmt now
// tries visitFuncBodyDef first, then visitClosureDef, then the
// v0.7.8 visitFunctionDef. The visitor catches multi-stmt
// Assign / AugAssign / If / Return / bare-Call bodies over
// BinOp / UnaryOp / Compare / Call / Attribute / Subscript /
// Tuple / IfExp / Constant / Name expressions, with elif chains
// and terminating if-else as the last statement. Visitor parity
// climbs from 142/246 to 205/246. compiler/func_body.go (the
// 1,516-LOC v0.6 recursive expression compiler) stays alive for
// the long tail; its funeral moves to the v0.7.10.x systematic
// series that lifts the visitor's emit-time LOAD_FAST_BORROW
// classification and LFLBLFLB super-instruction fusion into
// real post-emit passes mirroring CPython's optimize_load_fast
// and insert_superinstructions.
// v0.7.10.1 opens the v0.7.10.x systematic series with a
// plumbing-only release: tests/fixtures/funcbody/ is created,
// tests/run.sh and TestVisitorParity walk it as a flat second
// pass, and 8 baseline fixtures land. Visitor parity climbs
// to 213/254 (all 8 new baselines route through the visitor).
// v0.7.10.2 collapses the per-shape If helpers
// (validateFuncBodyIf / validateFuncBodyTerminatingIf /
// emitFuncBodyIf / emitFuncBodyTerminatingIf) into a single
// recursive codegenIf + codegenJumpIf + validateIfStmt in
// visit_if.go, mirroring CPython 3.14 Python/codegen.c::codegen_if
// and codegen_jump_if. First v0.7.10.x release with explicit
// `// MIRRORS:` 1:1 port headers. No fixture coverage change.
// v0.7.10.3 ports CPython's insert_superinstructions
// (Python/flowgraph.c:2588) into compiler/flowgraph/. Visitor
// stops pre-fusing LFLBLFLB pairs at emit time; the new pass
// runs inside optimize.Run between inlineSmallOrNoLinenoBlocks and
// resolveJumps. fuseLflblflbTail is deleted from visit_func_stmt.go.
// Anything reaching codegen.Build now returns ErrUnsupported and
// the caller falls back to the classifier.
//
// SOURCE: CPython 3.14 Python/codegen.c.
package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/symtable"
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
	_ = opts
	return nil, ErrUnsupported
}
