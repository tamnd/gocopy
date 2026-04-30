package flowgraph

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// FoldTupleOfConstants collapses
//
//	LOAD_CONST c1, ..., LOAD_CONST cn, BUILD_TUPLE n
//
// into
//
//	LOAD_CONST (c1, ..., cn)
//
// when every loader is a const-loading instruction (LOAD_CONST or
// LOAD_SMALL_INT). The folded tuple is appended to the consts pool
// and the BUILD_TUPLE is rewritten to LOAD_CONST(newIdx); the n
// loaders are removed from their block. Returns the (possibly
// extended) consts pool.
//
// Mixed sequences (e.g., one LOAD_NAME among the loaders) leave the
// BUILD_TUPLE unchanged.
//
// SOURCE: CPython 3.14 Python/flowgraph.c:1454 fold_tuple_of_constants +
// :1293 get_const_value + :1350 get_const_loading_instrs +
// :1412 instr_make_load_const.
//
// Out of scope (separate v0.7.10.x sub-releases):
//
//   - The BUILD_TUPLE(n) UNPACK_SEQUENCE(n) special-cases at
//     Python/flowgraph.c:2310 (n=1 → NOP/NOP; n=2,3 → SWAP).
//   - fold_constant_intrinsic_list_to_tuple (BUILD_LIST + LIST_APPEND
//     + CALL_INTRINSIC_1 → LOAD_CONST tuple).
func FoldTupleOfConstants(seq *ir.InstrSeq, consts []any) []any {
	if seq == nil {
		return consts
	}
	for _, b := range seq.Blocks {
		consts = foldTupleOfConstantsInBlock(b, consts)
	}
	return consts
}

// foldTupleOfConstantsInBlock walks a single block and folds each
// foldable BUILD_TUPLE in place, rebuilding b.Instrs with the
// matching loader instructions removed.
func foldTupleOfConstantsInBlock(b *ir.Block, consts []any) []any {
	if b == nil {
		return consts
	}
	// Collect indices to drop, then rebuild Instrs in one pass.
	drop := make(map[int]bool)
	for i := range b.Instrs {
		inst := &b.Instrs[i]
		if inst.Op != bytecode.BUILD_TUPLE {
			continue
		}
		n := int(inst.Arg)
		if n <= 0 {
			continue
		}
		loaders := getConstLoadingInstrs(b, i-1, n, drop)
		if loaders == nil {
			continue
		}
		tup := make(bytecode.ConstTuple, n)
		for k, ldIdx := range loaders {
			tup[k] = constValueOfLoader(&b.Instrs[ldIdx], consts)
		}
		// Rewrite BUILD_TUPLE → LOAD_CONST newIdx.
		consts = append(consts, tup)
		newIdx := uint32(len(consts) - 1)
		inst.Op = bytecode.LOAD_CONST
		inst.Arg = newIdx
		// Mark the loaders for removal.
		for _, ldIdx := range loaders {
			drop[ldIdx] = true
		}
	}
	if len(drop) == 0 {
		return consts
	}
	out := make([]ir.Instr, 0, len(b.Instrs)-len(drop))
	for i, inst := range b.Instrs {
		if drop[i] {
			continue
		}
		out = append(out, inst)
	}
	b.Instrs = out
	return consts
}

// getConstLoadingInstrs walks backwards from start collecting n
// consecutive const-loading instructions (LOAD_CONST or
// LOAD_SMALL_INT). Returns the indices in source order
// (oldest-first), or nil if fewer than n loaders are found before
// hitting a non-loading instruction. Skips over instructions already
// marked for removal (so a later BUILD_TUPLE cannot reuse a loader
// already claimed by an earlier fold).
//
// SOURCE: CPython 3.14 Python/flowgraph.c:1367 get_const_loading_instrs.
func getConstLoadingInstrs(b *ir.Block, start, n int, drop map[int]bool) []int {
	out := make([]int, 0, n)
	for i := start; i >= 0 && len(out) < n; i-- {
		if drop[i] {
			return nil
		}
		op := b.Instrs[i].Op
		if op == bytecode.LOAD_CONST || op == bytecode.LOAD_SMALL_INT {
			out = append(out, i)
			continue
		}
		return nil
	}
	if len(out) != n {
		return nil
	}
	// Reverse to source order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// constValueOfLoader returns the Python-level value loaded by inst.
// LOAD_CONST reads from the consts pool; LOAD_SMALL_INT encodes the
// value directly in oparg as int64 (CPython's small-int range is
// 0..255 and the assembler treats the oparg as unsigned).
//
// SOURCE: CPython 3.14 Python/flowgraph.c:1294 get_const_value.
func constValueOfLoader(inst *ir.Instr, consts []any) any {
	switch inst.Op {
	case bytecode.LOAD_CONST:
		return consts[inst.Arg]
	case bytecode.LOAD_SMALL_INT:
		return int64(inst.Arg)
	}
	return nil
}
