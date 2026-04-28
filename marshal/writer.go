package marshal

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/tamnd/gocopy/v1/bytecode"
)

// Marshal encodes c as a CPython 3.14 marshal stream (no .pyc header).
//
// CPython's Python/marshal.c::w_object stamps FLAG_REF on the type byte
// whenever Py_REFCNT(v) > 1, then dedups subsequent occurrences via
// TYPE_REF. We don't have CPython refcounts; instead we encode the
// empirical rules that produce byte-identical output:
//
//   - Code objects: always FLAG_REF.
//   - The Bytecode bytestring field of a Code: always FLAG_REF (mirrors
//     CPython's bytecode bytes object reaching refcount > 1 between
//     compile and marshal).
//   - Empty bytestrings: always FLAG_REF (CPython singleton).
//   - Other bytestrings (linetable, exctable): FLAG_REF iff occurs more
//     than once across the whole walk (lets exctable dedup with an empty
//     localspluskinds, etc.).
//   - Empty tuples: always FLAG_REF (CPython singleton).
//   - Non-empty tuples (consts, names): FLAG_REF iff occurs > 1.
//   - Interned strings (TYPE_SHORT_ASCII_INTERNED): always FLAG_REF
//     (CPython interned-pool guarantees refcount > 1).
//   - Non-interned strings (TYPE_SHORT_ASCII): FLAG_REF iff occurs > 1.
//
// The walk order matches Python/marshal.c's code-object branch field by
// field. The ref index assigned to each FLAG_REF object is the order in
// which reservations happen (0, 1, 2, ...).
func Marshal(c *bytecode.CodeObject) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("marshal: nil CodeObject")
	}
	rc := newRefCounter()
	rc.code(c, true)

	w := &writer{counts: rc.counts}
	w.code(c, true)
	if w.err != nil {
		return nil, w.err
	}
	return w.buf, nil
}

// writer is the second-pass emitter.
type writer struct {
	buf    []byte
	counts map[any]int
	refs   map[any]uint32
	nextID uint32
	err    error
}

func (w *writer) reserveKey(key any) byte {
	if w.refs == nil {
		w.refs = make(map[any]uint32)
	}
	w.refs[key] = w.nextID
	w.nextID++
	return FlagRef
}

func (w *writer) emitRef(key any) bool {
	if w.refs == nil {
		return false
	}
	if id, ok := w.refs[key]; ok {
		w.buf = append(w.buf, TYPE_REF)
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], id)
		w.buf = append(w.buf, b[:]...)
		return true
	}
	return false
}

func (w *writer) writeI32(v int32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	w.buf = append(w.buf, b[:]...)
}

func (w *writer) writeU32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.buf = append(w.buf, b[:]...)
}

// code emits a code object. topLevel=true for the .pyc root (FLAG_REF);
// topLevel=false for nested code objects in co_consts (no FLAG_REF).
func (w *writer) code(c *bytecode.CodeObject, topLevel bool) {
	if topLevel {
		w.reserveKey(c)
		w.buf = append(w.buf, TYPE_CODE|FlagRef)
	} else {
		w.buf = append(w.buf, TYPE_CODE)
	}

	w.writeI32(c.ArgCount)
	w.writeI32(c.PosOnlyArgCount)
	w.writeI32(c.KwOnlyArgCount)
	w.writeI32(c.StackSize)
	w.writeI32(int32(c.Flags))

	w.bytecodeField(c.Bytecode)
	w.tuple(c.Consts)
	w.stringTuple(c.Names)
	w.stringTuple(c.LocalsPlusNames)
	w.bytestring(c.LocalsPlusKinds)
	w.shortAscii(c.Filename, true) // co_filename: always interned
	w.shortAscii(c.Name, true)     // co_name: always interned
	w.shortAscii(c.QualName, true) // co_qualname: always interned
	w.writeI32(c.FirstLineNo)
	w.bytestring(c.LineTable)
	w.bytestring(c.ExcTable)
}

// bytecodeField writes the Code.Bytecode payload. Always FLAG_REF (the
// bytecode bytes object holds refcount > 1 in CPython by the time marshal
// runs; observed empirically on python3.14 -m py_compile output).
func (w *writer) bytecodeField(b []byte) {
	key := bsKey(b)
	// Distinct ref slot from any other bytestring with the same content,
	// so we don't accidentally dedup the bytecode with linetable/etc.
	w.reserveKey(struct {
		tag string
		k   any
	}{"bytecode", key})
	w.buf = append(w.buf, TYPE_STRING|FlagRef)
	w.writeU32(uint32(len(b)))
	w.buf = append(w.buf, b...)
}

