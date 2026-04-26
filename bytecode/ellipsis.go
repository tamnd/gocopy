package bytecode

// Ellipsis is the gocopy sentinel for Python's `Ellipsis` singleton
// (the value of the `...` literal). Go has no built-in equivalent and
// `nil` is already taken by `None`, so we use a private named type with
// a single exported value. Equality (`v == bytecode.Ellipsis`) works
// because `EllipsisType` is comparable.
//
// The marshal layer emits this sentinel as the single byte
// `TYPE_ELLIPSIS` (0x2e) with no FLAG_REF (CPython treats Ellipsis as
// a built-in singleton).
type EllipsisType struct{}

// Ellipsis is the singleton EllipsisType value used in CodeObject.Consts
// to represent Python's `...` literal.
var Ellipsis = EllipsisType{}
