package codegen

import (
	"errors"
	"strconv"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// forModule captures the modFor shape: a top-level
// `for loopVar in iter: bodyVar = val` loop where iter and
// loopVar are 1..15-char Names and val is a small int (0..255).
// No break/continue/else. Mirrors `compiler/classify_ast.go::
// extractForAssign` byte-for-byte.
type forModule struct {
	IterName    string
	IterCol     uint16
	IterEnd     uint16
	LoopVarName string
	LoopVarCol  uint16
	LoopVarEnd  uint16
	ForLine     uint32
	BodyLine    uint32
	BodyVal     byte
	BodyVarName string
	BodyVarCol  uint16
	BodyVarEnd  uint16
	ValCol      uint16
	ValEnd      uint16
}

// classifyForModule recognises a single-statement module whose only
// stmt is `*ast.For` matching the modFor shape.
func classifyForModule(mod *ast.Module, _ []byte) (forModule, bool) {
	if len(mod.Body) != 1 {
		return forModule{}, false
	}
	f, ok := mod.Body[0].(*ast.For)
	if !ok {
		return forModule{}, false
	}
	if len(f.Orelse) != 0 {
		return forModule{}, false
	}
	iter, iterOK := f.Iter.(*ast.Name)
	if !iterOK || iter.P.Col > 255 || len(iter.Id) < 1 || len(iter.Id) > 15 {
		return forModule{}, false
	}
	loopVar, loopVarOK := f.Target.(*ast.Name)
	if !loopVarOK || loopVar.P.Col > 255 || len(loopVar.Id) < 1 || len(loopVar.Id) > 15 {
		return forModule{}, false
	}
	if len(f.Body) != 1 {
		return forModule{}, false
	}
	bodyAssign, isAssign := f.Body[0].(*ast.Assign)
	if !isAssign || len(bodyAssign.Targets) != 1 {
		return forModule{}, false
	}
	bodyVar, isName := bodyAssign.Targets[0].(*ast.Name)
	if !isName || bodyVar.P.Col > 255 || len(bodyVar.Id) < 1 || len(bodyVar.Id) > 15 {
		return forModule{}, false
	}
	constVal, isConst := bodyAssign.Value.(*ast.Constant)
	if !isConst || constVal.Kind != "int" || constVal.P.Col > 255 {
		return forModule{}, false
	}
	iv, ok := constVal.Value.(int64)
	if !ok || iv < 0 || iv > 255 {
		return forModule{}, false
	}
	iterCol := uint16(iter.P.Col)
	loopVarCol := uint16(loopVar.P.Col)
	bodyVarCol := uint16(bodyVar.P.Col)
	valCol := uint16(constVal.P.Col)
	return forModule{
		IterName:    iter.Id,
		IterCol:     iterCol,
		IterEnd:     iterCol + uint16(len(iter.Id)),
		LoopVarName: loopVar.Id,
		LoopVarCol:  loopVarCol,
		LoopVarEnd:  loopVarCol + uint16(len(loopVar.Id)),
		ForLine:     uint32(f.P.Line),
		BodyLine:    uint32(bodyAssign.P.Line),
		BodyVal:     byte(iv),
		BodyVarName: bodyVar.Id,
		BodyVarCol:  bodyVarCol,
		BodyVarEnd:  bodyVarCol + uint16(len(bodyVar.Id)),
		ValCol:      valCol,
		ValEnd:      valCol + uint16(len(strconv.Itoa(int(iv)))),
	}, true
}

// buildForModule emits the bytecode CPython 3.14 generates for a
// `for loopVar in iter: bodyVar = val` loop at module scope. Mirrors
// `compiler/compiler.go::compileFor` byte-for-byte.
//
// Layout:
//
//	RESUME 0                              ; synthetic
//	LOAD_NAME iterIdx           at iterLoc    ; 1 cu
//	GET_ITER 0                  at iterLoc    ; 1 cu
//	FOR_ITER 5                  at iterLoc    ; +1 cache → 2 cu (4-cu run merges)
//	STORE_NAME loopVarIdx       at loopVarLoc ; 1 cu
//	LOAD_SMALL_INT bodyVal      at valLoc     ; 1 cu
//	STORE_NAME bodyVarIdx       at bodyVarLoc ; 1 cu
//	JUMP_BACKWARD 7             at bodyVarLoc ; +1 cache → 2 cu (3-cu run merges)
//	END_FOR 0                   at iterLoc    ; 1 cu  (LONG, lineDelta<0)
//	POP_ITER 0                  at iterLoc    ; 1 cu
//	LOAD_CONST noneIdx          at iterLoc    ; 1 cu
//	RETURN_VALUE 0              at iterLoc    ; 1 cu  (4-cu run merges)
//
// Jump opargs (constants for this exact shape):
//   - FOR_ITER 5: STORE_NAME+LOAD_SMALL_INT+STORE_NAME+JUMP_BACKWARD+cache
//   - JUMP_BACKWARD 7: from end-of-(JUMP_BACKWARD+cache) back to FOR_ITER
//
// co_consts = [int64(BodyVal), nil].
// co_names: insertion-order, deduped — iterName, loopVarName,
// bodyVarName.
// co.StackSize = 2 (iterator stays on the stack while the body
// runs).
func buildForModule(f forModule, opts Options) (*bytecode.CodeObject, error) {
	if f.IterName == "" || f.LoopVarName == "" || f.BodyVarName == "" {
		return nil, errors.New("codegen.buildForModule: empty iter/loopVar/bodyVar name")
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
	iterIdx := addName(f.IterName)
	loopVarIdx := addName(f.LoopVarName)
	bodyVarIdx := addName(f.BodyVarName)

	iterLoc := bytecode.Loc{
		Line: f.ForLine, EndLine: f.ForLine,
		Col: f.IterCol, EndCol: f.IterEnd,
	}
	loopVarLoc := bytecode.Loc{
		Line: f.ForLine, EndLine: f.ForLine,
		Col: f.LoopVarCol, EndCol: f.LoopVarEnd,
	}
	valLoc := bytecode.Loc{
		Line: f.BodyLine, EndLine: f.BodyLine,
		Col: f.ValCol, EndCol: f.ValEnd,
	}
	bodyVarLoc := bytecode.Loc{
		Line: f.BodyLine, EndLine: f.BodyLine,
		Col: f.BodyVarCol, EndCol: f.BodyVarEnd,
	}
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	noneIdx := uint32(1)

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: iterIdx, Loc: iterLoc},
		ir.Instr{Op: bytecode.GET_ITER, Arg: 0, Loc: iterLoc},
		ir.Instr{Op: bytecode.FOR_ITER, Arg: 5, Loc: iterLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: loopVarIdx, Loc: loopVarLoc},
		ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: uint32(f.BodyVal), Loc: valLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: bodyVarIdx, Loc: bodyVarLoc},
		ir.Instr{Op: bytecode.JUMP_BACKWARD, Arg: 7, Loc: bodyVarLoc},
		ir.Instr{Op: bytecode.END_FOR, Arg: 0, Loc: iterLoc},
		ir.Instr{Op: bytecode.POP_ITER, Arg: 0, Loc: iterLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: iterLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: iterLoc},
	)

	consts := []any{int64(f.BodyVal), nil}

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
	if co.StackSize < 2 {
		co.StackSize = 2
	}
	return co, nil
}