// bytestring emits a TYPE_STRING for non-bytecode payloads (linetable,
// exctable, localspluskinds). Empty bytestrings are CPython singletons and
// always get FLAG_REF; non-empty bytestrings get FLAG_REF only if the walk
// sees them more than once.
func (w *writer) bytestring(b []byte) {
	key := bsKey(b)
	if w.emitRef(key) {
		return
	}
	flag := byte(0)
	if len(b) == 0 || w.counts[key] > 1 {
		flag = w.reserveKey(key)
	}
	w.buf = append(w.buf, TYPE_STRING|flag)
	w.writeU32(uint32(len(b)))
	w.buf = append(w.buf, b...)
}

// shortAscii emits a TYPE_SHORT_ASCII or TYPE_SHORT_ASCII_INTERNED. Interned
// strings always FLAG_REF; non-interned only if seen more than once.
func (w *writer) shortAscii(s string, interned bool) {
	if len(s) > 255 {
		w.err = fmt.Errorf("marshal: shortAscii overflow (len %d)", len(s))
		return
	}
	tag := byte(TYPE_SHORT_ASCII)
	if interned {
		tag = TYPE_SHORT_ASCII_INTERNED
	}
	key := saKey{s: s, interned: interned}
	if w.emitRef(key) {
		return
	}
	flag := byte(0)
	if interned || w.counts[key] > 1 {
		flag = w.reserveKey(key)
	}
	w.buf = append(w.buf, tag|flag)
	w.buf = append(w.buf, byte(len(s)))
	w.buf = append(w.buf, s...)
}

// tuple emits the consts tuple. Empty tuples always FLAG_REF; non-empty
// tuples only if seen more than once.
func (w *writer) tuple(items []any) {
	key := tupleKey(items)
	if w.emitRef(key) {
		return
	}
	flag := byte(0)
	if len(items) == 0 || w.counts[key] > 1 {
		flag = w.reserveKey(key)
	}
	if len(items) > 255 {
		w.buf = append(w.buf, TYPE_TUPLE|flag)
		w.writeU32(uint32(len(items)))
	} else {
		w.buf = append(w.buf, TYPE_SMALL_TUPLE|flag)
		w.buf = append(w.buf, byte(len(items)))
	}
	for _, e := range items {
		w.emitObject(e)
	}
}

// stringTuple is a tuple whose elements are interned strings (names,
// localsplusnames). Same FLAG_REF rule as tuple.
func (w *writer) stringTuple(items []string) {
	key := strTupleKey(items)
	if w.emitRef(key) {
		return
	}
	flag := byte(0)
	if len(items) == 0 || w.counts[key] > 1 {
		flag = w.reserveKey(key)
	}
	if len(items) > 255 {
		w.buf = append(w.buf, TYPE_TUPLE|flag)
		w.writeU32(uint32(len(items)))
	} else {
		w.buf = append(w.buf, TYPE_SMALL_TUPLE|flag)
		w.buf = append(w.buf, byte(len(items)))
	}
	for _, s := range items {
		w.shortAscii(s, true)
	}
}

// longAscii emits a TYPE_ASCII string for strings longer than 255 bytes.
// FLAG_REF only if seen more than once.
func (w *writer) longAscii(s string) {
	for i := range len(s) {
		if s[i] > 0x7f {
			w.err = fmt.Errorf("marshal: string const not ASCII (len %d)", len(s))
			return
		}
	}
	key := saKey{s: s, interned: false}
	if w.emitRef(key) {
		return
	}
	flag := byte(0)
	if w.counts[key] > 1 {
		flag = w.reserveKey(key)
	}
	w.buf = append(w.buf, TYPE_ASCII|flag)
	w.writeU32(uint32(len(s)))
	w.buf = append(w.buf, s...)
}

