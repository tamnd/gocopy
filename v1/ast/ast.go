// Package ast is the gocopy-owned AST surface that compiler internals
// consume. It is the analogue of CPython 3.14's Python/Python-ast.c
// shape: codegen and the symbol table speak ast types, not parser
// types.
//
// At v0.6.2 the package is intentionally a thin alias layer over
// github.com/tamnd/gopapy/parser. Every type below is a Go type
// alias, so identity is preserved and the parser's
// ParseFile-produced values can be handed directly to the compiler
// after a single trip through compiler/lower.
//
// Aliasing buys two things now and one thing later:
//
//   - Now: every compiler/* file flips its import from
//     gopapy/parser to gocopy/compiler/ast. The contract "compiler
//     consumes *ast.Module, parser produces *parser.Module" is real
//     and enforced by an internal_test invariant.
//   - Later: when codegen needs to attach derived data (Symbol,
//     ConstRef) to nodes, individual aliases get replaced by
//     concrete gocopy types, one type at a time, without touching
//     every call site in one PR.
//
// SOURCE: CPython 3.14 Python/Python-ast.c (asdl-generated AST
// definitions) and Parser/Python.asdl.
package ast

import parser "github.com/tamnd/gopapy/parser"

// Position type. Compiler diagnostics carry Pos through the same
// channel as parser errors, so the alias keeps the loss-free path.
type Pos = parser.Pos

// Top-level module and the two big interfaces.
type (
	Module = parser.Module
	Stmt   = parser.Stmt
	Expr   = parser.Expr
)

// Expression nodes.
type (
	Constant       = parser.Constant
	Name           = parser.Name
	UnaryOp        = parser.UnaryOp
	BinOp          = parser.BinOp
	BoolOp         = parser.BoolOp
	Compare        = parser.Compare
	Attribute      = parser.Attribute
	Subscript      = parser.Subscript
	Slice          = parser.Slice
	Call           = parser.Call
	Keyword        = parser.Keyword
	List           = parser.List
	Tuple          = parser.Tuple
	Set            = parser.Set
	Dict           = parser.Dict
	Comprehension  = parser.Comprehension
	ListComp       = parser.ListComp
	SetComp        = parser.SetComp
	DictComp       = parser.DictComp
	GeneratorExp   = parser.GeneratorExp
	Lambda         = parser.Lambda
	Arguments      = parser.Arguments
	Arg            = parser.Arg
	NamedExpr      = parser.NamedExpr
	Starred        = parser.Starred
	Await          = parser.Await
	Yield          = parser.Yield
	YieldFrom      = parser.YieldFrom
	JoinedStr      = parser.JoinedStr
	FormattedValue = parser.FormattedValue
	TemplateStr    = parser.TemplateStr
	Interpolation  = parser.Interpolation
	IfExp          = parser.IfExp
	TypeIgnore     = parser.TypeIgnore
)

// Statement nodes.
type (
	ExprStmt         = parser.ExprStmt
	Assign           = parser.Assign
	AugAssign        = parser.AugAssign
	AnnAssign        = parser.AnnAssign
	Return           = parser.Return
	Raise            = parser.Raise
	Pass             = parser.Pass
	Break            = parser.Break
	Continue         = parser.Continue
	Alias            = parser.Alias
	Import           = parser.Import
	ImportFrom       = parser.ImportFrom
	Global           = parser.Global
	Nonlocal         = parser.Nonlocal
	Delete           = parser.Delete
	Assert           = parser.Assert
	If               = parser.If
	While            = parser.While
	For              = parser.For
	AsyncFor         = parser.AsyncFor
	ExceptHandler    = parser.ExceptHandler
	Try              = parser.Try
	TryStar          = parser.TryStar
	WithItem         = parser.WithItem
	With             = parser.With
	AsyncWith        = parser.AsyncWith
	FunctionDef      = parser.FunctionDef
	AsyncFunctionDef = parser.AsyncFunctionDef
	ClassDef         = parser.ClassDef
	TypeAlias        = parser.TypeAlias
)

// Type-parameter nodes (PEP 695).
type (
	TypeParam    = parser.TypeParam
	TypeVar      = parser.TypeVar
	TypeVarTuple = parser.TypeVarTuple
	ParamSpec    = parser.ParamSpec
)

// Match-statement nodes (PEP 634).
type (
	Match          = parser.Match
	MatchCase      = parser.MatchCase
	Pattern        = parser.Pattern
	MatchValue     = parser.MatchValue
	MatchSingleton = parser.MatchSingleton
	MatchSequence  = parser.MatchSequence
	MatchMapping   = parser.MatchMapping
	MatchClass     = parser.MatchClass
	MatchStar      = parser.MatchStar
	MatchAs        = parser.MatchAs
	MatchOr        = parser.MatchOr
)
