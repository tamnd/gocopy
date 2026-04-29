package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/assemble"
	"github.com/tamnd/gocopy/compiler/ast"
	"github.com/tamnd/gocopy/compiler/ir"
)

// noOpStmt records a no-op module-level statement's source position.
// Line/EndLine are 1-indexed. EndCol is the 0-indexed exclusive end
// column on EndLine, computed from the source text the same way the
// classifier computes it (line trimmed of the trailing comment and
// whitespace).
type noOpStmt struct {
	Line    uint32
	EndLine uint32
	EndCol  uint16
}

// classifyNoOpModule recognizes a module body whose every statement
// is a no-op:
//
//   - *ast.Pass.
//   - *ast.ExprStmt whose value is a non-string *ast.Constant
//     (None, True, False, Ellipsis, int, float, complex, bytes).
//
// Returns the per-statement positions on success, false otherwise.
// The classifier's modNoOps shape accepts non-leading string
// literals as no-ops too; v0.6.7 keeps strings out (the docstring
// shape is the next sub-release) so a bare string statement falls
// through to the classifier.
func classifyNoOpModule(mod *ast.Module, source []byte) ([]noOpStmt, bool) {
	if len(mod.Body) == 0 {
		return nil, false
	}
	lines := splitLines(source)
	out := make([]noOpStmt, 0, len(mod.Body))
	for _, stmt := range mod.Body {
		var line int
		switch s := stmt.(type) {
		case *ast.Pass:
			line = s.P.Line
		case *ast.ExprStmt:
			c, ok := s.Value.(*ast.Constant)
			if !ok {
				return nil, false
			}
			switch c.Kind {
			case "None", "True", "False", "Ellipsis",
				"int", "float", "complex", "bytes":
				line = s.P.Line
			default:
				return nil, false
			}
		default:
			return nil, false
		}
		ec, ok := lineEndCol(lines, line)
		if !ok {
			return nil, false
		}
		out = append(out, noOpStmt{
			Line:    uint32(line),
			EndLine: uint32(line),
			EndCol:  uint16(ec),
		})
	}
	return out, true
}

// buildEmptyModule emits the synthetic prologue CPython generates
// for a module whose body is empty. All three instructions share
// the synthetic-prologue Loc; the PEP 626 encoder turns them into
// a single LONG entry covering three code units.
func buildEmptyModule(opts Options) (*bytecode.CodeObject, error) {
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = []ir.Instr{
		{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		{Op: bytecode.LOAD_CONST, Arg: 0, Loc: syntheticLoc},
		{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: syntheticLoc},
	}

	return assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{},
	})
}

// buildNoOpsModule emits the bytecode CPython generates for a module
// body of N >= 1 no-op statements:
//
//	RESUME 0           ; synthetic prologue, Loc{0,1,0,0}
//	NOP                ; stmts[0]      (only when N >= 2)
//	...
//	NOP                ; stmts[N-2]
//	LOAD_CONST 0       ; stmts[N-1] — None
//	RETURN_VALUE       ; stmts[N-1]
//
// The encoder collapses RESUME into a 1-unit synthetic prologue,
// emits one entry per non-last stmt (length 1), and merges
// LOAD_CONST + RETURN_VALUE into a 2-unit entry sharing the last
// stmt's Loc. The bytes match `bytecode.LineTableNoOps`
// byte-for-byte; the parity test asserts this for every fixture.
func buildNoOpsModule(stmts []noOpStmt, opts Options) (*bytecode.CodeObject, error) {
	if len(stmts) == 0 {
		return nil, errors.New("codegen.buildNoOpsModule: empty stmt list")
	}
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
	)
	for _, s := range stmts[:len(stmts)-1] {
		block.Instrs = append(block.Instrs, ir.Instr{
			Op:  bytecode.NOP,
			Arg: 0,
			Loc: locOf(s),
		})
	}
	last := stmts[len(stmts)-1]
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: locOf(last)},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: locOf(last)},
	)

	return assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{nil},
		Names:    []string{},
	})
}

func locOf(s noOpStmt) bytecode.Loc {
	return bytecode.Loc{Line: s.Line, EndLine: s.EndLine, Col: 0, EndCol: s.EndCol}
}