// emitObject dispatches on the dynamic type of a constant value.
// String consts are emitted as TYPE_SHORT_ASCII_INTERNED when every
// byte is a name char (alphanumeric or underscore) — the rule CPython's
// `all_name_chars` applies in `intern_string_constants`. Anything else
// (spaces, newlines, punctuation) is plain TYPE_SHORT_ASCII. Strings
// longer than 255 bytes use TYPE_ASCII. Bytes consts route through the
// same TYPE_STRING bytestring path used by linetable/exctable.
// Ellipsis emits as a single TYPE_ELLIPSIS byte with no FLAG_REF.
func (w *writer) emitObject(v any) {
	switch x := v.(type) {
	case nil:
		w.buf = append(w.buf, TYPE_NONE)
	case bool:
		if x {
			w.buf = append(w.buf, TYPE_TRUE)
		} else {
			w.buf = append(w.buf, TYPE_FALSE)
		}
	case string:
		if len(x) > 255 {
			w.longAscii(x)
			return
		}
		if !isShortAscii(x) {
			w.err = fmt.Errorf("marshal: string const not short-ASCII (len %d)", len(x))
			return
		}
		w.shortAscii(x, isInterned(x))
	case []byte:
		w.bytestring(x)
	case bytecode.EllipsisType:
		w.buf = append(w.buf, TYPE_ELLIPSIS)
	case int64:
		if x < -1<<31 || x > 1<<31-1 {
			w.err = fmt.Errorf("marshal: int64 const %d out of TYPE_INT range", x)
			return
		}
		key := intKey(x)
		if w.emitRef(key) {
			return
		}
		// CPython 3.14: immortal small ints [-5, 256] always get FLAG_REF
		// (refcount is permanently > 1). Larger ints only if seen > 1x.
		immortal := x >= -5 && x <= 256
		if immortal || w.counts[key] > 1 {
			w.reserveKey(key)
			w.buf = append(w.buf, TYPE_INT|FlagRef)
		} else {
			w.buf = append(w.buf, TYPE_INT)
		}
		w.writeI32(int32(x))
	case float64:
		key := float64Key(x)
		if w.emitRef(key) {
			return
		}
		// Floats are not immortal; FLAG_REF only if seen more than once.
		if w.counts[key] > 1 {
			w.reserveKey(key)
			w.buf = append(w.buf, TYPE_BINARY_FLOAT|FlagRef)
		} else {
			w.buf = append(w.buf, TYPE_BINARY_FLOAT)
		}
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(x))
		w.buf = append(w.buf, b[:]...)
	case complex128:
		key := complexKey(x)
		if w.emitRef(key) {
			return
		}
		// Complex is not immortal; FLAG_REF only if seen more than once.
		if w.counts[key] > 1 {
			w.reserveKey(key)
			w.buf = append(w.buf, TYPE_BINARY_COMPLEX|FlagRef)
		} else {
			w.buf = append(w.buf, TYPE_BINARY_COMPLEX)
		}
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(real(x)))
		w.buf = append(w.buf, b[:]...)
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(imag(x)))
		w.buf = append(w.buf, b[:]...)
	case bytecode.ConstTuple:
		w.tuple([]any(x))
	case *bytecode.CodeObject:
		w.code(x, false)
	default:
		w.err = fmt.Errorf("marshal: unsupported const type %T", v)
	}
}

// isShortAscii reports whether s fits the TYPE_SHORT_ASCII_INTERNED
// constraints: pure ASCII bytes and length up to 255.
func isShortAscii(s string) bool {
	if len(s) > 255 {
		return false
	}
	for i := range len(s) {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}

// isAllNameChars mirrors CPython's all_name_chars: every byte must be ASCII
// alphanumeric or underscore. The empty string returns false.
func isAllNameChars(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_':
		default:
			return false
		}
	}
	return true
}

// isInterned reports whether CPython would emit this string as
// TYPE_SHORT_ASCII_INTERNED. Strings with all-name chars are interned by
// intern_string_constants in compile.c. A handful of other strings are
// pre-interned by the CPython runtime at startup (e.g. "*" for wildcard
// imports) and therefore also appear as interned at marshal time.
func isInterned(s string) bool {
	if isAllNameChars(s) {
		return true
	}
	// CPython pre-interns "*" via importlib bootstrap.
	return s == "*"
}

// --- ref-key helpers ----------------------------------------------------

type bsKeyType struct{ s string }
type saKey struct {
	s        string
	interned bool
}
type tupleKeyType struct{ s string }
type strTupleKeyType struct{ s string }
type intKeyType struct{ v int64 }
type float64KeyType struct{ v uint64 }
type complexKeyType struct{ r, i uint64 }

// emptyTupleKey is the ref-map key for the empty-tuple singleton ().
// CPython interns () across co_consts, co_names, co_localsplusnames, etc.
// All three callers (tuple / stringTuple / ConstTuple) must share this key.
type emptyTupleKey struct{}

