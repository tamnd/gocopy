package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// augAssignModule captures the modAugAssign shape: a basic
// `<name> = <initInt>` followed by `<name> <op>= <augInt>`
// where both values are non-negative ints, optionally followed
// by no-op statements.
type augAssignModule struct {
	InitLine     uint32
	Name         string
	NameLen      uint16
	InitVal      int64
	InitValStart uint16
	InitValEnd   uint16
	AugLine      uint32
	AugVal       int64
	AugValStart  uint16
	AugValEnd    uint16
	AugOparg     uint32
	Tail         []noOpStmt
}

// classifyAugAssignModule recognizes the init+augmented-assign
// shape.
func classifyAugAssignModule(mod *ast.Module, source []byte) (augAssignModule, bool) {
	if len(mod.Body) < 2 {
		return augAssignModule{}, false
	}
	first, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return augAssignModule{}, false
	}
	if len(first.Targets) != 1 {
		return augAssignModule{}, false
	}
	initName, ok := first.Targets[0].(*ast.Name)
	if !ok {
		return augAssignModule{}, false
	}
	if n := len(initName.Id); n < 1 || n > 15 {
		return augAssignModule{}, false
	}
	initC, ok := first.Value.(*ast.Constant)
	if !ok || initC.Kind != "int" {
		return augAssignModule{}, false
	}
	initVal, ok := initC.Value.(int64)
	if !ok || initVal < 0 {
		return augAssignModule{}, false
	}
	if initC.P.Col > 255 {
		return augAssignModule{}, false
	}

	aug, ok := mod.Body[1].(*ast.AugAssign)
	if !ok {
		return augAssignModule{}, false
	}
	augName, ok := aug.Target.(*ast.Name)
	if !ok || augName.Id != initName.Id {
		return augAssignModule{}, false
	}
	augC, ok := aug.Value.(*ast.Constant)
	if !ok || augC.Kind != "int" {
		return augAssignModule{}, false
	}
	augVal, ok := augC.Value.(int64)
	if !ok || augVal < 0 {
		return augAssignModule{}, false
	}
	if augC.P.Col > 255 {
		return augAssignModule{}, false
	}
	oparg, ok := augOpargFromOp(aug.Op)
	if !ok {
		return augAssignModule{}, false
	}

	lines := splitLines(source)
	initLine := first.P.Line
	augLine := aug.P.Line
	if initLine < 1 || augLine < 1 {
		return augAssignModule{}, false
	}
	initEnd, ok := lineEndCol(lines, initLine)
	if !ok {
		return augAssignModule{}, false
	}
	augEnd, ok := lineEndCol(lines, augLine)
	if !ok {
		return augAssignModule{}, false
	}

	tail := make([]noOpStmt, 0, len(mod.Body)-2)
	for _, stmt := range mod.Body[2:] {
		s, ok := classifyTailNoOp(stmt, lines)
		if !ok {
			return augAssignModule{}, false
		}
		tail = append(tail, s)
	}

	return augAssignModule{
		InitLine:     uint32(initLine),
		Name:         initName.Id,
		NameLen:      uint16(len(initName.Id)),
		InitVal:      initVal,
		InitValStart: uint16(initC.P.Col),
		InitValEnd:   uint16(initEnd),
		AugLine:      uint32(augLine),
		AugVal:       augVal,
		AugValStart:  uint16(augC.P.Col),
		AugValEnd:    uint16(augEnd),
		AugOparg:     uint32(oparg),
		Tail:         tail,
	}, true
}

// augOpargFromOp maps an *ast.AugAssign Op to its CPython
// NB_INPLACE_* enum value. Mirrors compiler/classify_ast.go's
// helper of the same name.
func augOpargFromOp(op string) (byte, bool) {
	switch op {
	case "Add":
		return bytecode.NbInplaceAdd, true
	case "Sub":
		return bytecode.NbInplaceSubtract, true
	case "Mult":
		return bytecode.NbInplaceMultiply, true
	case "Div":
		return bytecode.NbInplaceTrueDivide, true
	case "FloorDiv":
		return bytecode.NbInplaceFloorDivide, true
	case "Mod":
		return bytecode.NbInplaceRemainder, true
	case "Pow":
		return bytecode.NbInplacePower, true
	case "BitAnd":
		return bytecode.NbInplaceAnd, true
	case "BitOr":
		return bytecode.NbInplaceOr, true
	case "BitXor":
		return bytecode.NbInplaceXor, true
	case "LShift":
		return bytecode.NbInplaceLshift, true
	case "RShift":
		return bytecode.NbInplaceRshift, true
	}
	return 0, false
}

