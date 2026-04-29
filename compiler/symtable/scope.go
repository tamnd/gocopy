// Package symtable builds CPython-parity symbol tables from a
// gopapy AST.
//
// The package mirrors the role of CPython's Python/symtable.c: it
// walks the parsed AST, produces a tree of Scopes (one per
// module/function/class/comprehension), and resolves every name to
// a SymbolKind matching CPython's DEF_* / USE flags. Codegen
// consults the resulting tree to emit LOAD_FAST / LOAD_DEREF /
// LOAD_GLOBAL and to assemble co_localsplusnames /
// co_localspluskinds.
//
// v0.6.1 implements module + function scopes. Class and
// comprehension scopes are stub-only: constructing one panics with
// a clear "not yet supported" message until v0.6.11/v0.6.12 land.
//
// SOURCE: CPython 3.14 Python/symtable.c and
// Include/internal/pycore_symtable.h.
package symtable

import (
	"fmt"

	"github.com/tamnd/gocopy/bytecode"
)

// ScopeKind names the four scope flavours CPython distinguishes via
// _Py_block_ty in Include/internal/pycore_symtable.h.
type ScopeKind uint8

const (
	ScopeModule ScopeKind = iota
	ScopeFunction
	ScopeClass         // stub; v0.6.11
	ScopeComprehension // stub; v0.6.12
)

func (k ScopeKind) String() string {
	switch k {
	case ScopeModule:
		return "module"
	case ScopeFunction:
		return "function"
	case ScopeClass:
		return "class"
	case ScopeComprehension:
		return "comprehension"
	default:
		return fmt.Sprintf("ScopeKind(%d)", uint8(k))
	}
}

// SymbolKind is the bitset of properties CPython tracks for each
// name in a scope. It mirrors the DEF_* / USE / SCOPE_* macros in
// Include/internal/pycore_symtable.h.
//
// The bits split into three groups:
//   - definition kind: SymParam, SymLocal, SymCell, SymFree,
//     SymGlobal, SymGlobalImplicit, SymBuiltin
//   - usage record: SymAssigned, SymUsed, SymAnnotated
//   - module-level marker: SymStarImport
//
// Multiple bits combine: a positional argument captured by a nested
// scope has SymParam | SymLocal | SymCell. A name that is both
// assigned and read has SymAssigned | SymUsed.
type SymbolKind uint16

const (
	SymParam SymbolKind = 1 << iota
	SymLocal
	SymCell
	SymFree
	SymGlobal         // declared `global`
	SymGlobalImplicit // referenced but not bound anywhere reachable
	SymBuiltin        // resolves to a builtin
	SymAssigned       // bound (assignment, augassign, for-target, with-as, ...)
	SymUsed           // referenced as a load
	SymAnnotated      // appeared as an AnnAssign target
	SymStarImport     // module scope only: from X import * was seen
)

// Has reports whether all bits in mask are set.
func (k SymbolKind) Has(mask SymbolKind) bool { return k&mask == mask }

// HasAny reports whether any bit in mask is set.
func (k SymbolKind) HasAny(mask SymbolKind) bool { return k&mask != 0 }

// Symbol is one named entry within a Scope. Index is the slot
// position within the scope's ordered locals table (computed by
// LocalsPlusNames).
type Symbol struct {
	Name  string
	Flags SymbolKind
	Index int    // -1 until LocalsPlusNames is computed
	Scope *Scope // owning scope (back-pointer, set by Scope.Define)
}

