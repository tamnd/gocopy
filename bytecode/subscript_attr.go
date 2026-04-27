package bytecode

// SubscriptLoadBytecode returns the instruction stream for `x = a[b]`
// (module-level subscript read, object and key are names).
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME 0 (a)
//	LOAD_NAME 1 (b)
//	BINARY_OP 26 ([]) + 5 cache words
//	STORE_NAME 2 (x)
//	LOAD_CONST 0 (None)
//	RETURN_VALUE 0
//
// co_names: [a, b, x]  co_consts: [None]  co_stacksize: 2
func SubscriptLoadBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), 0,
		byte(LOAD_NAME), 1,
		byte(BINARY_OP), NbGetItem,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 5 cache words
		byte(STORE_NAME), 2,
		byte(LOAD_CONST), 0,
		byte(RETURN_VALUE), 0,
	}
}

// SubscriptLoadLineTable returns the PEP 626 line table for `x = a[b]`.
//
// objCol/objEnd: column range of the object name (a).
// keyCol/keyEnd: column range of the key name (b).
// closeEnd: column after the closing `]` (= keyEnd + 1 for simple `a[b]`).
// targetLen: length of the target name x.
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. LOAD_NAME a: 1 code unit
//  3. LOAD_NAME b: 1 code unit
//  4. BINARY_OP + 5 caches: 6 code units at (objCol, closeEnd)
//  5. STORE_NAME + LOAD_CONST + RETURN_VALUE: 3 code units at (0, targetLen)
func SubscriptLoadLineTable(line int, objCol, objEnd, keyCol, keyEnd, closeEnd, targetLen byte) []byte {
	out := make([]byte, 0, 5+3+2+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)     // prologue
	out = appendValueEntry(out, line, objCol, objEnd)     // LOAD_NAME a
	out = appendSameLine(out, 1, keyCol, keyEnd)          // LOAD_NAME b
	out = appendSameLine(out, 6, objCol, closeEnd)        // BINARY_OP + 5 caches
	out = appendSameLine(out, 3, 0, targetLen)            // STORE_NAME + LC + RV
	return out
}

// SubscriptStoreBytecode returns the instruction stream for `a[b] = x`
// (module-level subscript store, object, key and value are names).
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME 0 (x)  -- value to store
//	LOAD_NAME 1 (a)  -- object
//	LOAD_NAME 2 (b)  -- key
//	STORE_SUBSCR 0 + 1 cache word
//	LOAD_CONST 0 (None)
//	RETURN_VALUE 0
//
// co_names: [x, a, b]  co_consts: [None]  co_stacksize: 3
func SubscriptStoreBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), 0,
		byte(LOAD_NAME), 1,
		byte(LOAD_NAME), 2,
		byte(STORE_SUBSCR), 0,
		0, 0, // 1 cache word
		byte(LOAD_CONST), 0,
		byte(RETURN_VALUE), 0,
	}
}

// SubscriptStoreLineTable returns the PEP 626 line table for `a[b] = x`.
//
// valCol/valEnd: column range of the value name (x).
// objCol/objEnd: column range of the object name (a).
// keyCol/keyEnd: column range of the key name (b).
// closeEnd: column after the closing `]` (= keyEnd + 1).
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. LOAD_NAME x: 1 code unit
//  3. LOAD_NAME a: 1 code unit
//  4. LOAD_NAME b: 1 code unit
//  5. STORE_SUBSCR + 1 cache + LOAD_CONST + RETURN_VALUE: 4 code units at (objCol, closeEnd)
func SubscriptStoreLineTable(line int, valCol, valEnd, objCol, objEnd, keyCol, keyEnd, closeEnd byte) []byte {
	out := make([]byte, 0, 5+3+2+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)     // prologue
	out = appendValueEntry(out, line, valCol, valEnd)     // LOAD_NAME x
	out = appendSameLine(out, 1, objCol, objEnd)          // LOAD_NAME a
	out = appendSameLine(out, 1, keyCol, keyEnd)          // LOAD_NAME b
	out = appendSameLine(out, 4, objCol, closeEnd)        // STORE_SUBSCR + cache + LC + RV
	return out
}

