package symtable

import (
	"fmt"
	"slices"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
)

// UnsupportedScopeError reports a scope shape the v0.6.1 symtable
// does not yet handle. Class scopes land in v0.6.11, comprehension
// scopes in v0.6.12, lambdas/async in their own slot. The error
// names the AST node so the offending fixture is obvious.
type UnsupportedScopeError struct {
	Kind string
	Pos  ast.Pos
}

func (e *UnsupportedScopeError) Error() string {
	return fmt.Sprintf("symtable: unsupported scope %s at line %d col %d", e.Kind, e.Pos.Line, e.Pos.Col)
}

// Build constructs the symbol-table tree for a module AST.
//
// Two passes mirror CPython's _PySymtable_Build:
//
//  1. Definition pass — visit every node, push a Scope for each
//     FunctionDef, record assignments / params / def-names as
//     definitions, record Name-loads as uses. Also tracks Global
//     and Nonlocal declarations.
//  2. Analysis pass — for each function scope, classify every
//     symbol into local / cell / free / global / global-implicit;
//     propagate cell promotion to enclosing scopes; populate the
//     ordered Params / Cells / Frees / Globals slices.
//
// On unsupported scope shapes (class, comprehension, lambda,
// async), Build returns an UnsupportedScopeError.
func Build(mod *ast.Module) (*Scope, error) {
	root := NewScope(ScopeModule, "", nil)

	b := &builder{root: root}
	if err := b.visitStmts(root, mod.Body); err != nil {
		return nil, err
	}

	if err := b.analyze(root); err != nil {
		return nil, err
	}
	finalizeQualnames(root)
	return root, nil
}

type builder struct {
	root *Scope
}

