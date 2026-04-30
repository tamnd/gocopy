// Package future collects __future__ feature flags from a module
// AST. It mirrors CPython 3.14's Python/future.c: a small pre-pass
// that walks the module's leading statements, finds
// `from __future__ import NAME` directives, and ORs the matching
// flags into a Flags bitset.
//
// Only features that change codegen are tracked here. CPython's
// FUTURE_* table contains historical names (`with_statement`,
// `nested_scopes`, etc.) that are no-ops on modern interpreters; we
// recognize them for parity but produce no observable effect. The
// only flag the codegen actually consumes today is Annotations
// (PEP 563 deferred evaluation), and even that is dormant until
// v0.6.6 routes function annotations through the new pipeline.
//
// CPython rules, mirrored:
//
//   - A `from __future__ import …` statement may appear only after
//     an optional module docstring and any other future imports.
//     Once a non-docstring, non-import statement has been seen, a
//     subsequent future import is a SyntaxError.
//   - `from __future__ import *` is rejected.
//   - `import __future__` (plain import) is allowed and is *not*
//     a future-flag directive.
//   - Unknown feature names under __future__ raise an error.
//
// SOURCE: CPython 3.14 Python/future.c, Lib/__future__.py.
package future

import (
	"fmt"

	"github.com/tamnd/gocopy/v1/ast"
)

// Flags is the OR of recognized __future__ features in a module.
type Flags uint32

const (
	// Annotations toggles PEP 563 / PEP 649 deferred annotation
	// evaluation. Codegen reads this when emitting __annotations__
	// blocks (deferred to v0.6.6).
	Annotations Flags = 1 << iota
	// Division corresponds to `from __future__ import division`.
	// Historical no-op on Python 3.x.
	Division
	// AbsoluteImport corresponds to `from __future__ import
	// absolute_import`. Historical no-op on Python 3.x.
	AbsoluteImport
	// WithStatement corresponds to `from __future__ import
	// with_statement`. Historical no-op on Python 3.x.
	WithStatement
	// PrintFunction corresponds to `from __future__ import
	// print_function`. Historical no-op on Python 3.x.
	PrintFunction
	// NestedScopes corresponds to `from __future__ import
	// nested_scopes`. Historical no-op on Python 3.x.
	NestedScopes
	// UnicodeLiterals corresponds to `from __future__ import
	// unicode_literals`. Historical no-op on Python 3.x.
	UnicodeLiterals
	// GeneratorStop corresponds to `from __future__ import
	// generator_stop`. Always-on in Python 3.7+.
	GeneratorStop
	// BarryAsBdfl corresponds to `from __future__ import
	// barry_as_FLUFL`. Recognized for parity only.
	BarryAsBdfl
)

// featureNames maps the recognized __future__ feature names to
// their flag bit. Names not in this map produce an Error.
var featureNames = map[string]Flags{
	"annotations":      Annotations,
	"division":         Division,
	"absolute_import":  AbsoluteImport,
	"with_statement":   WithStatement,
	"print_function":   PrintFunction,
	"nested_scopes":    NestedScopes,
	"unicode_literals": UnicodeLiterals,
	"generator_stop":   GeneratorStop,
	"barry_as_FLUFL":   BarryAsBdfl,
}

// Error is the failure type returned by Collect for misplaced or
// invalid __future__ imports. The compiler driver surfaces these
// with the same formatting as parser errors.
type Error struct {
	Pos ast.Pos
	Msg string
}

func (e *Error) Error() string {
	return fmt.Sprintf("future: %s at line %d col %d", e.Msg, e.Pos.Line, e.Pos.Col)
}

// Collect walks the module's leading statements and returns the OR
// of every recognized __future__ feature.
func Collect(m *ast.Module) (Flags, error) {
	if m == nil {
		return 0, nil
	}
	var flags Flags
	docstringAllowed := true
	futureAllowed := true
	for _, stmt := range m.Body {
		// A leading bare-string ExprStmt is the module docstring;
		// it does not close the __future__ window but only one
		// such statement is allowed.
		if isDocstring(stmt) && docstringAllowed {
			docstringAllowed = false
			continue
		}
		docstringAllowed = false

		imp, ok := stmt.(*ast.ImportFrom)
		if !ok || imp.Module != "__future__" {
			// Any non-future statement closes the window.
			futureAllowed = false
			continue
		}

		if !futureAllowed {
			return 0, &Error{
				Pos: imp.P,
				Msg: "from __future__ imports must occur at the beginning of the file",
			}
		}
		bit, err := collectImport(imp)
		if err != nil {
			return 0, err
		}
		flags |= bit
	}
	return flags, nil
}

// collectImport processes a single `from __future__ import …` and
// returns the OR of the named feature bits.
func collectImport(imp *ast.ImportFrom) (Flags, error) {
	if len(imp.Names) == 0 {
		return 0, &Error{Pos: imp.P, Msg: "empty __future__ import"}
	}
	var bits Flags
	for _, alias := range imp.Names {
		if alias.Name == "*" {
			return 0, &Error{
				Pos: imp.P,
				Msg: "future feature * is not defined",
			}
		}
		bit, ok := featureNames[alias.Name]
		if !ok {
			return 0, &Error{
				Pos: imp.P,
				Msg: fmt.Sprintf("future feature %s is not defined", alias.Name),
			}
		}
		bits |= bit
	}
	return bits, nil
}

// isDocstring reports whether stmt is a bare string-Constant
// ExprStmt suitable as a module docstring.
func isDocstring(stmt ast.Stmt) bool {
	es, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	c, ok := es.Value.(*ast.Constant)
	if !ok {
		return false
	}
	_, isStr := c.Value.(string)
	return isStr
}
