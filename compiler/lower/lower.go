// Package lower is the single translation point between the gopapy
// parser and the gocopy compiler. Everything under compiler/* speaks
// ast.* types; the parser produces parser.* values. Lower bridges
// the two.
//
// At v0.6.2 the bridge is the identity function: ast types are Go
// type aliases for parser types, so a *parser.Module already
// satisfies *ast.Module. The signature here is the contract that
// future releases honor — when codegen needs derived-data slots
// (Symbol, ConstRef) attached to AST nodes (v0.6.7+), the alias
// becomes a concrete gocopy type and Lower grows a real recursive
// walk. Call sites do not change.
//
// The error slot is reserved for shape rejections that we want to
// surface earlier than codegen would. None are exercised at v0.6.2.
package lower

import (
	"github.com/tamnd/gocopy/compiler/ast"
	parser "github.com/tamnd/gopapy/parser"
)

// Lower translates a parser-produced module into the AST that the
// compiler consumes.
func Lower(m *parser.Module) (*ast.Module, error) {
	return m, nil
}