func (b *builder) visitStmts(s *Scope, stmts []ast.Stmt) error {
	for _, st := range stmts {
		if err := b.visitStmt(s, st); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) visitStmt(s *Scope, st ast.Stmt) error {
	switch n := st.(type) {
	case *ast.Pass, *ast.Break, *ast.Continue:
		return nil

	case *ast.ExprStmt:
		return b.visitExpr(s, n.Value)

	case *ast.Assign:
		if err := b.visitExpr(s, n.Value); err != nil {
			return err
		}
		for _, t := range n.Targets {
			if err := b.visitTarget(s, t); err != nil {
				return err
			}
		}
		return nil

	case *ast.AugAssign:
		if err := b.visitExpr(s, n.Value); err != nil {
			return err
		}
		// AugAssign target is both used and assigned. visitTarget
		// would only set SymAssigned, so add SymUsed for Name targets.
		if name, ok := n.Target.(*ast.Name); ok {
			s.Define(name.Id, SymAssigned|SymUsed)
			return nil
		}
		return b.visitTarget(s, n.Target)

	case *ast.AnnAssign:
		if n.Annotation != nil {
			if err := b.visitExpr(s, n.Annotation); err != nil {
				return err
			}
		}
		if n.Value != nil {
			if err := b.visitExpr(s, n.Value); err != nil {
				return err
			}
		}
		if name, ok := n.Target.(*ast.Name); ok {
			flags := SymAnnotated
			if n.Value != nil {
				flags |= SymAssigned
			}
			s.Define(name.Id, flags)
			return nil
		}
		return b.visitTarget(s, n.Target)

	case *ast.Return:
		if n.Value != nil {
			return b.visitExpr(s, n.Value)
		}
		return nil

	case *ast.Raise:
		if n.Exc != nil {
			if err := b.visitExpr(s, n.Exc); err != nil {
				return err
			}
		}
		if n.Cause != nil {
			return b.visitExpr(s, n.Cause)
		}
		return nil

	case *ast.If:
		if err := b.visitExpr(s, n.Test); err != nil {
			return err
		}
		if err := b.visitStmts(s, n.Body); err != nil {
			return err
		}
		return b.visitStmts(s, n.Orelse)

	case *ast.While:
		if err := b.visitExpr(s, n.Test); err != nil {
			return err
		}
		if err := b.visitStmts(s, n.Body); err != nil {
			return err
		}
		return b.visitStmts(s, n.Orelse)

	case *ast.For:
		if err := b.visitExpr(s, n.Iter); err != nil {
			return err
		}
		if err := b.visitTarget(s, n.Target); err != nil {
			return err
		}
		if err := b.visitStmts(s, n.Body); err != nil {
			return err
		}
		return b.visitStmts(s, n.Orelse)

	case *ast.Import:
		for _, a := range n.Names {
			bound := a.Asname
			if bound == "" {
				// `import a.b` binds `a` (the top-level package).
				bound = topName(a.Name)
			}
			s.Define(bound, SymAssigned)
		}
		return nil

	case *ast.ImportFrom:
		for _, a := range n.Names {
			if a.Name == "*" {
				if s.Kind == ScopeModule {
					// from X import * is only legal at module scope.
					// Mark the module as having a star-import; no
					// symbol gets defined.
					sym := s.Define("*", SymStarImport)
					_ = sym
					continue
				}
				return &UnsupportedScopeError{Kind: "from-import-star outside module", Pos: n.P}
			}
			bound := a.Asname
			if bound == "" {
				bound = a.Name
			}
			s.Define(bound, SymAssigned)
		}
		return nil

	case *ast.Global:
		for _, name := range n.Names {
			s.Define(name, SymGlobal)
		}
		return nil

	case *ast.Nonlocal:
		// Nonlocal is illegal at module scope; otherwise marks the
		// name as a free variable that must be bound in some
		// enclosing function scope.
		if s.Kind == ScopeModule {
			return &UnsupportedScopeError{Kind: "nonlocal at module scope", Pos: n.P}
		}
		for _, name := range n.Names {
			s.Define(name, SymFree)
		}
		return nil

	case *ast.FunctionDef:
		// Defaults / decorators / annotations are evaluated in the
		// enclosing scope.
		if err := b.visitArgsDefaults(s, n.Args); err != nil {
			return err
		}
		for _, d := range n.DecoratorList {
			if err := b.visitExpr(s, d); err != nil {
				return err
			}
		}
		if n.Returns != nil {
			if err := b.visitExpr(s, n.Returns); err != nil {
				return err
			}
		}
		// The function name is bound in the enclosing scope.
		s.Define(n.Name, SymAssigned)

		// Build the inner scope.
		inner := NewScope(ScopeFunction, n.Name, s)
		inner.Loc = bytecode.Loc{Line: uint32(n.P.Line), EndLine: uint32(n.P.Line)}
		if err := b.bindArgs(inner, n.Args); err != nil {
			return err
		}
		return b.visitStmts(inner, n.Body)

	case *ast.AsyncFunctionDef:
		return &UnsupportedScopeError{Kind: "async def", Pos: n.P}

	case *ast.ClassDef:
		return &UnsupportedScopeError{Kind: "class def", Pos: n.P}

	case *ast.Try, *ast.TryStar, *ast.With, *ast.AsyncWith,
		*ast.AsyncFor, *ast.Match, *ast.TypeAlias, *ast.Delete,
		*ast.Assert:
		return &UnsupportedScopeError{Kind: fmt.Sprintf("%T", st), Pos: stmtPos(st)}

	default:
		return &UnsupportedScopeError{Kind: fmt.Sprintf("%T", st), Pos: stmtPos(st)}
	}
}

func stmtPos(st ast.Stmt) ast.Pos {
	type poser interface{ getPos() ast.Pos }
	// Each Stmt struct has a P field. Cover the common ones; fall
	// through to zero Pos for unknowns.
	switch n := st.(type) {
	case *ast.Try:
		return n.P
	case *ast.TryStar:
		return n.P
	case *ast.With:
		return n.P
	case *ast.AsyncWith:
		return n.P
	case *ast.AsyncFor:
		return n.P
	case *ast.Match:
		return n.P
	case *ast.TypeAlias:
		return n.P
	case *ast.Delete:
		return n.P
	case *ast.Assert:
		return n.P
	}
	return ast.Pos{}
}

// visitArgsDefaults visits default-value expressions in the
// enclosing scope before the function scope is entered.
func (b *builder) visitArgsDefaults(s *Scope, args *ast.Arguments) error {
	if args == nil {
		return nil
	}
	for _, d := range args.Defaults {
		if err := b.visitExpr(s, d); err != nil {
			return err
		}
	}
	for _, d := range args.KwOnlyDef {
		if d == nil {
			continue
		}
		if err := b.visitExpr(s, d); err != nil {
			return err
		}
	}
	// Annotations on params are also evaluated in the enclosing scope.
	visit := func(a *ast.Arg) error {
		if a == nil || a.Annotation == nil {
			return nil
		}
		return b.visitExpr(s, a.Annotation)
	}
	for _, a := range args.PosOnly {
		if err := visit(a); err != nil {
			return err
		}
	}
	for _, a := range args.Args {
		if err := visit(a); err != nil {
			return err
		}
	}
	if err := visit(args.Vararg); err != nil {
		return err
	}
	for _, a := range args.KwOnly {
		if err := visit(a); err != nil {
			return err
		}
	}
	return visit(args.Kwarg)
}

// bindArgs records each parameter as SymParam|SymLocal in the
// function scope and populates Params, ArgCount, KwOnlyCount,
// PosOnlyCount, HasVararg, HasKwarg.
func (b *builder) bindArgs(s *Scope, args *ast.Arguments) error {
	if args == nil {
		return nil
	}
	add := func(a *ast.Arg) {
		s.Define(a.Name, SymParam|SymLocal|SymAssigned)
		s.Params = append(s.Params, a.Name)
	}
	for _, a := range args.PosOnly {
		add(a)
		s.PosOnlyCount++
		s.ArgCount++
	}
	for _, a := range args.Args {
		add(a)
		s.ArgCount++
	}
	if args.Vararg != nil {
		add(args.Vararg)
		s.HasVararg = true
	}
	for _, a := range args.KwOnly {
		add(a)
		s.KwOnlyCount++
	}
	if args.Kwarg != nil {
		add(args.Kwarg)
		s.HasKwarg = true
	}
	return nil
}

// visitTarget records every Name beneath an assignment target as
// SymAssigned|SymLocal. Tuple/List targets recurse. Attribute and
// Subscript targets visit their bases as uses (the base must
// already exist).
func (b *builder) visitTarget(s *Scope, t ast.Expr) error {
	switch n := t.(type) {
	case *ast.Name:
		s.Define(n.Id, SymAssigned|SymLocal)
		return nil
	case *ast.Tuple:
		for _, e := range n.Elts {
			if err := b.visitTarget(s, e); err != nil {
				return err
			}
		}
		return nil
	case *ast.List:
		for _, e := range n.Elts {
			if err := b.visitTarget(s, e); err != nil {
				return err
			}
		}
		return nil
	case *ast.Starred:
		return b.visitTarget(s, n.Value)
	case *ast.Attribute:
		return b.visitExpr(s, n.Value)
	case *ast.Subscript:
		if err := b.visitExpr(s, n.Value); err != nil {
			return err
		}
		return b.visitExpr(s, n.Slice)
	default:
		// Other shapes (e.g. NamedExpr, Constant) are illegal as
		// assignment targets; report them.
		return &UnsupportedScopeError{Kind: fmt.Sprintf("assignment target %T", t), Pos: exprPos(t)}
	}
}

// visitExpr records every Name read as SymUsed and recurses into
// child expressions. Lambda / comprehension / async-aware nodes are
// rejected as unsupported scope shapes.
func (b *builder) visitExpr(s *Scope, e ast.Expr) error {
	if e == nil {
		return nil
	}
	switch n := e.(type) {
	case *ast.Constant:
		return nil
	case *ast.Name:
		s.Define(n.Id, SymUsed)
		return nil
	case *ast.UnaryOp:
		return b.visitExpr(s, n.Operand)
	case *ast.BinOp:
		if err := b.visitExpr(s, n.Left); err != nil {
			return err
		}
		return b.visitExpr(s, n.Right)
	case *ast.BoolOp:
		for _, v := range n.Values {
			if err := b.visitExpr(s, v); err != nil {
				return err
			}
		}
		return nil
	case *ast.Compare:
		if err := b.visitExpr(s, n.Left); err != nil {
			return err
		}
		for _, c := range n.Comparators {
			if err := b.visitExpr(s, c); err != nil {
				return err
			}
		}
		return nil
	case *ast.Attribute:
		return b.visitExpr(s, n.Value)
	case *ast.Subscript:
		if err := b.visitExpr(s, n.Value); err != nil {
			return err
		}
		return b.visitExpr(s, n.Slice)
	case *ast.Slice:
		if err := b.visitExpr(s, n.Lower); err != nil {
			return err
		}
		if err := b.visitExpr(s, n.Upper); err != nil {
			return err
		}
		return b.visitExpr(s, n.Step)
	case *ast.Call:
		if err := b.visitExpr(s, n.Func); err != nil {
			return err
		}
		for _, a := range n.Args {
			if err := b.visitExpr(s, a); err != nil {
				return err
			}
		}
		for _, kw := range n.Keywords {
			if err := b.visitExpr(s, kw.Value); err != nil {
				return err
			}
		}
		return nil
	case *ast.List:
		for _, el := range n.Elts {
			if err := b.visitExpr(s, el); err != nil {
				return err
			}
		}
		return nil
	case *ast.Tuple:
		for _, el := range n.Elts {
			if err := b.visitExpr(s, el); err != nil {
				return err
			}
		}
		return nil
	case *ast.Set:
		for _, el := range n.Elts {
			if err := b.visitExpr(s, el); err != nil {
				return err
			}
		}
		return nil
	case *ast.Dict:
		for _, k := range n.Keys {
			if err := b.visitExpr(s, k); err != nil {
				return err
			}
		}
		for _, v := range n.Values {
			if err := b.visitExpr(s, v); err != nil {
				return err
			}
		}
		return nil
	case *ast.IfExp:
		if err := b.visitExpr(s, n.Test); err != nil {
			return err
		}
		if err := b.visitExpr(s, n.Body); err != nil {
			return err
		}
		return b.visitExpr(s, n.OrElse)
	case *ast.Starred:
		return b.visitExpr(s, n.Value)
	case *ast.NamedExpr:
		if err := b.visitExpr(s, n.Value); err != nil {
			return err
		}
		return b.visitTarget(s, n.Target)
	case *ast.JoinedStr:
		for _, v := range n.Values {
			if err := b.visitExpr(s, v); err != nil {
				return err
			}
		}
		return nil
	case *ast.FormattedValue:
		if err := b.visitExpr(s, n.Value); err != nil {
			return err
		}
		return b.visitExpr(s, n.FormatSpec)

	case *ast.Lambda, *ast.ListComp, *ast.SetComp, *ast.DictComp,
		*ast.GeneratorExp, *ast.Await, *ast.Yield, *ast.YieldFrom,
		*ast.TemplateStr, *ast.Interpolation:
		return &UnsupportedScopeError{Kind: fmt.Sprintf("%T", e), Pos: exprPos(e)}

	default:
		return &UnsupportedScopeError{Kind: fmt.Sprintf("%T", e), Pos: exprPos(e)}
	}
}

// exprPos extracts the source position from any concrete Expr by
// switching on the public types. The parser's pos() method is
// unexported, so external packages have to read the P field
// directly.
func exprPos(e ast.Expr) ast.Pos {
	switch n := e.(type) {
	case *ast.Constant:
		return n.P
	case *ast.Name:
		return n.P
	case *ast.UnaryOp:
		return n.P
	case *ast.BinOp:
		return n.P
	case *ast.BoolOp:
		return n.P
	case *ast.Compare:
		return n.P
	case *ast.Attribute:
		return n.P
	case *ast.Subscript:
		return n.P
	case *ast.Slice:
		return n.P
	case *ast.Call:
		return n.P
	case *ast.List:
		return n.P
	case *ast.Tuple:
		return n.P
	case *ast.Set:
		return n.P
	case *ast.Dict:
		return n.P
	case *ast.IfExp:
		return n.P
	case *ast.Starred:
		return n.P
	case *ast.NamedExpr:
		return n.P
	case *ast.JoinedStr:
		return n.P
	case *ast.FormattedValue:
		return n.P
	case *ast.Lambda:
		return n.P
	case *ast.ListComp:
		return n.P
	case *ast.SetComp:
		return n.P
	case *ast.DictComp:
		return n.P
	case *ast.GeneratorExp:
		return n.P
	case *ast.Await:
		return n.P
	case *ast.Yield:
		return n.P
	case *ast.YieldFrom:
		return n.P
	case *ast.TemplateStr:
		return n.P
	case *ast.Interpolation:
		return n.P
	}
	return ast.Pos{}
}

// analyze runs the second pass: classify every symbol into
// local / cell / free / global / global-implicit and populate the
// Cells / Frees / Globals slices on each scope. Walks bottom-up so
// child frees promote their bindings in the parent.
func (b *builder) analyze(s *Scope) error {
	// Recurse into children first so we know which names they pull
	// out as frees.
	for _, c := range s.Children {
		if err := b.analyze(c); err != nil {
			return err
		}
	}

	if s.Kind != ScopeFunction {
		return nil
	}

	for _, name := range s.OrderedNames {
		sym := s.Symbols[name]

		switch {
		case sym.Flags.Has(SymGlobal):
			s.Globals = append(s.Globals, name)

		case sym.Flags.Has(SymFree):
			// Either declared `nonlocal` or pulled up by a child.
			// In either case, find the binding in the enclosing
			// function scope and mark it cell.
			if _, owner := s.Resolve(name); owner != nil {
				promoteToCell(owner, name)
			}
			s.Frees = append(s.Frees, name)

		case sym.Flags.Has(SymParam):
			// Param: stays local (Cell promotion may have already
			// flipped it via promoteToCell in a child's analysis).

		case sym.Flags.HasAny(SymAssigned | SymLocal):
			// Plain local.
			sym.Flags |= SymLocal

		case sym.Flags.Has(SymUsed):
			// Used but not bound here — try parent function scopes;
			// if found, this becomes a free; otherwise implicit
			// global.
			if _, owner := s.Resolve(name); owner != nil {
				sym.Flags |= SymFree
				promoteToCell(owner, name)
				s.Frees = append(s.Frees, name)
			} else {
				sym.Flags |= SymGlobalImplicit
			}
		}
	}

	// Cells in this scope: any symbol with SymCell that is not
	// already in Frees, in OrderedNames order.
	for _, name := range s.OrderedNames {
		sym := s.Symbols[name]
		if sym.Flags.Has(SymCell) && !contains(s.Frees, name) {
			if !contains(s.Cells, name) {
				s.Cells = append(s.Cells, name)
			}
		}
	}
	return nil
}

// promoteToCell flips a binding in scope to a cell. Called when a
// nested scope captures the name as free.
func promoteToCell(s *Scope, name string) {
	if sym, ok := s.Symbols[name]; ok {
		sym.Flags |= SymCell
	}
}

func contains(xs []string, x string) bool {
	return slices.Contains(xs, x)
}

// topName returns the leading dotted component of a module path
// like "a.b.c" -> "a".
func topName(path string) string {
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			return path[:i]
		}
	}
	return path
}
