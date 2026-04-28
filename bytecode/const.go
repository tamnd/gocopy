package bytecode

import (
	"math"
	"math/cmplx"
	"strings"
)

// ConstKind tags one variant of Const. The discrete set mirrors the
// Python types CPython's marshal format encodes for code-object
// constants: None, True, False, Ellipsis, int, float, complex, str,
// bytes, tuple, code.
type ConstKind uint8

const (
	KindNone ConstKind = iota
	KindFalse
	KindTrue
	KindEllipsis
	KindInt
	KindFloat
	KindComplex
	KindStr
	KindBytes
	KindTuple
	KindCode
)

// Const is a tagged union covering every kind of value gocopy emits
// into co_consts. The struct stays small: tagged-int / tagged-float /
// pointer-into-pool layouts match what the marshal writer needs.
type Const struct {
	Kind     ConstKind
	Int      int64
	Float    float64
	Complex  complex128
	Str      string
	Bytes    []byte
	Tuple    []ConstRef
	Code     *CodeObject
}

// ConstRef indexes into a ConstPool.
type ConstRef uint32

// ConstPool stores Const values with type-specific interning
// matching CPython's marshal-time identity rules:
//
//   - None / True / False / Ellipsis are singletons.
//   - Strings intern by content.
//   - Ints intern by value.
//   - Floats intern by bit pattern (so -0.0 and 0.0 stay distinct,
//     and NaN bit patterns preserve identity).
//   - Complex numbers intern by the bit pattern of (real, imag).
//   - Bytes intern by content.
//   - Tuples intern by the sequence of ConstRefs they contain.
//   - Code objects are always distinct: each compiled function gets
//     its own entry.
type ConstPool struct {
	consts []Const

	noneRef     ConstRef
	trueRef     ConstRef
	falseRef    ConstRef
	ellipsisRef ConstRef
	hasNone     bool
	hasTrue     bool
	hasFalse    bool
	hasEllipsis bool

	intRefs     map[int64]ConstRef
	floatRefs   map[uint64]ConstRef
	complexRefs map[[2]uint64]ConstRef
	strRefs     map[string]ConstRef
	bytesRefs   map[string]ConstRef
	tupleRefs   map[string]ConstRef
}

// NewConstPool returns an empty pool.
func NewConstPool() *ConstPool {
	return &ConstPool{
		intRefs:     make(map[int64]ConstRef),
		floatRefs:   make(map[uint64]ConstRef),
		complexRefs: make(map[[2]uint64]ConstRef),
		strRefs:     make(map[string]ConstRef),
		bytesRefs:   make(map[string]ConstRef),
		tupleRefs:   make(map[string]ConstRef),
	}
}

func (p *ConstPool) push(c Const) ConstRef {
	r := ConstRef(len(p.consts))
	p.consts = append(p.consts, c)
	return r
}

// AddNone returns the ConstRef for None, allocating it on first use.
func (p *ConstPool) AddNone() ConstRef {
	if p.hasNone {
		return p.noneRef
	}
	p.noneRef = p.push(Const{Kind: KindNone})
	p.hasNone = true
	return p.noneRef
}

// AddBool returns the ConstRef for True or False.
func (p *ConstPool) AddBool(b bool) ConstRef {
	if b {
		if p.hasTrue {
			return p.trueRef
		}
		p.trueRef = p.push(Const{Kind: KindTrue})
		p.hasTrue = true
		return p.trueRef
	}
	if p.hasFalse {
		return p.falseRef
	}
	p.falseRef = p.push(Const{Kind: KindFalse})
	p.hasFalse = true
	return p.falseRef
}

// AddEllipsis returns the ConstRef for the Ellipsis singleton.
func (p *ConstPool) AddEllipsis() ConstRef {
	if p.hasEllipsis {
		return p.ellipsisRef
	}
	p.ellipsisRef = p.push(Const{Kind: KindEllipsis})
	p.hasEllipsis = true
	return p.ellipsisRef
}

// AddInt interns an int constant.
func (p *ConstPool) AddInt(v int64) ConstRef {
	if r, ok := p.intRefs[v]; ok {
		return r
	}
	r := p.push(Const{Kind: KindInt, Int: v})
	p.intRefs[v] = r
	return r
}

// AddFloat interns a float constant by bit pattern.
func (p *ConstPool) AddFloat(v float64) ConstRef {
	bits := math.Float64bits(v)
	if r, ok := p.floatRefs[bits]; ok {
		return r
	}
	r := p.push(Const{Kind: KindFloat, Float: v})
	p.floatRefs[bits] = r
	return r
}

// AddComplex interns a complex constant by the bit pattern of its
// real and imaginary parts.
func (p *ConstPool) AddComplex(v complex128) ConstRef {
	key := [2]uint64{
		math.Float64bits(real(v)),
		math.Float64bits(imag(v)),
	}
	if r, ok := p.complexRefs[key]; ok {
		return r
	}
	// Defensive: if real or imag is NaN, keep in the map keyed by
	// bits, but cmplx.IsNaN is the closest sanity hook we have.
	_ = cmplx.IsNaN(v)
	r := p.push(Const{Kind: KindComplex, Complex: v})
	p.complexRefs[key] = r
	return r
}

// AddStr interns a string constant by content.
func (p *ConstPool) AddStr(s string) ConstRef {
	if r, ok := p.strRefs[s]; ok {
		return r
	}
	r := p.push(Const{Kind: KindStr, Str: s})
	p.strRefs[s] = r
	return r
}

// AddBytes interns a bytes constant by content.
func (p *ConstPool) AddBytes(b []byte) ConstRef {
	key := string(b)
	if r, ok := p.bytesRefs[key]; ok {
		return r
	}
	dup := append([]byte(nil), b...)
	r := p.push(Const{Kind: KindBytes, Bytes: dup})
	p.bytesRefs[key] = r
	return r
}

// AddTuple interns a tuple constant by the sequence of ConstRefs.
func (p *ConstPool) AddTuple(elems []ConstRef) ConstRef {
	key := tupleKey(elems)
	if r, ok := p.tupleRefs[key]; ok {
		return r
	}
	dup := append([]ConstRef(nil), elems...)
	r := p.push(Const{Kind: KindTuple, Tuple: dup})
	p.tupleRefs[key] = r
	return r
}

// AddCode adds a code-object constant. Code objects are never
// interned; each compiled function gets its own entry.
func (p *ConstPool) AddCode(c *CodeObject) ConstRef {
	return p.push(Const{Kind: KindCode, Code: c})
}

// Get returns the Const at r.
func (p *ConstPool) Get(r ConstRef) Const {
	return p.consts[r]
}

// Slice returns a defensive copy of the pool in insertion order.
func (p *ConstPool) Slice() []Const {
	out := make([]Const, len(p.consts))
	copy(out, p.consts)
	return out
}

// Len returns the number of entries in the pool.
func (p *ConstPool) Len() int {
	return len(p.consts)
}

func tupleKey(refs []ConstRef) string {
	var b strings.Builder
	b.Grow(len(refs) * 5)
	var buf [4]byte
	for _, r := range refs {
		buf[0] = byte(r)
		buf[1] = byte(r >> 8)
		buf[2] = byte(r >> 16)
		buf[3] = byte(r >> 24)
		b.Write(buf[:])
	}
	return b.String()
}
