package codegen

import (
	"bytes"
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// multiAssignModule captures a module body whose first N >= 2
// statements are independent single-target simple-constant
// assignments, optionally followed by no-op statements.
//
// The negLiteral / foldedBinOp value sub-shapes from the
// classifier's modMultiAssign are NOT covered here — those need
// the constant-fold pass.
type multiAssignModule struct {
	Asgns []assignOne
	Tail  []noOpStmt
}

type assignOne struct {
	Line     uint32
	Name     string
	NameLen  uint16
	Value    any
	ValStart uint16
	ValEnd   uint16
}

// classifyMultiAssignModule recognizes the simple-constant
// multi-assignment shape (N >= 2 independent assignments).
// Returns the captured fields on success.
func classifyMultiAssignModule(mod *ast.Module, source []byte) (multiAssignModule, bool) {
	if len(mod.Body) < 2 {
		return multiAssignModule{}, false
	}
	lines := splitLines(source)

	// Walk leading Assign statements; require at least two.
	asgns := make([]assignOne, 0, len(mod.Body))
	idx := 0
	for idx < len(mod.Body) {
		a, ok := mod.Body[idx].(*ast.Assign)
		if !ok {
			break
		}
		one, ok := classifyAssignOne(a, lines)
		if !ok {
			return multiAssignModule{}, false
		}
		asgns = append(asgns, one)
		idx++
	}
	if len(asgns) < 2 {
		return multiAssignModule{}, false
	}

	tail := make([]noOpStmt, 0, len(mod.Body)-idx)
	for _, stmt := range mod.Body[idx:] {
		s, ok := classifyTailNoOp(stmt, lines)
		if !ok {
			return multiAssignModule{}, false
		}
		tail = append(tail, s)
	}
	return multiAssignModule{Asgns: asgns, Tail: tail}, true
}

// classifyAssignOne classifies one `<name> = <const>` statement
// for both the multi-assign and the basic-assign shapes.
func classifyAssignOne(a *ast.Assign, lines [][]byte) (assignOne, bool) {
	if len(a.Targets) != 1 {
		return assignOne{}, false
	}
	name, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return assignOne{}, false
	}
	if n := len(name.Id); n < 1 || n > 15 {
		return assignOne{}, false
	}
	c, ok := a.Value.(*ast.Constant)
	if !ok {
		return assignOne{}, false
	}
	val, ok := constantToValue(c)
	if !ok {
		return assignOne{}, false
	}
	if sv, isStr := val.(string); isStr {
		for i := 0; i < len(sv); i++ {
			if sv[i] == '\n' {
				return assignOne{}, false
			}
		}
		if !isPlainAsciiNoEscape(sv) {
			return assignOne{}, false
		}
	}
	if c.P.Col > 255 {
		return assignOne{}, false
	}
	line := a.P.Line
	if line < 1 {
		return assignOne{}, false
	}
	ec, ok := lineEndCol(lines, line)
	if !ok {
		return assignOne{}, false
	}
	return assignOne{
		Line:     uint32(line),
		Name:     name.Id,
		NameLen:  uint16(len(name.Id)),
		Value:    val,
		ValStart: uint16(c.P.Col),
		ValEnd:   uint16(ec),
	}, true
}

