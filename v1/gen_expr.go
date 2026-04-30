package compiler

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ast"
)

// compileGenExpr lowers a general expression assignment `x = <expr>` at
// module scope. The expression must be recursively composed of Name,
// small-int Constant (0-255), BinOp, and UnaryOp (USub/Invert) nodes.
func compileGenExpr(filename string, cls classification) (*bytecode.CodeObject, error) {
	g := cls.genExprAsgn
	gs := newGenState(g.line, g.srcLines)

	startCol, _, depth := gs.walk(g.expr)
	_ = startCol

	// STORE_NAME target + LOAD_CONST None + RETURN_VALUE: SHORT0(3)[0,targetLen)
	targetIdx := gs.nameIdx(g.targetName)
	noneIdx := gs.noneIdx()
	gs.bc = append(gs.bc,
		byte(bytecode.STORE_NAME), byte(targetIdx),
		byte(bytecode.LOAD_CONST), byte(noneIdx),
		byte(bytecode.RETURN_VALUE), 0,
	)
	gs.lt = bytecode.GenExprSameLine(gs.lt, 3, 0, g.targetLen)

	// Prepend RESUME 0 + prologue linetable entry.
	bc := make([]byte, 0, 2+len(gs.bc))
	bc = append(bc, byte(bytecode.RESUME), 0)
	bc = append(bc, gs.bc...)

	lt := make([]byte, 0, 5+len(gs.lt))
	lt = append(lt, bytecode.GenExprProlog()...)
	lt = append(lt, gs.lt...)

	co := module(filename, bc, lt, gs.buildConsts(), gs.names)
	if depth > 1 {
		co.StackSize = int32(depth)
	}
	return co, nil
}

// genState accumulates bytecode, linetable entries, names, and
// constants for a general expression compilation.
type genState struct {
	// assignment source line (1-indexed)
	assignLine int
	srcLines   [][]byte

	bc []byte
	lt []byte

	// name table (insertion-ordered, first occurrence wins)
	names    []string
	namesMap map[string]byte

	// constant table
	consts         []any
	constsMap      map[any]int // non-small-int values → co_consts index
	firstConstDone bool        // true after first constant added to consts

	// linetable state: true until first real instruction emitted
	firstDone bool
}

func newGenState(assignLine int, srcLines [][]byte) *genState {
	return &genState{
		assignLine: assignLine,
		srcLines:   srcLines,
		namesMap:   map[string]byte{},
		constsMap:  map[any]int{},
	}
}

// walk compiles expr, returning (startCol, endCol, maxStackDepth).
func (gs *genState) walk(e ast.Expr) (startCol, endCol byte, depth int) {
	switch n := e.(type) {
	case *ast.Name:
		idx := gs.nameIdx(n.Id)
		sc := byte(n.P.Col)
		ec := sc + byte(len(n.Id))
		gs.bc = append(gs.bc, byte(bytecode.LOAD_NAME), idx)
		gs.emitLoad(1, sc, ec)
		return sc, ec, 1

	case *ast.Constant:
		iv := n.Value.(int64)
		sc := byte(n.P.Col)
		ec := gs.constEndCol(n)
		gs.addFirstConst(iv)
		gs.bc = append(gs.bc, byte(bytecode.LOAD_SMALL_INT), byte(iv))
		gs.emitLoad(1, sc, ec)
		return sc, ec, 1

	case *ast.BinOp:
		lsc, _, ld := gs.walk(n.Left)
		_, rec, rd := gs.walk(n.Right)
		oparg, _ := binOpargFromOp(n.Op)
		cacheWords := int(bytecode.CacheSize[bytecode.BINARY_OP])
		cuCount := 1 + cacheWords
		gs.emitBinOp(oparg, cacheWords, cuCount, lsc, rec)
		d := max(ld, rd+1)
		return lsc, rec, d

	case *ast.UnaryOp:
		_, oec, od := gs.walk(n.Operand)
		opc := byte(n.P.Col)
		switch n.Op {
		case "USub":
			gs.bc = append(gs.bc, byte(bytecode.UNARY_NEGATIVE), 0)
		case "Invert":
			gs.bc = append(gs.bc, byte(bytecode.UNARY_INVERT), 0)
		}
		gs.emitSame(1, opc, oec)
		return opc, oec, od
	}
	return 0, 0, 1
}

// emitLoad appends one linetable entry for a LOAD instruction.
// The first call uses ONE_LINE1 (delta+1 from synthetic line 0); all
// subsequent calls on the same assignment line use SHORTn.
func (gs *genState) emitLoad(cuCount int, sc, ec byte) {
	if !gs.firstDone {
		gs.lt = bytecode.GenExprFirstEntry(gs.lt, 1, cuCount, sc, ec)
		gs.firstDone = true
	} else {
		gs.lt = bytecode.GenExprSameLine(gs.lt, cuCount, sc, ec)
	}
}

// emitSame appends one linetable entry for an instruction on the same
// line as the previous entry (delta=0).
func (gs *genState) emitSame(cuCount int, sc, ec byte) {
	gs.lt = bytecode.GenExprSameLine(gs.lt, cuCount, sc, ec)
}

// emitBinOp appends BINARY_OP + cache bytes to bc and one linetable entry.
func (gs *genState) emitBinOp(oparg byte, cacheWords, cuCount int, sc, ec byte) {
	gs.bc = append(gs.bc, byte(bytecode.BINARY_OP), oparg)
	for range cacheWords {
		gs.bc = append(gs.bc, 0, 0)
	}
	gs.emitSame(cuCount, sc, ec)
}

// nameIdx returns the co_names index for name, adding it if new.
func (gs *genState) nameIdx(name string) byte {
	if idx, ok := gs.namesMap[name]; ok {
		return idx
	}
	idx := byte(len(gs.names))
	gs.names = append(gs.names, name)
	gs.namesMap[name] = idx
	return idx
}

// addFirstConst adds a small-int constant to co_consts[0] if and only
// if it is the first constant encountered in the expression.
func (gs *genState) addFirstConst(iv int64) {
	if gs.firstConstDone {
		return
	}
	gs.consts = append(gs.consts, iv)
	gs.firstConstDone = true
}

// noneIdx returns the co_consts index for None, adding it if needed.
func (gs *genState) noneIdx() byte {
	if idx, ok := gs.constsMap[nil]; ok {
		return byte(idx)
	}
	idx := len(gs.consts)
	gs.consts = append(gs.consts, nil)
	gs.constsMap[nil] = idx
	return byte(idx)
}

// buildConsts returns the final co_consts slice (consts already has None appended).
func (gs *genState) buildConsts() []any {
	return gs.consts
}

// constEndCol returns the column after the last character of the
// constant token starting at n.P.Col on the assignment line.
func (gs *genState) constEndCol(n *ast.Constant) byte {
	line := n.P.Line
	if line < 1 || line > len(gs.srcLines) {
		return byte(n.P.Col) + 1
	}
	src := gs.srcLines[line-1]
	col := n.P.Col
	if col >= len(src) {
		return byte(col) + 1
	}
	i := col
	for i < len(src) {
		c := src[i]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '.' {
			i++
		} else {
			break
		}
	}
	return byte(i)
}
