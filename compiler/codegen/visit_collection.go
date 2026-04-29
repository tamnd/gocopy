package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// collectionModule captures the modCollection shape: a single
// `<target> = <List|Tuple|Set|Dict>` statement where target is a
// Name of 1..15 ASCII chars and every element (or key/value pair
// for Dict) is a Name on the same source line as the assignment.
// Empty collections (`[]`, `()`, `{}` for dict) are also covered.
type collectionModule struct {
	Line      uint32
	Target    string
	TargetLen uint16
	OpenCol   uint16
	CloseEnd  uint16
	Kind      bytecode.CollKind
	Elts      []bytecode.CollElt
}

// classifyCollectionModule recognizes a single-statement
// `<target> = <collection>` body. To stay byte-identical with the
// classifier's compileCollection (which has no slot for trailing
// no-ops), the codegen path accepts only single-statement modules.
func classifyCollectionModule(mod *ast.Module, src []byte) (collectionModule, bool) {
	if len(mod.Body) != 1 {
		return collectionModule{}, false
	}
	a, ok := mod.Body[0].(*ast.Assign)
	if !ok {
		return collectionModule{}, false
	}
	if len(a.Targets) != 1 {
		return collectionModule{}, false
	}
	target, ok := a.Targets[0].(*ast.Name)
	if !ok {
		return collectionModule{}, false
	}
	if n := len(target.Id); n < 1 || n > 15 {
		return collectionModule{}, false
	}
	line := a.P.Line
	if line < 1 {
		return collectionModule{}, false
	}

	var kind bytecode.CollKind
	var openCol int
	var eltsExprs []ast.Expr
	switch v := a.Value.(type) {
	case *ast.List:
		kind = bytecode.CollList
		openCol = v.P.Col
		eltsExprs = v.Elts
	case *ast.Tuple:
		kind = bytecode.CollTuple
		openCol = v.P.Col
		eltsExprs = v.Elts
	case *ast.Set:
		kind = bytecode.CollSet
		openCol = v.P.Col
		eltsExprs = v.Elts
	case *ast.Dict:
		if len(v.Keys) != len(v.Values) {
			return collectionModule{}, false
		}
		kind = bytecode.CollDict
		openCol = v.P.Col
		flat := make([]ast.Expr, 0, 2*len(v.Keys))
		for i, k := range v.Keys {
			if k == nil {
				return collectionModule{}, false // **other unpacking
			}
			flat = append(flat, k, v.Values[i])
		}
		eltsExprs = flat
	default:
		return collectionModule{}, false
	}
	if openCol > 255 {
		return collectionModule{}, false
	}

	elts := make([]bytecode.CollElt, 0, len(eltsExprs))
	for _, e := range eltsExprs {
		n, isName := e.(*ast.Name)
		if !isName {
			return collectionModule{}, false
		}
		if n.P.Line != line || n.P.Col > 255 || len(n.Id) > 15 {
			return collectionModule{}, false
		}
		elts = append(elts, bytecode.CollElt{
			Name:    n.Id,
			Col:     byte(n.P.Col),
			NameLen: byte(len(n.Id)),
		})
	}

	lines := splitLines(src)
	closeEnd, ok := lineEndCol(lines, line)
	if !ok {
		return collectionModule{}, false
	}

	return collectionModule{
		Line:      uint32(line),
		Target:    target.Id,
		TargetLen: uint16(len(target.Id)),
		OpenCol:   uint16(openCol),
		CloseEnd:  uint16(closeEnd),
		Kind:      kind,
		Elts:      elts,
	}, true
}

// buildCollectionModule emits the bytecode CPython 3.14 generates
// for a single `<target> = <collection>` assignment at module
// scope. Mirrors `bytecode.CollectionEmptyBytecode` /
// `bytecode.CollectionNamesBytecode` and their line-table siblings
// byte-for-byte.
func buildCollectionModule(c collectionModule, opts Options) (*bytecode.CodeObject, error) {
	if c.TargetLen == 0 || c.TargetLen > 15 {
		return nil, errors.New("codegen.buildCollectionModule: target name length out of SHORT0 range")
	}

	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	buildLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: c.OpenCol, EndCol: c.CloseEnd,
	}
	targetLoc := bytecode.Loc{
		Line: c.Line, EndLine: c.Line,
		Col: 0, EndCol: c.TargetLen,
	}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
	)

	n := uint32(len(c.Elts))
	storeNameArg := uint32(0)
	if n > 0 {
		for i, e := range c.Elts {
			eltLoc := bytecode.Loc{
				Line: c.Line, EndLine: c.Line,
				Col: uint16(e.Col), EndCol: uint16(e.Col) + uint16(e.NameLen),
			}
			block.Instrs = append(block.Instrs,
				ir.Instr{Op: bytecode.LOAD_NAME, Arg: uint32(i), Loc: eltLoc},
			)
		}
		buildOp, buildArg, err := buildOpFor(c.Kind, n)
		if err != nil {
			return nil, err
		}
		block.Instrs = append(block.Instrs,
			ir.Instr{Op: buildOp, Arg: buildArg, Loc: buildLoc},
		)
		storeNameArg = n
	} else {
		// Empty: BUILD_LIST 0 / BUILD_MAP 0 / LOAD_CONST 1 (() sentinel).
		switch c.Kind {
		case bytecode.CollList:
			block.Instrs = append(block.Instrs,
				ir.Instr{Op: bytecode.BUILD_LIST, Arg: 0, Loc: buildLoc},
			)
		case bytecode.CollDict:
			block.Instrs = append(block.Instrs,
				ir.Instr{Op: bytecode.BUILD_MAP, Arg: 0, Loc: buildLoc},
			)
		case bytecode.CollTuple:
			block.Instrs = append(block.Instrs,
				ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1, Loc: buildLoc},
			)
		case bytecode.CollSet:
			// Empty set has no literal form (`{}` is dict). Reject.
			return nil, errors.New("codegen.buildCollectionModule: empty set has no literal form")
		default:
			return nil, errors.New("codegen.buildCollectionModule: unknown collection kind")
		}
	}

	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.STORE_NAME, Arg: storeNameArg, Loc: targetLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: targetLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: targetLoc},
	)

	consts := []any{nil}
	if n == 0 && c.Kind == bytecode.CollTuple {
		consts = []any{nil, bytecode.ConstTuple{}}
	}

	names := make([]string, 0, len(c.Elts)+1)
	for _, e := range c.Elts {
		names = append(names, e.Name)
	}
	names = append(names, c.Target)

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
	if n > 1 && co.StackSize < int32(n) {
		co.StackSize = int32(n)
	}
	return co, nil
}

// buildOpFor returns the BUILD_* opcode and oparg for a non-empty
// collection of the given kind with N flat elements (key/value
// pairs are flattened, so dict oparg = N/2).
func buildOpFor(kind bytecode.CollKind, n uint32) (bytecode.Opcode, uint32, error) {
	switch kind {
	case bytecode.CollList:
		return bytecode.BUILD_LIST, n, nil
	case bytecode.CollTuple:
		return bytecode.BUILD_TUPLE, n, nil
	case bytecode.CollSet:
		return bytecode.BUILD_SET, n, nil
	case bytecode.CollDict:
		return bytecode.BUILD_MAP, n / 2, nil
	}
	return 0, 0, errors.New("codegen.buildOpFor: unknown collection kind")
}
