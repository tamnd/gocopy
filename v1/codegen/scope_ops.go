package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
	"github.com/tamnd/gocopy/v1/symtable"
)

// nameOpKind classifies how a Name should be lowered in the
// surrounding compileUnit. It mirrors the COMPILE_OP_FAST /
// COMPILE_OP_DEREF / COMPILE_OP_GLOBAL / COMPILE_OP_NAME branches of
// CPython 3.14 Python/codegen.c::compiler_nameop.
//
// A kind is paired with a uint32 argument by resolveNameOp:
//
//   - nameOpFast    — local / param slot index (LocalsPlusNames index)
//   - nameOpDeref   — cell / free slot index   (LocalsPlusNames index)
//   - nameOpGlobal  — co_names index           (caller shifts << 1
//                                                and ORs the
//                                                push_null bit for
//                                                LOAD_GLOBAL call
//                                                targets)
//   - nameOpModule  — co_names index           (LOAD_NAME / STORE_NAME)
//
// SOURCE: CPython 3.14 Python/codegen.c::compiler_nameop.
type nameOpKind int

const (
	nameOpFast nameOpKind = iota
	nameOpDeref
	nameOpGlobal
	nameOpModule
)

// resolveNameOp picks the dispatch family for name in u's Scope and
// returns the matching slot or names-table index. Mirrors
// CPython 3.14 Python/codegen.c::compiler_nameop's classification
// half — the half that runs before the Load/Store/Del switch.
//
// Resolution order (matches CPython):
//
//  1. If u.Scope is nil (legacy path or pre-v0.7.10 single-arg
//     callers), or u.Scope is a module scope, fall through to the
//     module branch — every name resolves to LOAD_NAME / STORE_NAME.
//  2. If the name is bound in the current scope as Param, Local, or
//     a Cell that started life as a param/local (i.e. SymCell with
//     SymParam or SymLocal also set, OR plain SymLocal): treat as
//     fast or deref depending on the cell flag.
//  3. If the name is in u.Scope.Frees: deref against the slot table
//     position for free variables.
//  4. If the name is bound as Global / GlobalImplicit / Builtin in
//     the current scope: global.
//  5. Otherwise (a name first observed in this function with no
//     binding flag in the current scope) the lookup falls back to
//     the parent symbol-table chain and emits LOAD_GLOBAL — this is
//     CPython's COMPILE_OP_GLOBAL fallback.
//
// The slot indices for nameOpFast / nameOpDeref are the ones stamped
// on the Symbol by Scope.LocalsPlusNames(). The caller is expected
// to have already invoked LocalsPlusNames on the scope (as
// codegen_function_body does in CPython); doing so once when the
// compileUnit is pushed is fine.
func (u *compileUnit) resolveNameOp(name string) (nameOpKind, uint32) {
	scope := u.Scope
	if scope == nil || scope.Kind == symtable.ScopeModule {
		return nameOpModule, u.addName(name)
	}
	if sym, ok := scope.Symbols[name]; ok {
		switch {
		case sym.Flags.HasAny(symtable.SymCell | symtable.SymFree):
			// Cell and free both lower to LOAD_DEREF / STORE_DEREF —
			// the slot index is the LocalsPlus position. SymFree must
			// win over SymLocal here: a `nonlocal x; x = 2` binding
			// has both flags set, but the deref must run against the
			// parent's cell, not the fast slot.
			return nameOpDeref, uint32(sym.Index)
		case sym.Flags.HasAny(symtable.SymParam | symtable.SymLocal):
			return nameOpFast, uint32(sym.Index)
		case sym.Flags.HasAny(symtable.SymGlobal | symtable.SymGlobalImplicit | symtable.SymBuiltin):
			return nameOpGlobal, u.addName(name)
		}
	}
	return nameOpGlobal, u.addName(name)
}

// emitNameLoad appends the appropriate LOAD_* instruction for name
// at loc into u's current block. For nameOpFast the instruction is
// plain LOAD_FAST; the LOAD_FAST → LOAD_FAST_BORROW promotion is
// owned by the optimize_load_fast pass
// (compiler/flowgraph/optimize_load_fast.go).
//
// For nameOpGlobal the oparg is the names-table index shifted left
// by 1 with the low bit cleared (push_null = 0). Call-target uses
// (LOAD_GLOBAL whose result is consumed by the next CALL) need the
// low bit set; callers in that context use emitNameLoadCall instead.
//
// SOURCE: CPython 3.14 Python/codegen.c::compiler_nameop, Load
// branch.
func (u *compileUnit) emitNameLoad(name string, loc bytecode.Loc) {
	kind, arg := u.resolveNameOp(name)
	op := bytecode.LOAD_NAME
	switch kind {
	case nameOpFast:
		op = bytecode.LOAD_FAST
	case nameOpDeref:
		op = bytecode.LOAD_DEREF
	case nameOpGlobal:
		op = bytecode.LOAD_GLOBAL
		arg = arg << 1
	case nameOpModule:
		op = bytecode.LOAD_NAME
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{Op: op, Arg: arg, Loc: loc})
}

// emitNameLoadCall is the call-target variant of emitNameLoad. For
// nameOpGlobal the LOAD_GLOBAL oparg gets its low bit set
// (push_null = 1) so the runtime pushes the implicit NULL the
// CALL opcode expects between callable and arg0. For every other
// kind the emit is identical to emitNameLoad — including the
// nameOpFast arm, which emits plain LOAD_FAST and defers the
// LOAD_FAST_BORROW promotion to the optimize_load_fast pass
// (compiler/flowgraph/optimize_load_fast.go).
//
// SOURCE: CPython 3.14 Python/codegen.c::compiler_load_global with
// push_null=1.
func (u *compileUnit) emitNameLoadCall(name string, loc bytecode.Loc) {
	kind, arg := u.resolveNameOp(name)
	op := bytecode.LOAD_NAME
	switch kind {
	case nameOpFast:
		op = bytecode.LOAD_FAST
	case nameOpDeref:
		op = bytecode.LOAD_DEREF
	case nameOpGlobal:
		op = bytecode.LOAD_GLOBAL
		arg = (arg << 1) | 1
	case nameOpModule:
		op = bytecode.LOAD_NAME
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{Op: op, Arg: arg, Loc: loc})
}

// emitNameStore appends the appropriate STORE_* instruction for
// name at loc into u's current block. STORE_FAST stays STORE_FAST
// (no _BORROW variant exists for stores).
//
// SOURCE: CPython 3.14 Python/codegen.c::compiler_nameop, Store
// branch.
func (u *compileUnit) emitNameStore(name string, loc bytecode.Loc) {
	kind, arg := u.resolveNameOp(name)
	op := bytecode.STORE_NAME
	switch kind {
	case nameOpFast:
		op = bytecode.STORE_FAST
	case nameOpDeref:
		op = bytecode.STORE_DEREF
	case nameOpGlobal:
		op = bytecode.STORE_GLOBAL
	case nameOpModule:
		op = bytecode.STORE_NAME
	}
	block := u.currentBlock()
	block.Instrs = append(block.Instrs, ir.Instr{Op: op, Arg: arg, Loc: loc})
}
