// Package marshal serializes a CodeObject to the CPython marshal stream.
// The wire format mirrors Python/marshal.c byte-for-byte and is the inverse
// of github.com/tamnd/goipy/marshal/reader.go.
package marshal

// Type tags. Match CPython's enum in Python/marshal.c. Source-of-truth for
// reading is github.com/tamnd/goipy/marshal/reader.go; this file is the
// writing side.
const (
	TYPE_NULL                 = '0'
	TYPE_NONE                 = 'N'
	TYPE_FALSE                = 'F'
	TYPE_TRUE                 = 'T'
	TYPE_STOPITER             = 'S'
	TYPE_ELLIPSIS             = '.'
	TYPE_INT                  = 'i'
	TYPE_INT64                = 'I'
	TYPE_FLOAT                = 'f'
	TYPE_BINARY_FLOAT         = 'g'
	TYPE_COMPLEX              = 'x'
	TYPE_BINARY_COMPLEX       = 'y'
	TYPE_LONG                 = 'l'
	TYPE_STRING               = 's'
	TYPE_INTERNED             = 't'
	TYPE_REF                  = 'r'
	TYPE_TUPLE                = '('
	TYPE_LIST                 = '['
	TYPE_DICT                 = '{'
	TYPE_CODE                 = 'c'
	TYPE_UNICODE              = 'u'
	TYPE_UNKNOWN              = '?'
	TYPE_SET                  = '<'
	TYPE_FROZENSET            = '>'
	TYPE_ASCII                = 'a'
	TYPE_ASCII_INTERNED       = 'A'
	TYPE_SMALL_TUPLE          = ')'
	TYPE_SHORT_ASCII          = 'z'
	TYPE_SHORT_ASCII_INTERNED = 'Z'
	TYPE_SLICE                = ':'

	// FlagRef bit ORed into the type byte to tell the reader to record this
	// object in the reference table for later TYPE_REF back-references.
	FlagRef = 0x80
)