// buildAugAssignModule emits the bytecode CPython generates for
// `<name> = <initInt>` followed by `<name> <op>= <augInt>` and
// M no-op tail statements.
//
// Layout:
//
//	RESUME 0
//	LOAD_SMALL_INT <init> | LOAD_CONST 0
//	STORE_NAME 0
//	LOAD_NAME 0                         ; aug Loc, name col range
//	LOAD_SMALL_INT <aug> | LOAD_CONST 1
//	BINARY_OP <oparg>                   ; encoder appends 5 cache words
//	STORE_NAME 0
//	[NOPs for tail[0..M-2]]
//	LOAD_CONST <noneIdx>
//	RETURN_VALUE
//
// co.StackSize = 2 (LOAD_NAME + LOAD augVal both on stack).
func buildAugAssignModule(a augAssignModule, opts Options) (*bytecode.CodeObject, error) {
	if a.NameLen == 0 || a.NameLen > 15 {
		return nil, errors.New("codegen.buildAugAssignModule: NameLen out of SHORT0 range")
	}

	initSmall := a.InitVal >= 0 && a.InitVal <= 255
	augSmall := a.AugVal >= 0 && a.AugVal <= 255

	var consts []any
	var noneIdx uint32
	if augSmall {
		consts = []any{a.InitVal, nil}
		noneIdx = 1
	} else {
		consts = []any{a.InitVal, a.AugVal, nil}
		noneIdx = 2
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	initValLoc := bytecode.Loc{
		Line: a.InitLine, EndLine: a.InitLine,
		Col: a.InitValStart, EndCol: a.InitValEnd,
	}
	initNameLoc := bytecode.Loc{
		Line: a.InitLine, EndLine: a.InitLine,
		Col: 0, EndCol: a.NameLen,
	}
	augNameLoadLoc := bytecode.Loc{
		Line: a.AugLine, EndLine: a.AugLine,
		Col: 0, EndCol: a.NameLen,
	}
	augValLoc := bytecode.Loc{
		Line: a.AugLine, EndLine: a.AugLine,
		Col: a.AugValStart, EndCol: a.AugValEnd,
	}
	binOpLoc := bytecode.Loc{
		Line: a.AugLine, EndLine: a.AugLine,
		Col: 0, EndCol: a.AugValEnd,
	}
	augStoreLoc := bytecode.Loc{
		Line: a.AugLine, EndLine: a.AugLine,
		Col: 0, EndCol: a.NameLen,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
	)

	if initSmall {
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: uint32(a.InitVal), Loc: initValLoc},
		)
	} else {
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: initValLoc},
		)
	}
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 0, Loc: initNameLoc},
		ir.Instr{Op: bytecode.LOAD_NAME, Arg: 0, Loc: augNameLoadLoc},
	)
	if augSmall {
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_SMALL_INT, Arg: uint32(a.AugVal), Loc: augValLoc},
		)
	} else {
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1, Loc: augValLoc},
		)
	}
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.BINARY_OP, Arg: a.AugOparg, Loc: binOpLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 0, Loc: augStoreLoc},
	)

	// Tail NOPs (all but the last tail stmt).
	lastLoc := augStoreLoc
	if len(a.Tail) > 0 {
		for _, s := range a.Tail[:len(a.Tail)-1] {
			block.Instrs = append(block.Instrs, ir.Instr{
				Op: bytecode.NOP, Arg: 0, Loc: locOf(s),
			})
		}
		lastLoc = locOf(a.Tail[len(a.Tail)-1])
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
		Names:    []string{a.Name},
	})
	if err != nil {
		return nil, err
	}
	if co.StackSize < 2 {
		co.StackSize = 2
	}
	return co, nil
}