// Scope is one scope in the symbol-table tree. Module is the root;
// Functions / Classes / Comprehensions hang off as Children in
// source order.
//
// Symbols is the unordered map keyed by name. OrderedNames preserves
// the source-order definition sequence for each name's first
// appearance — this is the basis for slot ordering.
//
// Params, Cells, Frees are slot-ordered name slices populated by
// finalize(). Globals lists names declared `global` or resolved as
// implicit globals in this scope.
type Scope struct {
	Kind     ScopeKind
	Name     string
	QualName string
	Parent   *Scope
	Children []*Scope

	Symbols      map[string]*Symbol
	OrderedNames []string

	Params  []string // positional + *args + kw-only + **kwargs in declaration order
	Cells   []string // names that need a cell in this scope
	Frees   []string // names that resolve to a cell in an enclosing scope
	Globals []string

	// Loc is the source span of the scope's defining node (the def
	// keyword for functions, the entire module for the root).
	Loc bytecode.Loc

	// HasVararg / HasKwarg / KwOnlyCount / PosOnlyCount mirror the
	// fields of Arguments; codegen needs these to reconstruct
	// ArgCount and friends.
	PosOnlyCount int
	ArgCount     int // positional+kw, excluding *args/**kwargs/kw-only
	KwOnlyCount  int
	HasVararg    bool
	HasKwarg     bool
}

// NewScope constructs a Scope of the given kind. ScopeClass and
// ScopeComprehension panic until v0.6.11/v0.6.12 implement them; no
// fixture in the current 246 reaches this path, and the panic makes
// the gap obvious if a new fixture does.
func NewScope(kind ScopeKind, name string, parent *Scope) *Scope {
	switch kind {
	case ScopeClass:
		panic("symtable: ScopeClass not yet supported (v0.6.11)")
	case ScopeComprehension:
		panic("symtable: ScopeComprehension not yet supported (v0.6.12)")
	}
	s := &Scope{
		Kind:    kind,
		Name:    name,
		Parent:  parent,
		Symbols: map[string]*Symbol{},
	}
	if parent != nil {
		parent.Children = append(parent.Children, s)
	}
	return s
}

// Define ensures a Symbol exists for name in this scope and ORs the
// given flags into it. Returns the symbol. The first Define for a
// given name records the source order via OrderedNames.
func (s *Scope) Define(name string, flags SymbolKind) *Symbol {
	sym, ok := s.Symbols[name]
	if !ok {
		sym = &Symbol{Name: name, Index: -1, Scope: s}
		s.Symbols[name] = sym
		s.OrderedNames = append(s.OrderedNames, name)
	}
	sym.Flags |= flags
	return sym
}

// Lookup returns the symbol for name in this scope, or nil if absent.
func (s *Scope) Lookup(name string) *Symbol {
	return s.Symbols[name]
}

// Resolve walks parent function scopes looking for one that binds
// name as a Param / Local / Cell. It returns the symbol and its
// owning scope, or (nil, nil) if no enclosing function binds the
// name. Module and class scopes are skipped: module-level
// assignments do not promote nested-function reads to free
// variables (those resolve as LOAD_GLOBAL), and CPython's
// free-variable rules treat class scopes as transparent.
func (s *Scope) Resolve(name string) (*Symbol, *Scope) {
	for cur := s.Parent; cur != nil; cur = cur.Parent {
		if cur.Kind != ScopeFunction {
			continue
		}
		if sym, ok := cur.Symbols[name]; ok && sym.Flags.HasAny(SymParam|SymLocal|SymCell) {
			return sym, cur
		}
	}
	return nil, nil
}

// LocalsPlusNames returns the slot-ordered name table that becomes
// CodeObject.LocalsPlusNames. Order matches CPython:
//  1. Params in declaration order (positional, *args, kw-only,
//     **kwargs).
//  2. Plain locals (SymLocal without SymParam) in source-definition
//     order.
//  3. Cells (SymCell without SymParam — params that are also cells
//     stay in slot 1's group).
//  4. Frees in source-definition order.
//
// Indices are stamped onto each Symbol.Index as a side effect so
// codegen can look up `slot := scope.Symbols[name].Index` later.
func (s *Scope) LocalsPlusNames() []string {
	out := make([]string, 0, len(s.OrderedNames))

	// 1. Params in declaration order.
	for _, p := range s.Params {
		out = append(out, p)
	}

	// 2. Plain locals: in OrderedNames, anything that is SymLocal
	//    and not a Param and not in Frees.
	freeSet := map[string]bool{}
	for _, f := range s.Frees {
		freeSet[f] = true
	}
	paramSet := map[string]bool{}
	for _, p := range s.Params {
		paramSet[p] = true
	}
	for _, n := range s.OrderedNames {
		if paramSet[n] || freeSet[n] {
			continue
		}
		sym := s.Symbols[n]
		if sym.Flags.HasAny(SymLocal | SymCell) {
			out = append(out, n)
		}
	}

	// 3. Frees in their stored order.
	for _, f := range s.Frees {
		out = append(out, f)
	}

	// Stamp Index on each symbol.
	for i, n := range out {
		if sym, ok := s.Symbols[n]; ok {
			sym.Index = i
		}
	}

	return out
}

