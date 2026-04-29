package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// genExprModule captures the modGenExpr shape: a single
// `<target> = <expr>` statement where target is a 1..15-char
// Name and expr is recursively composed of Name, small-int
// Constant (0..255), BinOp (any of the 13 supported operators),
// or UnaryOp (USub or Invert) — all on the same source line.
type genExprModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	Expr      ast.Expr
	SrcLines  [][]byte
}

// classifyGenExprModule recognises a single-statement
// `<target> = <recursive expr>` body. Stays byte-identical with
// the classifier's compileGenExpr — single-statement only.
func classifyGenExprModule(mod *ast.Module, src []byte) (genExprModule, bool) {
	if len(mod.Body) != 1 {
		return genExprModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return genExprModule{}, false
	}
	if len(a.Targets) != 1 {
		return genExprModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return genExprModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return genExprModule{}, false
	}
	if target.P.Col > 255 {
		return genExprModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return genExprModule{}, false
	}
	// Mirror the classifier's "extractValue first" rule: bare Constants,
	// UnaryOp(USub, numeric Constant), and BinOp(numeric Constant, op,
	// numeric Constant) all collapse to modAssign in the classifier path.
	// modGenExpr only owns truly compound shapes that survive folding.
	if classifierWouldFold(a.Value) {
		return genExprModule{}, false
	}
	if !isGenExprNode(a.Value, line) {
		return genExprModule{}, false
	}
	return genExprModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		Expr:      a.Value,
		SrcLines:  splitLines(src),
	}, true
}

// classifierWouldFold reports whether the classifier path's
// extractValue would accept e and route it to modAssign. modGenExpr
// must reject those so the modAssign codegen / classifier path keeps
// owning negative literals and folded constant BinOps.
func classifierWouldFold(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Constant:
		return true
	case *ast.UnaryOp:
		if v.Op != "USub" {
			return false
		}
		c, isConst := v.Operand.(*ast.Constant)
		if !isConst {
			return false
		}
		switch c.Kind {
		case "int":
			iv, ok := c.Value.(int64)
			return ok && iv != 0
		case "float":
			return true
		}
		return false
	case *ast.BinOp:
		left, leftOK := v.Left.(*ast.Constant)
		right, rightOK := v.Right.(*ast.Constant)
		if !leftOK || !rightOK {
			return false
		}
		return numericConstKind(left) && numericConstKind(right)
	}
	return false
}

func numericConstKind(c *ast.Constant) bool {
	return c.Kind == "int" || c.Kind == "float"
}

// isGenExprNode mirrors compiler/classify_ast.go::isGenExpr but
// also enforces that every position lives on the assignment's
// source line.
func isGenExprNode(e ast.Expr, line int) bool {
	switch n := e.(type) {
	case *ast.Name:
		return len(n.Id) >= 1 && len(n.Id) <= 15 && n.P.Col <= 255 && n.P.Line == line
	case *ast.Constant:
		if n.P.Col > 255 || n.P.Line != line {
			return false
		}
		if n.Kind != "int" {
			return false
		}
		iv, ok := n.Value.(int64)
		return ok && iv >= 0 && iv <= 255
	case *ast.BinOp:
		if _, ok := binOpargFromOp(n.Op); !ok {
			return false
		}
		return isGenExprNode(n.Left, line) && isGenExprNode(n.Right, line)
	case *ast.UnaryOp:
		if n.Op != "USub" && n.Op != "Invert" {
			return false
		}
		if n.P.Col > 255 || n.P.Line != line {
			return false
		}
		return isGenExprNode(n.Operand, line)
	}
	return false
}

// genWalker accumulates IR instructions, names, and constants for
// a recursive walk of a genExpr. Mirrors compiler/gen_expr.go's
// genState byte-for-byte.
type genWalker struct {
	line     uint32
	srcLines [][]byte
	instrs   []ir.Instr

	names    []string
	namesMap map[string]uint32

	consts         []any
	firstConstDone bool
}

func newGenWalker(line uint32, srcLines [][]byte) *genWalker {
	return &genWalker{
		line:     line,
		srcLines: srcLines,
		namesMap: map[string]uint32{},
	}
}

