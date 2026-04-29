package bytecode

// ArgKind classifies what an opcode's oparg means.
//
// Distinct from CacheSize: a cache size says how many bytes follow
// the opcode for inline-caching. ArgKind says what the one oparg
// byte itself is *for*.
type ArgKind uint8

const (
	ArgNone    ArgKind = iota // oparg unused (still occupies one byte)
	ArgConst                  // index into co_consts
	ArgName                   // index into co_names
	ArgLocal                  // index into co_localsplusnames (fast)
	ArgFree                   // index into freevars region of locals
	ArgJumpRel                // relative jump in instruction words
	ArgJumpAbs                // absolute jump target
	ArgRaw                    // opcode-specific encoding (CALL n, COMPARE_OP cmp, etc.)
)

// OpFlags is a bitset of opcode-level booleans.
type OpFlags uint8

const (
	// OpJump is set on any opcode that can transfer control to a
	// non-fallthrough target (forward, backward, conditional, or
	// absolute).
	OpJump OpFlags = 1 << iota
	// OpTerminator ends a basic block: control does not fall through
	// to the next instruction (RETURN_VALUE, JUMP_FORWARD,
	// JUMP_BACKWARD).
	OpTerminator
	// OpPseudo is set on placeholder opcodes that codegen uses but
	// the assembler resolves before emission.
	OpPseudo
)

// OpMeta describes one opcode's contract.
//
//   - Name matches CPython's opcode.opname[op].
//   - HasArg matches dis._inst_has_arg(op).
//   - CacheSize matches CPython's _PyOpcode_Caches[op].
//   - ArgKind groups opcodes by what their oparg means.
//   - Flags carries jump/terminator/pseudo bits.
//   - StackEff is the net stack delta for fixed-effect opcodes.
//   - StackVar is true when the effect depends on the oparg
//     (CALL n, BUILD_TUPLE n, etc.).
type OpMeta struct {
	Name      string
	HasArg    bool
	CacheSize uint8
	ArgKind   ArgKind
	Flags     OpFlags
	StackEff  int8
	StackVar  bool
}

