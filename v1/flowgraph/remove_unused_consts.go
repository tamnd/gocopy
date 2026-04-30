package flowgraph

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

// RemoveUnusedConsts condenses the const pool to entries reachable
// from LOAD_CONST instructions that survive the IR. Slot 0 is always
// preserved — CPython's pyc layout treats consts[0] as the implicit
// docstring slot, so removing it would shift every other index even
// when the docstring is None. Returns the condensed pool and rewrites
// every surviving LOAD_CONST oparg to its new index.
//
// SOURCE: CPython 3.14 Python/flowgraph.c:3174 remove_unused_consts.
//
// The CPython implementation builds an index_map[old]=new array and
// rewrites bytecode in two passes (mark used, then remap). gocopy
// follows the same structure, with two simplifications:
//
//   - The only opcode with HAS_CONST_FLAG that gocopy currently
//     emits is LOAD_CONST. CPython 3.14 also has LOAD_CONST_IMMORTAL
//     and LOAD_CONST_MORTAL, but those are post-specialization
//     forms the visitor never produces.
//   - The pool is a Go []any slice rather than a PyListObject, so
//     condensing is `consts[i] = consts[oldIdx]` followed by a
//     reslice instead of PyList_SetItem + PyList_SetSlice.
//
// Returns the original slice when no entries were dropped.
func RemoveUnusedConsts(seq *ir.InstrSeq, consts []any) []any {
	if seq == nil || len(consts) == 0 {
		return consts
	}

	indexMap := make([]int, len(consts))
	for i := 1; i < len(consts); i++ {
		indexMap[i] = -1
	}
	indexMap[0] = 0

	for _, b := range seq.Blocks {
		for _, inst := range b.Instrs {
			if !opcodeHasConst(inst.Op) {
				continue
			}
			if int(inst.Arg) >= len(consts) {
				continue
			}
			indexMap[inst.Arg] = int(inst.Arg)
		}
	}

	nUsed := 0
	for i := range len(consts) {
		if indexMap[i] != -1 {
			indexMap[nUsed] = indexMap[i]
			nUsed++
		}
	}
	if nUsed == len(consts) {
		return consts
	}

	newConsts := make([]any, nUsed)
	for i := range nUsed {
		newConsts[i] = consts[indexMap[i]]
	}

	reverseIndexMap := make([]int, len(consts))
	for i := range len(consts) {
		reverseIndexMap[i] = -1
	}
	for i := range nUsed {
		reverseIndexMap[indexMap[i]] = i
	}

	for _, b := range seq.Blocks {
		for i := range b.Instrs {
			inst := &b.Instrs[i]
			if !opcodeHasConst(inst.Op) {
				continue
			}
			if int(inst.Arg) >= len(reverseIndexMap) {
				continue
			}
			ni := reverseIndexMap[inst.Arg]
			if ni < 0 {
				continue
			}
			inst.Arg = uint32(ni)
		}
	}

	return newConsts
}

// opcodeHasConst reports whether op references the const pool via
// its oparg. CPython 3.14 marks LOAD_CONST and its specialised
// variants LOAD_CONST_IMMORTAL / LOAD_CONST_MORTAL with
// HAS_CONST_FLAG. The visitor only ever emits LOAD_CONST.
//
// SOURCE: CPython 3.14 Include/internal/pycore_opcode_metadata.h
// OPCODE_HAS_CONST.
func opcodeHasConst(op bytecode.Opcode) bool {
	return op == bytecode.LOAD_CONST
}
