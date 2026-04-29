package codegen

import (
	"errors"
	"strconv"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// ifElseModuleBranch holds one condition+body pair from an
// `if cond: name = val` / `elif cond: name = val` chain.
type ifElseModuleBranch struct {
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

// ifElseModule captures the modIfElse shape: an if/elif/else chain
// where every condition is a 1..15-char Name and every body is a
// single `name = small_int` (0..255) assignment. Mirrors
// `compiler/classify.go::ifElseClassify` byte-for-byte.
type ifElseModule struct {
	Branches    []ifElseModuleBranch
	HasElse     bool
	ElseLine    uint32
	ElseVal     byte
	ElseVarName string
	ElseVarCol  uint16
	ElseVarEnd  uint16
	ElseValCol  uint16
	ElseValEnd  uint16
}

// classifyIfElseModule recognises a single-statement module whose
// only stmt is a top-level `if` chain matching the modIfElse shape.
func classifyIfElseModule(mod *ast.Module, _ []byte) (ifElseModule, bool) {
	if len(mod.Body) != 1 {
		return ifElseModule{}, false
	}
	first, ok := mod.Body[0].(*ast.If)
	if !ok {
		return ifElseModule{}, false
	}
	out := ifElseModule{}
	cur := first
	for cur != nil {
		cond, condOK := cur.Test.(*ast.Name)
		if !condOK || cond.P.Col > 255 || len(cond.Id) < 1 || len(cond.Id) > 15 {
			return ifElseModule{}, false
		}
		if len(cur.Body) != 1 {
			return ifElseModule{}, false
		}
		bodyAssign, isAssign := cur.Body[0].(*ast.Assign)
		if !isAssign || len(bodyAssign.Targets) != 1 {
			return ifElseModule{}, false
		}
		varName, isName := bodyAssign.Targets[0].(*ast.Name)
		if !isName || varName.P.Col > 255 || len(varName.Id) < 1 || len(varName.Id) > 15 {
			return ifElseModule{}, false
		}
		constVal, isConst := bodyAssign.Value.(*ast.Constant)
		if !isConst || constVal.Kind != "int" || constVal.P.Col > 255 {
			return ifElseModule{}, false
		}
		iv, ok := constVal.Value.(int64)
		if !ok || iv < 0 || iv > 255 {
			return ifElseModule{}, false
		}
		condCol := uint16(cond.P.Col)
		valCol := uint16(constVal.P.Col)
		varCol := uint16(varName.P.Col)
		out.Branches = append(out.Branches, ifElseModuleBranch{
			CondName: cond.Id,
			CondCol:  condCol,
			CondEnd:  condCol + uint16(len(cond.Id)),
			CondLine: uint32(cur.P.Line),
			BodyLine: uint32(bodyAssign.P.Line),
			BodyVal:  byte(iv),
			VarName:  varName.Id,
			VarCol:   varCol,
			VarEnd:   varCol + uint16(len(varName.Id)),
			ValCol:   valCol,
			ValEnd:   valCol + uint16(len(strconv.Itoa(int(iv)))),
		})
		switch len(cur.Orelse) {
		case 0:
			cur = nil
		case 1:
			if elif, isIf := cur.Orelse[0].(*ast.If); isIf {
				cur = elif
				continue
			}
			elseAssign, isAssign2 := cur.Orelse[0].(*ast.Assign)
			if !isAssign2 || len(elseAssign.Targets) != 1 {
				return ifElseModule{}, false
			}
			ev, isName2 := elseAssign.Targets[0].(*ast.Name)
			if !isName2 || ev.P.Col > 255 || len(ev.Id) < 1 || len(ev.Id) > 15 {
				return ifElseModule{}, false
			}
			ec, isConst2 := elseAssign.Value.(*ast.Constant)
			if !isConst2 || ec.Kind != "int" || ec.P.Col > 255 {
				return ifElseModule{}, false
			}
			eiv, ok := ec.Value.(int64)
			if !ok || eiv < 0 || eiv > 255 {
				return ifElseModule{}, false
			}
			elseVarCol := uint16(ev.P.Col)
			elseValCol := uint16(ec.P.Col)
			out.HasElse = true
			out.ElseLine = uint32(elseAssign.P.Line)
			out.ElseVal = byte(eiv)
			out.ElseVarName = ev.Id
			out.ElseVarCol = elseVarCol
			out.ElseVarEnd = elseVarCol + uint16(len(ev.Id))
			out.ElseValCol = elseValCol
			out.ElseValEnd = elseValCol + uint16(len(strconv.Itoa(int(eiv))))
			cur = nil
		default:
			return ifElseModule{}, false
		}
	}
	if len(out.Branches) == 0 {
		return ifElseModule{}, false
	}
	return out, true
}

// buildIfElseModule emits the bytecode CPython 3.14 generates for an
// if/elif/else chain at module scope. Mirrors
// `compiler/compiler.go::compileIfElse` byte-for-byte.
//
// Layout per branch (8-cu condition run + 1-cu LOAD_SMALL_INT +
// 3-cu STORE/LOAD_CONST/RETURN run):
//
//	LOAD_NAME condIdx      at L_cond
//	TO_BOOL 0              at L_cond  ; +3 cache words
//	POP_JUMP_IF_FALSE 5    at L_cond  ; +1 cache word
//	NOT_TAKEN 0            at L_cond
//	LOAD_SMALL_INT bodyVal at L_val
//	STORE_NAME varIdx      at L_var
//	LOAD_CONST noneIdx     at L_var
//	RETURN_VALUE 0         at L_var
//
// Else body (when HasElse):
//
//	LOAD_SMALL_INT elseVal at L_elseVal
//	STORE_NAME elseVarIdx  at L_elseVar
//	LOAD_CONST noneIdx     at L_elseVar
//	RETURN_VALUE 0         at L_elseVar
//
// No-else implicit return None: 2 code units (LOAD_CONST +
// RETURN_VALUE) attributed back to the first condition's source
// position. The assembler's encoder emits this as a LONG entry with
// negative `lineDelta` and `col+1`/`endCol+1` payloads — exactly
// matching the classifier's hand-written LONG entry.
//
// co_consts = [int64(branches[0].BodyVal), nil].
// co_names  = insertion-order, deduped: condName, varName per
// branch, then else varName when HasElse.
// co.StackSize = 1.
func buildIfElseModule(m ifElseModule, opts Options) (*bytecode.CodeObject, error) {
	if len(m.Branches) == 0 {
		return nil, errors.New("codegen.buildIfElseModule: no branches")
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

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	noneIdx := uint32(1)

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
	)

	for _, br := range m.Branches {
		condIdx := addName(br.CondName)
		varIdx := addName(br.VarName)
		condLoc := bytecode.Loc{
			Line: br.CondLine, EndLine: br.CondLine,
			Col: br.CondCol, EndCol: br.CondEnd,
		}
		valLoc := bytecode.Loc{
			Line: br.BodyLine, EndLine: br.BodyLine,
			Col: br.ValCol, EndCol: br.ValEnd,
		}
		varLoc := bytecode.Loc{
			Line: br.BodyLine, EndLine: br.BodyLine,
			Col: br.VarCol, EndCol: br.VarEnd,
		}
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_NAME, Arg: condIdx, Loc: condLoc},
			ir.Instr{Op: bytecode.TO_BOOL, Arg: 0, Loc: condLoc},
			ir.Instr{Op: bytecode.POP_JUMP_IF_FALSE, Arg: 5, Loc: condLoc},
			ir.Instr{Op: bytecode.NOT_TAKEN, Arg: 0, Loc: condLoc},
			ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: uint32(br.BodyVal), Loc: valLoc},
			ir.Instr{Op: bytecode.STORE_NAME, Arg: varIdx, Loc: varLoc},
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: varLoc},
			ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: varLoc},
		)
	}

	if m.HasElse {
		elseVarIdx := addName(m.ElseVarName)
		elseValLoc := bytecode.Loc{
			Line: m.ElseLine, EndLine: m.ElseLine,
			Col: m.ElseValCol, EndCol: m.ElseValEnd,
		}
		elseVarLoc := bytecode.Loc{
			Line: m.ElseLine, EndLine: m.ElseLine,
			Col: m.ElseVarCol, EndCol: m.ElseVarEnd,
		}
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: uint32(m.ElseVal), Loc: elseValLoc},
			ir.Instr{Op: bytecode.STORE_NAME, Arg: elseVarIdx, Loc: elseVarLoc},
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: elseVarLoc},
			ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: elseVarLoc},
		)
	} else {
		// Implicit return None: attribute the 2-cu run back to the
		// first condition's source position. The encoder picks LONG
		// because lineDelta is negative.
		first := m.Branches[0]
		tailLoc := bytecode.Loc{
			Line: first.CondLine, EndLine: first.CondLine,
			Col: first.CondCol, EndCol: first.CondEnd,
		}
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: tailLoc},
			ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: tailLoc},
		)
	}

	consts := []any{int64(m.Branches[0].BodyVal), nil}

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
