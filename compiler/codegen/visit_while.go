package codegen

import (
	"errors"
	"strconv"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// whileModule captures the modWhile shape: a top-level
// `while cond: name = val` loop where cond is a 1..15-char Name
// and val is a small int (0..255). No break/continue/else.
// Mirrors `compiler/classify_ast.go::extractWhileAssign` byte-for-
// byte.
type whileModule struct {
	CondName string
	CondCol  uint16
	CondEnd  uint16
	CondLine uint32
	BodyLine uint32
	BodyVal  byte
	VarName  string
	VarCol   uint16
	VarEnd   uint16
	ValCol   uint16
	ValEnd   uint16
}

// classifyWhileModule recognises a single-statement module whose
// only stmt is `*ast.While` matching the modWhile shape.
func classifyWhileModule(mod *ast.Module, _ []byte) (whileModule, bool) {
	if len(mod.Body) != 1 {
		return whileModule{}, false
	}
	w, ok := mod.Body[0].(*ast.While)
	if !ok {
		return whileModule{}, false
	}
	if len(w.Orelse) != 0 {
		return whileModule{}, false
	}
	cond, condOK := w.Test.(*ast.Name)
	if !condOK || cond.P.Col > 255 || len(cond.Id) < 1 || len(cond.Id) > 15 {
		return whileModule{}, false
	}
	if len(w.Body) != 1 {
		return whileModule{}, false
	}
	bodyAssign, isAssign := w.Body[0].(*ast.Assign)
	if !isAssign || len(bodyAssign.Targets) != 1 {
		return whileModule{}, false
	}
	varName, isName := bodyAssign.Targets[0].(*ast.Name)
	if !isName || varName.P.Col > 255 || len(varName.Id) < 1 || len(varName.Id) > 15 {
		return whileModule{}, false
	}
	constVal, isConst := bodyAssign.Value.(*ast.Constant)
	if !isConst || constVal.Kind != "int" || constVal.P.Col > 255 {
		return whileModule{}, false
	}
	iv, ok := constVal.Value.(int64)
	if !ok || iv < 0 || iv > 255 {
		return whileModule{}, false
	}
	condCol := uint16(cond.P.Col)
	valCol := uint16(constVal.P.Col)
	varCol := uint16(varName.P.Col)
	return whileModule{
		CondName: cond.Id,
		CondCol:  condCol,
		CondEnd:  condCol + uint16(len(cond.Id)),
		CondLine: uint32(w.P.Line),
		BodyLine: uint32(bodyAssign.P.Line),
		BodyVal:  byte(iv),
		VarName:  varName.Id,
		VarCol:   varCol,
		VarEnd:   varCol + uint16(len(varName.Id)),
		ValCol:   valCol,
		ValEnd:   valCol + uint16(len(strconv.Itoa(int(iv)))),
	}, true
}

// buildWhileModule emits the bytecode CPython 3.14 generates for a
// `while cond: name = val` loop at module scope. Mirrors
// `compiler/compiler.go::compileWhile` byte-for-byte.
//
// Layout:
//
//	RESUME 0                              ; synthetic prologue
//	LOAD_NAME condIdx           at L_cond ; 1 cu
//	TO_BOOL 0                   at L_cond ; +3 caches → 4 cu
//	POP_JUMP_IF_FALSE 5         at L_cond ; +1 cache  → 2 cu
//	NOT_TAKEN 0                 at L_cond ; 1 cu (8-cu run merges)
//	LOAD_SMALL_INT bodyVal      at L_val  ; 1 cu
//	STORE_NAME varIdx           at L_var  ; 1 cu
//	JUMP_BACKWARD 12            at L_var  ; +1 cache → 2 cu (3-cu run merges)
//	LOAD_CONST noneIdx          at L_cond ; 1 cu (LONG, lineDelta<0)
//	RETURN_VALUE 0              at L_cond ; 1 cu (2-cu run merges)
//
// Jump opargs (constants for this exact shape):
//   - POP_JUMP_IF_FALSE 5: NOT_TAKEN+LOAD_SMALL_INT+STORE_NAME+JUMP_BACKWARD+cache
//   - JUMP_BACKWARD 12: back from end-of-(JUMP_BACKWARD+cache) to LOAD_NAME
//
// co_consts = [int64(BodyVal), nil].
// co_names: insertion-order, deduped — condName then varName, with
// same-name dedupe collapsing both to one entry when condName ==
// varName.
// co.StackSize = 1.
func buildWhileModule(w whileModule, opts Options) (*bytecode.CodeObject, error) {
	if w.CondName == "" || w.VarName == "" {
		return nil, errors.New("codegen.buildWhileModule: empty cond or var name")
	}

	nameIdx := map[string]uint32{}
	names := []string{}
	addName := func(s string) uint32 {
		if idx, ok := nameIdx[s]; ok {
			return idx
		}
		idx := uint32(len(names))
		nameIdx[s] = idx
		names = append(names, s)
		return idx
	}
	condIdx := addName(w.CondName)
	varIdx := addName(w.VarName)

	condLoc := bytecode.Loc{
		Line: w.CondLine, EndLine: w.CondLine,
		Col: w.CondCol, EndCol: w.CondEnd,
	}
	valLoc := bytecode.Loc{
		Line: w.BodyLine, EndLine: w.BodyLine,
		Col: w.ValCol, EndCol: w.ValEnd,
	}
	varLoc := bytecode.Loc{
		Line: w.BodyLine, EndLine: w.BodyLine,
		Col: w.VarCol, EndCol: w.VarEnd,
	}
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	noneIdx := uint32(1)

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: condIdx, Loc: condLoc},
		ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: condLoc},
		ir.Instr{Op: bytecode.POP_JUMP_IF_FALSE, Arg: 5, Loc: condLoc},
		ir.Instr{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc},
		ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: uint32(w.BodyVal), Loc: valLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: varIdx, Loc: varLoc},
		ir.Instr{Op: bytecode.JUMP_BACKWARD, Arg: 12, Loc: varLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: condLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: condLoc},
	)

	consts := []any{int64(w.BodyVal), nil}

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   consts,
		Names:    names,
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 1 {
		co.StackSize = 1
	}
	return co, nil
}
