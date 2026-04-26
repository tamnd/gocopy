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
)
