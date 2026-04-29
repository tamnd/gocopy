package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// chainedAssignModule captures a module body whose first
// statement is a chained `t0 = t1 = ... = tN-1 = <const>` with
// N >= 2 Name targets and a non-folded simple-constant value,
// optionally followed by no-op statements.
type chainedAssignModule struct {
	Line     uint32
	Targets  []chainedTarget
	Value    any
	ValStart uint16
	ValEnd   uint16
	Tail     []noOpStmt
}

type chainedTarget struct {
	Name      string
	NameStart uint16
	NameLen   uint16
}

// classifyChainedAssignModule recognizes the simple-constant
// chained-assignment shape (>= 2 Name targets sharing one value).
func classifyChainedAssignModule(mod *ast.Module, source []byte) (chainedAssignModule, bool) {
	if len(mod.Body) == 0 {
		return chainedAssignModule{}, false
	}
	first, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return chainedAssignModule{}, false
	}
	if len(first.Targets) < 2 {
		return chainedAssignModule{}, false
	}
	targets := make([]chainedTarget, len(first.Targets))
	for i, t := range first.Targets {
		name, ok := t.(*ast.Name)
		if !ok {
			return chainedAssignModule{}, false
		}
		if n := len(name.Id); n < 1 || n > 15 {
			return chainedAssignModule{}, false
		}
		if name.P.Col > 255 {
			return chainedAssignModule{}, false
		}
		targets[i] = chainedTarget{
			Name:      name.Id,
			NameStart: uint16(name.P.Col),
			NameLen:   uint16(len(name.Id)),
		}
	}
	c, ok := first.Value.(*ast.Constant)
	if !ok {
		return chainedAssignModule{}, false
	}
	val, ok := constantToValue(c)
	if !ok {
		return chainedAssignModule{}, false
	}
	if sv, isStr := val.(string); isStr {
		for i := 0; i < len(sv); i++ {
			if sv[i] == '\n' {
				return chainedAssignModule{}, false
			}
		}
		if !isPlainAsciiNoEscape(sv) {
			return chainedAssignModule{}, false
		}
	}
	if c.P.Col > 255 {
		return chainedAssignModule{}, false
	}
	line := first.P.Line
	if line < 1 {
		return chainedAssignModule{}, false
	}
	lines := splitLines(source)
	ec, ok := lineEndCol(lines, line)
	if !ok {
		return chainedAssignModule{}, false
	}
	tail := make([]noOpStmt, 0, len(mod.Body)-1)
	for _, stmt := range mod.Body[1:] {
		s, ok := classifyTailNoOp(stmt, lines)
		if !ok {
			return chainedAssignModule{}, false
		}
		tail = append(tail, s)
	}
	return chainedAssignModule{
		Line:     uint32(line),
		Targets:  targets,
		Value:    val,
		ValStart: uint16(c.P.Col),
		ValEnd:   uint16(ec),
		Tail:     tail,
	}, true
}

// buildChainedAssignModule emits the bytecode CPython generates
// for `t0 = t1 = ... = tN-1 = <const>` followed by M no-op tail
// statements.
//
// Layout:
//
//	RESUME 0
//	LOAD_SMALL_INT <val> | LOAD_CONST <idx>
//	[for each target i in 0..N-2:
//	  COPY 1
//	  STORE_NAME <nameIdx[i]>]
//	STORE_NAME <nameIdx[N-1]>
//	[NOPs for tail[0..M-2] if M >= 2]
//	LOAD_CONST <noneIdx>
//	RETURN_VALUE
//
// co.StackSize = 2 (COPY pushes a second item).
func buildChainedAssignModule(c chainedAssignModule, opts Options) (*bytecode.CodeObject, error) {
	n := len(c.Targets)
	if n < 2 {
		return nil, errors.New("codegen.buildChainedAssignModule: need >=2 targets")
	}

	// Build dedup'd names with insertion order.
	namesIdx := map[string]uint32{}
	var names []string
	nameIdxs := make([]uint32, n)
	for i, t := range c.Targets {
		if idx, ok := namesIdx[t.Name]; ok {
			nameIdxs[i] = idx
		} else {
			idx := uint32(len(names))
			namesIdx[t.Name] = idx
			names = append(names, t.Name)
			nameIdxs[i] = idx
		}
	}

	// Build co_consts mirroring the classifier (single-value rules).
	var consts []any
	var noneIdx uint32
	var loadOp bytecode.Opcode
	var loadArg uint32

	switch tv := c.Value.(type) {
	case int64:
		if tv >= 0 && tv <= 255 {
			consts = []any{tv, nil}
			noneIdx = 1
			loadOp = bytecode.LOAD_SMALL_INT
			loadArg = uint32(tv)
		} else {
			consts = []any{tv, nil}
			noneIdx = 1
			loadOp = bytecode.LOAD_CONST
			loadArg = 0
		}
	case nil:
		consts = []any{nil}
		noneIdx = 0
		loadOp = bytecode.LOAD_CONST
		loadArg = 0
	default:
		consts = []any{c.Value, nil}
		noneIdx = 1
		loadOp = bytecode.LOAD_CONST
		loadArg = 0
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	valLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: c.ValStart, EndCol: c.ValEnd,
	}
	copyLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: 0, EndCol: c.ValEnd,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: loadOp, Arg: loadArg, Loc: valLoc},
	)

	var lastTargetLoc bytecode.Loc
	for i, t := range c.Targets {
		tgtLoc := bytecode.Loc{
			Line: c.Line, EndLine: c.Line,
			Col: t.NameStart, EndCol: t.NameStart + t.NameLen,
		}
		if i < n-1 {
			block.Instrs = append(block.Instrs,
				ir.Instr{Op: bytecode.COPY, Arg: 1, Loc: copyLoc},
				ir.Instr{Op: bytecode.STORE_NAME, Arg: nameIdxs[i], Loc: tgtLoc},
			)
		} else {
			block.Instrs = append(block.Instrs,
				ir.Instr{Op: bytecode.STORE_NAME, Arg: nameIdxs[i], Loc: tgtLoc},
			)
			lastTargetLoc = tgtLoc
		}
	}

	// Tail NOPs (all but the last tail stmt).
	lastLoc := lastTargetLoc
	if len(c.Tail) > 0 {
		for _, s := range c.Tail[:len(c.Tail)-1] {
			block.Instrs = append(block.Instrs, ir.Instr{
				Op: bytecode.NOP, Arg: 0, Loc: locOf(s),
			})
		}
		lastLoc = locOf(c.Tail[len(c.Tail)-1])
	}

	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: lastLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: lastLoc},
	)

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