// buildMultiAssignModule emits the bytecode CPython generates
// for N >= 2 `<name> = <const>` statements followed by M no-op
// tail statements.
//
// Layout:
//
//	RESUME 0
//	[for each i in 0..N-1:
//	  LOAD_SMALL_INT <val>  (int64 in 0..255)
//	  or LOAD_CONST <idx>
//	  STORE_NAME <nameIdx[i]>]
//	[NOPs for tail[0..M-2] if M >= 2]
//	LOAD_CONST <noneIdx>
//	RETURN_VALUE
//
// The trailing pair shares the *last assignment's* name Loc when
// there is no tail (so the encoder merges the final STORE_NAME +
// the trailing pair into one 3-unit SHORT0 entry); otherwise it
// shares the last tail stmt's Loc.
//
// co_consts encodes values in encounter order with deduplication
// for equal scalars; small-int LOAD_SMALL_INT values still get a
// phantom slot at index 0 only when they're the *first*
// assignment's value (mirrors compileMultiAssign).
//
// co_names dedups while preserving first-seen order.
func buildMultiAssignModule(m multiAssignModule, opts Options) (*bytecode.CodeObject, error) {
	if len(m.Asgns) < 2 {
		return nil, errors.New("codegen.buildMultiAssignModule: need >=2 assignments")
	}

	// Build dedup'd names with insertion order.
	namesIdx := map[string]uint32{}
	var names []string
	nameIdxs := make([]uint32, len(m.Asgns))
	for i, a := range m.Asgns {
		if idx, ok := namesIdx[a.Name]; ok {
			nameIdxs[i] = idx
		} else {
			idx := uint32(len(names))
			namesIdx[a.Name] = idx
			names = append(names, a.Name)
			nameIdxs[i] = idx
		}
	}

	// Build co_consts and per-assignment load info, mirroring the
	// classifier exactly.
	var consts []any
	loadSmall := make([]bool, len(m.Asgns))
	loadIdx := make([]uint32, len(m.Asgns))

	constIdx := func(v any) int {
		if bv, ok := v.([]byte); ok {
			for j, c := range consts {
				if bc, ok2 := c.([]byte); ok2 && bytes.Equal(bv, bc) {
					return j
				}
			}
			return -1
		}
		for j, c := range consts {
			if c == v {
				return j
			}
		}
		return -1
	}
	addConst := func(v any) uint32 {
		if idx := constIdx(v); idx >= 0 {
			return uint32(idx)
		}
		idx := uint32(len(consts))
		consts = append(consts, v)
		return idx
	}

	for i, a := range m.Asgns {
		switch tv := a.Value.(type) {
		case int64:
			if tv >= 0 && tv <= 255 {
				loadSmall[i] = true
				if i == 0 {
					addConst(tv) // phantom slot at co_consts[0]
				}
			} else {
				loadIdx[i] = addConst(tv)
			}
		case nil:
			loadIdx[i] = addConst(nil)
		default:
			loadIdx[i] = addConst(a.Value)
		}
	}

	// Add None if not present; record noneIdx.
	noneIdx := addConst(nil)

	// Build the IR.
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
	)

	var lastNameLoc bytecode.Loc
	for i, a := range m.Asgns {
		valLoc := bytecode.Loc{
			Line: a.Line, EndLine: a.Line,
			Col: a.ValStart, EndCol: a.ValEnd,
		}
		nameLoc := bytecode.Loc{
			Line: a.Line, EndLine: a.Line,
			Col: 0, EndCol: a.NameLen,
		}
		if loadSmall[i] {
			block.Instrs = append(block.Instrs, ir.Instr{
				Op: bytecode.LOAD_SMALL_INT, Arg: uint32(a.Value.(int64)), Loc: valLoc,
			})
		} else {
			block.Instrs = append(block.Instrs, ir.Instr{
				Op: bytecode.LOAD_CONST, Arg: loadIdx[i], Loc: valLoc,
			})
		}
		block.Instrs = append(block.Instrs, ir.Instr{
			Op: bytecode.STORE_NAME, Arg: nameIdxs[i], Loc: nameLoc,
		})
		lastNameLoc = nameLoc
	}

	// Tail NOPs (all but the last tail stmt).
	lastLoc := lastNameLoc
	if len(m.Tail) > 0 {
		for _, s := range m.Tail[:len(m.Tail)-1] {
			block.Instrs = append(block.Instrs, ir.Instr{
				Op: bytecode.NOP, Arg: 0, Loc: locOf(s),
			})
		}
		lastLoc = locOf(m.Tail[len(m.Tail)-1])
	}

	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: lastLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: lastLoc},
	)

	return assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   consts,
		Names:    names,
	})
}
