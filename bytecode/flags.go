package bytecode

// CodeObject co_flags bits. Mirror CPython's Include/cpython/code.h.
//
// SOURCE: https://github.com/python/cpython/blob/3.14/Include/cpython/code.h
const (
	CO_OPTIMIZED          uint32 = 0x0001 // function uses fast locals
	CO_NEWLOCALS          uint32 = 0x0002 // creates a new local namespace at call time
	CO_VARARGS            uint32 = 0x0004 // *args parameter present
	CO_VARKEYWORDS        uint32 = 0x0008 // **kwargs parameter present
	CO_NESTED             uint32 = 0x0010 // closure (refers to free variables)
	CO_GENERATOR          uint32 = 0x0020 // is a generator
	CO_NOFREE             uint32 = 0x0040 // no free or cell vars
	CO_COROUTINE          uint32 = 0x0080 // async def
	CO_ITERABLE_COROUTINE uint32 = 0x0100 // @types.coroutine wrapped
	CO_ASYNC_GENERATOR    uint32 = 0x0200 // async generator

	CO_FUTURE_BARRY_AS_BDFL  uint32 = 0x00400000
	CO_FUTURE_GENERATOR_STOP uint32 = 0x00800000
	CO_FUTURE_ANNOTATIONS    uint32 = 0x01000000
	CO_NO_MONITORING_EVENTS  uint32 = 0x02000000
	CO_HAS_DOCSTRING         uint32 = 0x04000000
)

// co_localspluskinds entry bytes. Mirror CPython's
// Include/internal/pycore_code.h CO_FAST_* macros.
//
// Each entry in co_localspluskinds is a byte describing one slot in
// co_localsplusnames. The bits are OR-combined.
const (
	FastHidden byte = 0x10
	FastLocal  byte = 0x20
	FastCell   byte = 0x40
	FastFree   byte = 0x80

	FastArgPos byte = 0x02
	FastArgKw  byte = 0x04
	FastArgVar byte = 0x08

	// FastArg is the common shorthand for a positional-or-keyword argument.
	FastArg = FastArgPos | FastArgKw // 0x06
)
