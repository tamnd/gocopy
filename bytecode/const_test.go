package bytecode

import (
	"math"
	"testing"
)

func TestConstNoneSingleton(t *testing.T) {
	p := NewConstPool()
	a := p.AddNone()
	b := p.AddNone()
	if a != b {
		t.Errorf("None should intern as singleton")
	}
	if p.Len() != 1 {
		t.Errorf("len=%d want 1", p.Len())
	}
	if p.Get(a).Kind != KindNone {
		t.Errorf("kind want KindNone")
	}
}

func TestConstBoolSingletons(t *testing.T) {
	p := NewConstPool()
	tt := p.AddBool(true)
	tt2 := p.AddBool(true)
	ff := p.AddBool(false)
	ff2 := p.AddBool(false)
	if tt != tt2 || ff != ff2 {
		t.Errorf("bools should intern as singletons")
	}
	if tt == ff {
		t.Errorf("True and False must be distinct")
	}
	if p.Get(tt).Kind != KindTrue || p.Get(ff).Kind != KindFalse {
		t.Errorf("kind mismatch")
	}
}

func TestConstEllipsisSingleton(t *testing.T) {
	p := NewConstPool()
	a := p.AddEllipsis()
	b := p.AddEllipsis()
	if a != b {
		t.Errorf("Ellipsis should intern as singleton")
	}
	if p.Get(a).Kind != KindEllipsis {
		t.Errorf("kind want KindEllipsis")
	}
}

func TestConstIntInterning(t *testing.T) {
	p := NewConstPool()
	a := p.AddInt(42)
	b := p.AddInt(42)
	c := p.AddInt(43)
	if a != b {
		t.Errorf("equal ints should intern")
	}
	if a == c {
		t.Errorf("distinct ints should not collide")
	}
	if got := p.Get(a).Int; got != 42 {
		t.Errorf("Int=%d want 42", got)
	}
}

func TestConstFloatBitInterning(t *testing.T) {
	p := NewConstPool()
	a := p.AddFloat(1.5)
	b := p.AddFloat(1.5)
	if a != b {
		t.Errorf("equal floats should intern")
	}
	pos := p.AddFloat(0.0)
	neg := p.AddFloat(math.Copysign(0, -1))
	if pos == neg {
		t.Errorf("0.0 and -0.0 must be distinct (different bit patterns)")
	}
	nan1 := p.AddFloat(math.NaN())
	nan2 := p.AddFloat(math.NaN())
	if nan1 != nan2 {
		t.Errorf("identical NaN bit patterns should intern")
	}
}

func TestConstComplexInterning(t *testing.T) {
	p := NewConstPool()
	a := p.AddComplex(complex(1, 2))
	b := p.AddComplex(complex(1, 2))
	c := p.AddComplex(complex(2, 1))
	if a != b {
		t.Errorf("equal complex should intern")
	}
	if a == c {
		t.Errorf("distinct complex should not collide")
	}
}

func TestConstStrInterning(t *testing.T) {
	p := NewConstPool()
	a := p.AddStr("hello")
	b := p.AddStr("hello")
	c := p.AddStr("world")
	if a != b {
		t.Errorf("equal strings should intern")
	}
	if a == c {
		t.Errorf("distinct strings should not collide")
	}
	if p.Get(a).Str != "hello" {
		t.Errorf("Str mismatch")
	}
}

func TestConstBytesInterning(t *testing.T) {
	p := NewConstPool()
	a := p.AddBytes([]byte("abc"))
	b := p.AddBytes([]byte("abc"))
	c := p.AddBytes([]byte("abd"))
	if a != b {
		t.Errorf("equal bytes should intern")
	}
	if a == c {
		t.Errorf("distinct bytes should not collide")
	}
	src := []byte("def")
	d := p.AddBytes(src)
	src[0] = 'X'
	if string(p.Get(d).Bytes) != "def" {
		t.Errorf("AddBytes must take a defensive copy of the input slice")
	}
}

func TestConstTupleInterning(t *testing.T) {
	p := NewConstPool()
	one := p.AddInt(1)
	two := p.AddInt(2)
	a := p.AddTuple([]ConstRef{one, two})
	b := p.AddTuple([]ConstRef{one, two})
	c := p.AddTuple([]ConstRef{two, one})
	if a != b {
		t.Errorf("equal tuples should intern")
	}
	if a == c {
		t.Errorf("differently-ordered tuples should not collide")
	}
}

func TestConstCodeNotInterned(t *testing.T) {
	p := NewConstPool()
	c1 := &CodeObject{Name: "f"}
	c2 := &CodeObject{Name: "f"}
	a := p.AddCode(c1)
	b := p.AddCode(c2)
	if a == b {
		t.Errorf("code objects should never intern")
	}
}

func TestConstSliceDefensiveCopy(t *testing.T) {
	p := NewConstPool()
	p.AddInt(1)
	p.AddInt(2)
	got := p.Slice()
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	got[0].Int = 999
	if p.Get(0).Int != 1 {
		t.Errorf("Slice() must return defensive copy")
	}
}
