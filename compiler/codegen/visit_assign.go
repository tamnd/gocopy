package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// assignModule captures a module body whose first statement is a
// single-target assignment to a Name with a non-folded simple
// constant RHS, optionally followed by no-op statements.
//
// negLiteral (`x = -1`) and foldedBinOp (`x = 1 + 2`) sub-shapes
// of the classifier's modAssign are NOT covered here — those need
// constant-folding wiring that lives in a later sub-release.
type assignModule struct {
	Line     uint32
	Name     string
	NameLen  uint16
	Value    any // nil | bool | int64 | float64 | complex128 | string | []byte | bytecode.EllipsisType
	ValStart uint16
	ValEnd   uint16
	Tail     []noOpStmt
}

// classifyAssignModule recognizes the simple-constant assignment
// shape. Returns the captured fields on success.
func classifyAssignModule(mod *ast.Module, source []byte) (assignModule, bool) {
	if len(mod.Body) == 0 {
		return assignModule{}, false
	}
	first, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return assignModule{}, false
	}
	if len(first.Targets) != 1 {
		return assignModule{}, false
	}
	name, ok := first.Targets[0].(*ast.Name)
	if !ok {
		return assignModule{}, false
	}
	if n := len(name.Id); n < 1 || n > 15 {
		return assignModule{}, false
	}
	c, ok := first.Value.(*ast.Constant)
	if !ok {
		return assignModule{}, false
	}
	val, ok := constantToValue(c)
	if !ok {
		return assignModule{}, false
	}
	if sv, isStr := val.(string); isStr {
		// Mirror the classifier: reject embedded newlines and
		// non-ASCII / backslash bytes.
		for i := 0; i < len(sv); i++ {
			if sv[i] == '\n' {
				return assignModule{}, false
			}
		}
		if !isPlainAsciiNoEscape(sv) {
			return assignModule{}, false
		}
	}
	if c.P.Col > 255 {
		return assignModule{}, false
	}
	lines := splitLines(source)
	line := first.P.Line
	if line < 1 {
		return assignModule{}, false
	}
	ec, ok := lineEndCol(lines, line)
	if !ok {
		return assignModule{}, false
	}
	tail := make([]noOpStmt, 0, len(mod.Body)-1)
	for _, stmt := range mod.Body[1:] {
		s, ok := classifyTailNoOp(stmt, lines)
		if !ok {
			return assignModule{}, false
		}
		tail = append(tail, s)
	}
	return assignModule{
		Line:     uint32(line),
		Name:     name.Id,
		NameLen:  uint16(len(name.Id)),
		Value:    val,
		ValStart: uint16(c.P.Col),
		ValEnd:   uint16(ec),
		Tail:     tail,
	}, true
}

// classifyTailNoOp recognizes one of the no-op tail statements the
// modAssign / modDocstring / modNoOps shapes accept (Pass or a
// non-string Constant ExprStmt). Returns its Loc on success.
func classifyTailNoOp(stmt ast.Stmt, lines [][]byte) (noOpStmt, bool) {
	var line int
	switch s := stmt.(type) {
	case *ast.Pass:
		line = s.P.Line
	case *ast.ExprStmt:
		c, ok := s.Value.(*ast.Constant)
		if !ok {
			return noOpStmt{}, false
		}
		switch c.Kind {
		case "None", "True", "False", "Ellipsis",
			"int", "float", "complex", "bytes":
			line = s.P.Line
		default:
			return noOpStmt{}, false
		}
	default:
		return noOpStmt{}, false
	}
	ec, ok := lineEndCol(lines, line)
	if !ok {
		return noOpStmt{}, false
	}
	return noOpStmt{
		Line:    uint32(line),
		EndLine: uint32(line),
		EndCol:  uint16(ec),
	}, true
}

// constantToValue mirrors compiler/classify_ast.go's predicate of
// the same name for the kinds codegen owns: returns the Go-typed
// value of an *ast.Constant or (nil, false) for unsupported kinds.
func constantToValue(c *ast.Constant) (any, bool) {
	switch c.Kind {
	case "None":
		return nil, true
	case "True":
		return true, true
	case "False":
		return false, true
	case "int":
		v, ok := c.Value.(int64)
		return v, ok
	case "float":
		v, ok := c.Value.(float64)
		return v, ok
	case "complex":
		v, ok := c.Value.(complex128)
		return v, ok
	case "str":
		v, ok := c.Value.(string)
		return v, ok
	case "bytes":
		v, ok := c.Value.([]byte)
		return v, ok
	case "Ellipsis":
		return bytecode.Ellipsis, true
	}
	return nil, false
}

// buildAssignModule emits the bytecode CPython generates for
// `<name> = <const>` followed by N no-op tail statements.
//
// Layout:
//
//	RESUME 0           ; synthetic prologue
//	LOAD_CONST 0       ; (or LOAD_SMALL_INT <val> for int64 in 0..255)
//	                  ; Loc on the value's column range
//	STORE_NAME 0       ; Loc on (col 0, NameLen)
//	[NOPs for tail[0..N-2]]
//	LOAD_CONST <noneIdx>  ; Loc on:
//	                  ;   no tail  → name Loc (so the encoder merges
//	                  ;              STORE_NAME + LOAD_CONST None +
//	                  ;              RETURN_VALUE into one SHORT0 entry)
//	                  ;   else    → tail[N-1] Loc
//	RETURN_VALUE
//
// co_consts = [nil] when the assigned value is None (both LOAD_CONSTs
// reuse index 0); else [value, nil] (LOAD_CONST 0 for the value,
// LOAD_CONST 1 for the trailing None).
//
// co_names = [name].
func buildAssignModule(a assignModule, opts Options) (*bytecode.CodeObject, error) {
	if a.NameLen == 0 || a.NameLen > 15 {
		return nil, errors.New("codegen.buildAssignModule: NameLen out of SHORT0 range")
	}
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	valLoc := bytecode.Loc{
		Line: a.Line, EndLine: a.Line,
		Col: uint16(a.ValStart), EndCol: uint16(a.ValEnd),
	}
	nameLoc := bytecode.Loc{
		Line: a.Line, EndLine: a.Line,
		Col: 0, EndCol: uint16(a.NameLen),
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
	)

	// Value load: LOAD_SMALL_INT for int64 in 0..255, else LOAD_CONST 0.
	if iv, ok := a.Value.(int64); ok && iv >= 0 && iv <= 255 {
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: uint32(iv), Loc: valLoc},
		)
	} else {
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: valLoc},
		)
	}
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 0, Loc: nameLoc},
	)

	// Tail NOPs (all but the last tail stmt).
	lastLoc := nameLoc
	if len(a.Tail) > 0 {
		for _, s := range a.Tail[:len(a.Tail)-1] {
			block.Instrs = append(block.Instrs, ir.Instr{
				Op:  bytecode.NOP,
				Arg: 0,
				Loc: locOf(s),
			})
		}
		lastLoc = locOf(a.Tail[len(a.Tail)-1])
	}

	// Trailing pair: LOAD_CONST <noneIdx>, RETURN_VALUE.
	consts := []any{a.Value, nil}
	noneIdx := uint32(1)
	if a.Value == nil {
		consts = []any{nil}
		noneIdx = 0
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
		Names:    []string{a.Name},
	})
}