// AttrLoadBytecode returns the instruction stream for `x = a.b`
// (module-level attribute read, object is a name).
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME 0 (a)
//	LOAD_ATTR 2 (b)  -- oparg = attrIdx<<1 = 1<<1 = 2
//	  + 9 cache words (18 bytes)
//	STORE_NAME 2 (x)
//	LOAD_CONST 0 (None)
//	RETURN_VALUE 0
//
// co_names: [a, b, x]  co_consts: [None]  co_stacksize: 1
func AttrLoadBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), 0,
		byte(LOAD_ATTR), 2, // oparg = 1<<1 (attr 'b' at names[1])
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 9 cache words
		byte(STORE_NAME), 2,
		byte(LOAD_CONST), 0,
		byte(RETURN_VALUE), 0,
	}
}

// AttrLoadLineTable returns the PEP 626 line table for `x = a.b`.
//
// LOAD_ATTR spans 10 code units (opcode + 9 cache words), which exceeds the
// 8-code-unit per-entry limit, so it is split into two consecutive entries
// covering 8 + 2 code units at the same column range.
//
// objCol/objEnd: column range of the object name (a).
// attrEnd: exclusive end column of the attribute (= objCol+objLen+1+attrLen).
// targetLen: length of the target name x.
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. LOAD_NAME a: 1 code unit
//  3. LOAD_ATTR + 7 caches: 8 code units at (objCol, attrEnd)
//  4. Remaining 2 caches: 2 code units at (objCol, attrEnd)
//  5. STORE_NAME + LOAD_CONST + RETURN_VALUE: 3 code units at (0, targetLen)
func AttrLoadLineTable(line int, objCol, objEnd, attrEnd, targetLen byte) []byte {
	out := make([]byte, 0, 5+3+2+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)  // prologue
	out = appendValueEntry(out, line, objCol, objEnd)  // LOAD_NAME a
	out = appendSameLine(out, 8, objCol, attrEnd)      // LOAD_ATTR + 7 caches
	out = appendSameLine(out, 2, objCol, attrEnd)      // remaining 2 caches
	out = appendSameLine(out, 3, 0, targetLen)         // STORE_NAME + LC + RV
	return out
}

// AttrStoreBytecode returns the instruction stream for `a.b = x`
// (module-level attribute store, object and value are names).
//
// Bytecode pattern:
//
//	RESUME 0
//	LOAD_NAME 0 (x)   -- value to store
//	LOAD_NAME 1 (a)   -- object
//	STORE_ATTR 2 (b)  -- oparg = 2 (index of 'b' in names=[x,a,b])
//	  + 4 cache words (8 bytes)
//	LOAD_CONST 0 (None)
//	RETURN_VALUE 0
//
// co_names: [x, a, b]  co_consts: [None]  co_stacksize: 2
func AttrStoreBytecode() []byte {
	return []byte{
		byte(RESUME), 0,
		byte(LOAD_NAME), 0,
		byte(LOAD_NAME), 1,
		byte(STORE_ATTR), 2,
		0, 0, 0, 0, 0, 0, 0, 0, // 4 cache words
		byte(LOAD_CONST), 0,
		byte(RETURN_VALUE), 0,
	}
}

// AttrStoreLineTable returns the PEP 626 line table for `a.b = x`.
//
// STORE_ATTR + 4 caches + LOAD_CONST + RETURN_VALUE = 7 code units; they
// all share the column range of the assignment target `a.b`.
//
// valCol/valEnd: column range of the value name (x).
// objCol/objEnd: column range of the object name (a).
// attrEnd: exclusive end column of the attribute (= objCol+objLen+1+attrLen).
//
// Line table entries:
//  1. Prologue (RESUME)
//  2. LOAD_NAME x: 1 code unit
//  3. LOAD_NAME a: 1 code unit
//  4. STORE_ATTR + 4 caches + LOAD_CONST + RETURN_VALUE: 7 code units at (objCol, attrEnd)
func AttrStoreLineTable(line int, valCol, valEnd, objCol, objEnd, attrEnd byte) []byte {
	out := make([]byte, 0, 5+3+2+2)
	out = append(out, 0xf0, 0x03, 0x01, 0x01, 0x01)  // prologue
	out = appendValueEntry(out, line, valCol, valEnd)  // LOAD_NAME x
	out = appendSameLine(out, 1, objCol, objEnd)       // LOAD_NAME a
	out = appendSameLine(out, 7, objCol, attrEnd)      // STORE_ATTR + caches + LC + RV
	return out
}