// walk emits IR for e and returns (startCol, endCol, depth).
func (w *genWalker) walk(e ast.Expr) (uint16, uint16, int) {
	switch n := e.(type) {
	case *ast.Name:
		idx := w.nameIdx(n.Id)
		sc := uint16(n.P.Col)
		ec := sc + uint16(len(n.Id))
		w.instrs = append(w.instrs, ir.Instr{
			Op: bytecode.LOAD_NAME, Arg: idx,
			Loc: bytecode.Loc{Line: w.line, EndLine: w.line, Col: sc, EndCol: ec},
		})
		return sc, ec, 1

	case *ast.Constant:
		iv := n.Value.(int64)
		sc := uint16(n.P.Col)
		ec := w.constEndCol(n)
		w.addFirstConst(iv)
		w.instrs = append(w.instrs, ir.Instr{
			Op: bytecode.LOAD_SMALL_INT, Arg: uint32(iv),
			Loc: bytecode.Loc{Line: w.line, EndLine: w.line, Col: sc, EndCol: ec},
		})
		return sc, ec, 1

	case *ast.BinOp:
		lsc, _, ld := w.walk(n.Left)
		_, rec, rd := w.walk(n.Right)
		oparg, _ := binOpargFromOp(n.Op)
		w.instrs = append(w.instrs, ir.Instr{
			Op: bytecode.BINARY_OP, Arg: uint32(oparg),
			Loc: bytecode.Loc{Line: w.line, EndLine: w.line, Col: lsc, EndCol: rec},
		})
		return lsc, rec, max(ld, rd+1)

	case *ast.UnaryOp:
		_, oec, od := w.walk(n.Operand)
		opc := uint16(n.P.Col)
		var op bytecode.Opcode
		switch n.Op {
		case "USub":
			op = bytecode.UNARY_NEGATIVE
		case "Invert":
			op = bytecode.UNARY_INVERT
		}
		w.instrs = append(w.instrs, ir.Instr{
			Op: op, Arg: 0,
			Loc: bytecode.Loc{Line: w.line, EndLine: w.line, Col: opc, EndCol: oec},
		})
		return opc, oec, od
	}
	return 0, 0, 1
}

// nameIdx returns the co_names index for name, adding it on first
// occurrence (insertion-order, first-occurrence wins).
func (w *genWalker) nameIdx(name string) uint32 {
	if idx, ok := w.namesMap[name]; ok {
		return idx
	}
	idx := uint32(len(w.names))
	w.names = append(w.names, name)
	w.namesMap[name] = idx
	return idx
}

// addFirstConst appends iv to co_consts only on the first call.
// CPython's compiler tracks the first int constant it encounters
// and emits it in co_consts; subsequent ints reach LOAD_SMALL_INT
// directly via oparg and never enter co_consts.
func (w *genWalker) addFirstConst(iv int64) {
	if w.firstConstDone {
		return
	}
	w.consts = append(w.consts, iv)
	w.firstConstDone = true
}

// constEndCol returns the column after the last character of the
// constant token starting at n.P.Col on the source line. Mirrors
// compiler/gen_expr.go::genState.constEndCol.
func (w *genWalker) constEndCol(n *ast.Constant) uint16 {
	line := n.P.Line
	if line < 1 || line > len(w.srcLines) {
		return uint16(n.P.Col) + 1
	}
	src := w.srcLines[line-1]
	col := n.P.Col
	if col >= len(src) {
		return uint16(col) + 1
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
	return uint16(i)
}

// buildGenExprModule emits the bytecode CPython 3.14 generates for
// `<target> = <recursive genExpr>` at module scope. Mirrors
// compiler/gen_expr.go::compileGenExpr byte-for-byte.
//
// Layout:
//
//	RESUME 0                              ; synthetic
//	<walker output>                       ; LOAD_NAME / LOAD_SMALL_INT /
//	                                      ; BINARY_OP+5cache / UNARY_*
//	STORE_NAME nameIdx(target)
//	LOAD_CONST noneIdx
//	RETURN_VALUE 0
//
// co_consts = [firstInt, nil] if any int constant was encountered,
// else [nil].
// co_names  = [insertion-order names... target] (first-occurrence
// wins; target is appended only if not already present).
// co.StackSize = max(1, walkerDepth). Trivial single-name and
// single-name UnaryOp shapes need only 1 slot; the lower bound was
// 2 while the dedicated unary_assign / binop_assign classifiers
// owned the simple cases. With those classifiers gone (v0.7.2) the
// floor drops to 1 so single-name UnaryOp matches the v0.6 byte
// output, and assemble.Assemble's flow analysis still bumps it for
// real BinOp/Unary chains.
func buildGenExprModule(g genExprModule, opts Options) (*bytecode.CodeObject, error) {
	if g.TargetLen == 0 || g.TargetLen > 15 {
		return nil, errors.New("codegen.buildGenExprModule: target name length out of SHORT0 range")
	}

	w := newGenWalker(g.Line, g.SrcLines)
	_, _, depth := w.walk(g.Expr)

	targetIdx := w.nameIdx(g.Target)
	noneIdx := uint32(len(w.consts))
	w.consts = append(w.consts, nil)

	targetLoc := bytecode.Loc{
		Line: g.Line, EndLine: g.Line,
		Col: 0, EndCol: g.TargetLen,
	}
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
	)
	block.Instrs = append(block.Instrs, w.instrs...)
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.STORE_NAME, Arg: targetIdx, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	co, err := assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   w.consts,
		Names:    w.names,
	})
	if err != nil {
		return nil, err
	}
	want := max(int32(1), int32(depth))
	if co.StackSize < want {
		co.StackSize = want
	}
	return co, nil
}