// LocalsPlusKinds returns the slot-ordered byte table that becomes
// CodeObject.LocalsPlusKinds. Each byte is the OR of FastX bits
// matching the symbol's flags:
//
//   - posonly param:   FastLocal | FastArgPos             (0x22)
//   - regular param:   FastLocal | FastArg                (0x26, LocalsKindArg)
//   - kwonly param:    FastLocal | FastArgKw              (0x24)
//   - *args param:     FastLocal | FastArgPos | FastArgVar (0x2a)
//   - **kwargs param:  FastLocal | FastArgKw  | FastArgVar (0x2c)
//   - any param + cell adds FastCell (0x40) onto the bits above
//   - plain local:     FastLocal                          (LocalsKindLocal)
//   - cell, not param: FastCell                           (LocalsKindCell)
//   - free:            FastFree                           (LocalsKindFree)
//
// The slice length and ordering match LocalsPlusNames; call
// LocalsPlusNames first or as part of the same pass.
//
// SOURCE: CPython 3.14 Python/compile.c compute_localsplus_info +
// Include/internal/pycore_code.h CO_FAST_* flags.
func (s *Scope) LocalsPlusKinds() []byte {
	names := s.LocalsPlusNames()
	out := make([]byte, len(names))
	freeSet := map[string]bool{}
	for _, f := range s.Frees {
		freeSet[f] = true
	}
	// Compute each param's arg-kind bits from its position in Params.
	// Params is laid out [PosOnly | Regular | KwOnly | Vararg | Kwarg]
	// (see bindArgs in build.go).
	paramKind := map[string]byte{}
	varargIdx := s.ArgCount + s.KwOnlyCount
	kwargIdx := varargIdx
	if s.HasVararg {
		kwargIdx++
	}
	for i, p := range s.Params {
		var argBits byte
		switch {
		case i < s.PosOnlyCount:
			argBits = bytecode.FastArgPos
		case i < s.ArgCount:
			argBits = bytecode.FastArgPos | bytecode.FastArgKw
		case i < s.ArgCount+s.KwOnlyCount:
			argBits = bytecode.FastArgKw
		case s.HasVararg && i == varargIdx:
			argBits = bytecode.FastArgPos | bytecode.FastArgVar
		case s.HasKwarg && i == kwargIdx:
			argBits = bytecode.FastArgKw | bytecode.FastArgVar
		}
		paramKind[p] = bytecode.FastLocal | argBits
	}
	for i, n := range names {
		sym := s.Symbols[n]
		bits, isParam := paramKind[n]
		switch {
		case freeSet[n]:
			out[i] = bytecode.LocalsKindFree
		case isParam && sym.Flags.Has(SymCell):
			out[i] = bits | bytecode.FastCell
		case isParam:
			out[i] = bits
		case sym.Flags.Has(SymCell):
			out[i] = bytecode.LocalsKindCell
		default:
			out[i] = bytecode.LocalsKindLocal
		}
	}
	return out
}

// Walk invokes fn on this scope, then on every descendant in
// pre-order source sequence. Used by tests and codegen wiring.
func (s *Scope) Walk(fn func(*Scope)) {
	fn(s)
	for _, c := range s.Children {
		c.Walk(fn)
	}
}