// OpMetaTable is the canonical lookup. Entries default to the zero
// OpMeta, meaning "unknown opcode" (Name == ""). Future releases of
// gocopy add entries as they emit more of the CPython 3.14 opcode
// space.
//
// SOURCE: cross-checked against python3.14 in
// cpython_parity_test.go for every named opcode in opcode.go.
var OpMetaTable = func() [256]OpMeta {
	var t [256]OpMeta

	set := func(op Opcode, m OpMeta) {
		m.CacheSize = CacheSize[op]
		t[op] = m
	}

	// No-arg opcodes. In CPython 3.14 every opcode has an oparg byte
	// in the wire format, but for these the byte is meaningless.
	set(NOP, OpMeta{Name: "NOP", HasArg: false, ArgKind: ArgNone})
	set(NOT_TAKEN, OpMeta{Name: "NOT_TAKEN", HasArg: false, ArgKind: ArgNone})
	set(POP_TOP, OpMeta{Name: "POP_TOP", HasArg: false, ArgKind: ArgNone, StackEff: -1})
	set(PUSH_NULL, OpMeta{Name: "PUSH_NULL", HasArg: false, ArgKind: ArgNone, StackEff: 1})
	set(END_FOR, OpMeta{Name: "END_FOR", HasArg: false, ArgKind: ArgNone, StackEff: -1})
	set(POP_ITER, OpMeta{Name: "POP_ITER", HasArg: false, ArgKind: ArgNone, StackEff: -1})
	set(GET_ITER, OpMeta{Name: "GET_ITER", HasArg: false, ArgKind: ArgNone, StackEff: 0})
	set(TO_BOOL, OpMeta{Name: "TO_BOOL", HasArg: false, ArgKind: ArgNone, StackEff: 0})
	set(UNARY_INVERT, OpMeta{Name: "UNARY_INVERT", HasArg: false, ArgKind: ArgNone, StackEff: 0})
	set(UNARY_NEGATIVE, OpMeta{Name: "UNARY_NEGATIVE", HasArg: false, ArgKind: ArgNone, StackEff: 0})
	set(UNARY_NOT, OpMeta{Name: "UNARY_NOT", HasArg: false, ArgKind: ArgNone, StackEff: 0})
	set(STORE_SUBSCR, OpMeta{Name: "STORE_SUBSCR", HasArg: false, ArgKind: ArgNone, StackEff: -3})
	// RETURN_VALUE is a terminator. dis.stack_effect reports 0
	// for terminators (control does not fall through), so the
	// table mirrors that for parity even though the runtime pops
	// the return value off the stack before returning.
	set(RETURN_VALUE, OpMeta{Name: "RETURN_VALUE", HasArg: false, ArgKind: ArgNone, Flags: OpTerminator, StackEff: 0})
	set(COPY_FREE_VARS, OpMeta{Name: "COPY_FREE_VARS", HasArg: true, ArgKind: ArgRaw, StackEff: 0})
	set(MAKE_FUNCTION, OpMeta{Name: "MAKE_FUNCTION", HasArg: false, ArgKind: ArgNone, StackEff: 0})

	// Constant / small-int / name loads.
	set(LOAD_CONST, OpMeta{Name: "LOAD_CONST", HasArg: true, ArgKind: ArgConst, StackEff: 1})
	set(LOAD_SMALL_INT, OpMeta{Name: "LOAD_SMALL_INT", HasArg: true, ArgKind: ArgRaw, StackEff: 1})
	set(LOAD_NAME, OpMeta{Name: "LOAD_NAME", HasArg: true, ArgKind: ArgName, StackEff: 1})
	set(STORE_NAME, OpMeta{Name: "STORE_NAME", HasArg: true, ArgKind: ArgName, StackEff: -1})
	set(IMPORT_NAME, OpMeta{Name: "IMPORT_NAME", HasArg: true, ArgKind: ArgName, StackEff: -1})
	set(IMPORT_FROM, OpMeta{Name: "IMPORT_FROM", HasArg: true, ArgKind: ArgName, StackEff: 1})
	set(STORE_ATTR, OpMeta{Name: "STORE_ATTR", HasArg: true, ArgKind: ArgName, StackEff: -2})
	set(LOAD_ATTR, OpMeta{Name: "LOAD_ATTR", HasArg: true, ArgKind: ArgRaw, StackVar: true})
	set(LOAD_GLOBAL, OpMeta{Name: "LOAD_GLOBAL", HasArg: true, ArgKind: ArgRaw, StackVar: true})

	// Locals / cells / frees.
	set(LOAD_FAST, OpMeta{Name: "LOAD_FAST", HasArg: true, ArgKind: ArgLocal, StackEff: 1})
	set(LOAD_FAST_BORROW, OpMeta{Name: "LOAD_FAST_BORROW", HasArg: true, ArgKind: ArgLocal, StackEff: 1})
	set(LOAD_FAST_BORROW_LOAD_FAST_BORROW, OpMeta{Name: "LOAD_FAST_BORROW_LOAD_FAST_BORROW", HasArg: true, ArgKind: ArgRaw, StackEff: 2})
	set(STORE_FAST, OpMeta{Name: "STORE_FAST", HasArg: true, ArgKind: ArgLocal, StackEff: -1})
	set(LOAD_DEREF, OpMeta{Name: "LOAD_DEREF", HasArg: true, ArgKind: ArgFree, StackEff: 1})
	set(STORE_DEREF, OpMeta{Name: "STORE_DEREF", HasArg: true, ArgKind: ArgFree, StackEff: -1})
	set(STORE_GLOBAL, OpMeta{Name: "STORE_GLOBAL", HasArg: true, ArgKind: ArgName, StackEff: -1})
	set(MAKE_CELL, OpMeta{Name: "MAKE_CELL", HasArg: true, ArgKind: ArgLocal, StackEff: 0})

	// Builders. Stack effect = -n + 1 for these, captured as
	// StackVar.
	set(BUILD_LIST, OpMeta{Name: "BUILD_LIST", HasArg: true, ArgKind: ArgRaw, StackVar: true})
	set(BUILD_MAP, OpMeta{Name: "BUILD_MAP", HasArg: true, ArgKind: ArgRaw, StackVar: true})
	set(BUILD_SET, OpMeta{Name: "BUILD_SET", HasArg: true, ArgKind: ArgRaw, StackVar: true})
	set(BUILD_TUPLE, OpMeta{Name: "BUILD_TUPLE", HasArg: true, ArgKind: ArgRaw, StackVar: true})
	set(LIST_APPEND, OpMeta{Name: "LIST_APPEND", HasArg: true, ArgKind: ArgRaw, StackEff: -1})
	set(LIST_EXTEND, OpMeta{Name: "LIST_EXTEND", HasArg: true, ArgKind: ArgRaw, StackEff: -1})

	// Calls and ops with variable effect.
	set(CALL, OpMeta{Name: "CALL", HasArg: true, ArgKind: ArgRaw, StackVar: true})
	set(CALL_INTRINSIC_1, OpMeta{Name: "CALL_INTRINSIC_1", HasArg: true, ArgKind: ArgRaw, StackEff: 0})
	set(SET_FUNCTION_ATTRIBUTE, OpMeta{Name: "SET_FUNCTION_ATTRIBUTE", HasArg: true, ArgKind: ArgRaw, StackEff: -1})

	// Comparisons / membership.
	set(COMPARE_OP, OpMeta{Name: "COMPARE_OP", HasArg: true, ArgKind: ArgRaw, StackEff: -1})
	set(CONTAINS_OP, OpMeta{Name: "CONTAINS_OP", HasArg: true, ArgKind: ArgRaw, StackEff: -1})
	set(IS_OP, OpMeta{Name: "IS_OP", HasArg: true, ArgKind: ArgRaw, StackEff: -1})
	set(BINARY_OP, OpMeta{Name: "BINARY_OP", HasArg: true, ArgKind: ArgRaw, StackEff: -1})

	// Stack manipulation.
	set(COPY, OpMeta{Name: "COPY", HasArg: true, ArgKind: ArgRaw, StackEff: 1})

	// Jumps.
	set(JUMP_FORWARD, OpMeta{Name: "JUMP_FORWARD", HasArg: true, ArgKind: ArgJumpRel, Flags: OpJump | OpTerminator, StackEff: 0})
	set(JUMP_BACKWARD, OpMeta{Name: "JUMP_BACKWARD", HasArg: true, ArgKind: ArgJumpRel, Flags: OpJump | OpTerminator, StackEff: 0})
	set(FOR_ITER, OpMeta{Name: "FOR_ITER", HasArg: true, ArgKind: ArgJumpRel, Flags: OpJump, StackEff: 1})
	set(POP_JUMP_IF_FALSE, OpMeta{Name: "POP_JUMP_IF_FALSE", HasArg: true, ArgKind: ArgJumpRel, Flags: OpJump, StackEff: -1})
	set(POP_JUMP_IF_TRUE, OpMeta{Name: "POP_JUMP_IF_TRUE", HasArg: true, ArgKind: ArgJumpRel, Flags: OpJump, StackEff: -1})
	set(POP_JUMP_IF_NONE, OpMeta{Name: "POP_JUMP_IF_NONE", HasArg: true, ArgKind: ArgJumpRel, Flags: OpJump, StackEff: -1})
	set(POP_JUMP_IF_NOT_NONE, OpMeta{Name: "POP_JUMP_IF_NOT_NONE", HasArg: true, ArgKind: ArgJumpRel, Flags: OpJump, StackEff: -1})

	// Frame.
	set(RESUME, OpMeta{Name: "RESUME", HasArg: true, ArgKind: ArgRaw, StackEff: 0})

	return t
}()

// MetaOf returns the OpMeta for op. The zero OpMeta means the entry
// is not yet populated; callers should treat that as "unknown
// opcode" and back off.
func MetaOf(op Opcode) OpMeta {
	return OpMetaTable[op]
}