func bsKey(b []byte) any       { return bsKeyType{s: string(b)} }
func intKey(v int64) any       { return intKeyType{v: v} }
func float64Key(v float64) any { return float64KeyType{v: math.Float64bits(v)} }
func complexKey(v complex128) any {
	return complexKeyType{r: math.Float64bits(real(v)), i: math.Float64bits(imag(v))}
}

func tupleKey(items []any) any {
	if len(items) == 0 {
		return emptyTupleKey{}
	}
	buf := make([]byte, 0, len(items)*2)
	for _, e := range items {
		switch x := e.(type) {
		case nil:
			buf = append(buf, 'N')
		case bool:
			buf = append(buf, 'b')
		case string:
			buf = append(buf, 's', 0)
			buf = append(buf, x...)
			buf = append(buf, 0)
		case []byte:
			buf = append(buf, 'B', 0)
			buf = append(buf, x...)
			buf = append(buf, 0)
		case bytecode.EllipsisType:
			buf = append(buf, 'E')
		case int64:
			buf = append(buf, 'I',
				byte(x), byte(x>>8), byte(x>>16), byte(x>>24))
		case float64:
			bits := math.Float64bits(x)
			buf = append(buf, 'G',
				byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24),
				byte(bits>>32), byte(bits>>40), byte(bits>>48), byte(bits>>56))
		case complex128:
			r := math.Float64bits(real(x))
			i := math.Float64bits(imag(x))
			buf = append(buf, 'Y',
				byte(r), byte(r>>8), byte(r>>16), byte(r>>24),
				byte(r>>32), byte(r>>40), byte(r>>48), byte(r>>56),
				byte(i), byte(i>>8), byte(i>>16), byte(i>>24),
				byte(i>>32), byte(i>>40), byte(i>>48), byte(i>>56))
		default:
			buf = append(buf, '?')
		}
	}
	return tupleKeyType{s: string(buf)}
}

func strTupleKey(items []string) any {
	if len(items) == 0 {
		return emptyTupleKey{}
	}
	total := 0
	for _, s := range items {
		total += len(s) + 1
	}
	buf := make([]byte, 0, total)
	for _, s := range items {
		buf = append(buf, s...)
		buf = append(buf, 0)
	}
	return strTupleKeyType{s: string(buf)}
}

// --- pass one: occurrence counter --------------------------------------

type refCounter struct {
	counts map[any]int
}

func newRefCounter() *refCounter {
	return &refCounter{counts: make(map[any]int)}
}

func (rc *refCounter) bump(k any) { rc.counts[k]++ }

func (rc *refCounter) code(c *bytecode.CodeObject, topLevel bool) {
	_ = topLevel
	rc.bytecode(c.Bytecode)
	rc.tuple(c.Consts)
	rc.stringTuple(c.Names)
	rc.stringTuple(c.LocalsPlusNames)
	rc.bytestring(c.LocalsPlusKinds)
	rc.shortAscii(c.Filename, true)
	rc.shortAscii(c.Name, true)
	rc.shortAscii(c.QualName, true)
	rc.bytestring(c.LineTable)
	rc.bytestring(c.ExcTable)
}

// bytecode does NOT contribute to bytestring counts because it lives in
// its own ref slot (see bytecodeField). Skipping it here prevents an
// accidental dedup between the bytecode payload and any other bytestring
// with the same content.
func (rc *refCounter) bytecode(b []byte) {}

func (rc *refCounter) bytestring(b []byte) {
	rc.bump(bsKey(b))
}

func (rc *refCounter) shortAscii(s string, interned bool) {
	rc.bump(saKey{s: s, interned: interned})
}

func (rc *refCounter) tuple(items []any) {
	rc.bump(tupleKey(items))
	for _, e := range items {
		switch x := e.(type) {
		case *bytecode.CodeObject:
			rc.code(x, false)
		case string:
			if len(x) > 255 {
				rc.bump(saKey{s: x, interned: false})
			} else {
				rc.shortAscii(x, isInterned(x))
			}
		case []byte:
			rc.bytestring(x)
		case int64:
			rc.bump(intKey(x))
		case float64:
			rc.bump(float64Key(x))
		case complex128:
			rc.bump(complexKey(x))
		case bytecode.ConstTuple:
			rc.tuple([]any(x))
		}
	}
}

func (rc *refCounter) stringTuple(items []string) {
	rc.bump(strTupleKey(items))
	for _, s := range items {
		rc.shortAscii(s, true)
	}
}
