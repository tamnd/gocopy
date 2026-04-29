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

// docstringModule captures the position of a module's leading string
// literal plus the trailing no-op statements (if any).
type docstringModule struct {
	Doc  noOpStmt
	Text string
	Tail []noOpStmt
}

// classifyDocstringModule recognizes a module body whose first
// statement is an *ast.ExprStmt wrapping an *ast.Constant of Kind
// "str", followed by zero or more no-op statements (the same
// predicate classifyNoOpModule uses for its statements). Returns the
// docstring's Loc and string value plus per-stmt Loc info for the
// tail.
func classifyDocstringModule(mod *ast.Module, source []byte) (docstringModule, bool) {
	if len(mod.Body) == 0 {
		return docstringModule{}, false
	}
	first, ok := mod.Body[0].(*ast.ExprStmt)
	if !ok {
		return docstringModule{}, false
	}
	c, ok := first.Value.(*ast.Constant)
	if !ok || c.Kind != "str" {
		return docstringModule{}, false
	}
	text, ok := c.Value.(string)
	if !ok {
		return docstringModule{}, false
	}
	lines := splitLines(source)
	docLine := c.P.Line
	docEndLine, docEndCol, tripleOK := findTripleQuoteEnd(lines, docLine)
	if !tripleOK {
		// No triple delimiter: every embedded newline in the value
		// would have to consume a source line, but plain "..." strings
		// with backslash escapes don't. Reject by source-line bounds
		// the same way the classifier does.
		n := 0
		for i := 0; i < len(text); i++ {
			if text[i] == '\n' {
				n++
			}
		}
		docEndLine = docLine + n
		if docEndLine > len(lines) {
			return docstringModule{}, false
		}
		ec, ok := lineEndCol(lines, docEndLine)
		if !ok {
			return docstringModule{}, false
		}
		docEndCol = ec
	}
	// ASCII-only: each newline-separated segment of the docstring must
	// be plain printable ASCII with no backslash. Mirrors the classifier
	// — keeps codegen producing bytes only for the shapes the oracle
	// has validated.
	for _, seg := range splitOnNewline(text) {
		if !isPlainAsciiNoEscape(seg) {
			return docstringModule{}, false
		}
	}
	doc := noOpStmt{
		Line:    uint32(docLine),
		EndLine: uint32(docEndLine),
		EndCol:  uint16(docEndCol),
	}
	tail := make([]noOpStmt, 0, len(mod.Body)-1)
	for _, stmt := range mod.Body[1:] {
		var line int
		switch s := stmt.(type) {
		case *ast.Pass:
			line = s.P.Line
		case *ast.ExprStmt:
			cc, ok := s.Value.(*ast.Constant)
			if !ok {
				return docstringModule{}, false
			}
			switch cc.Kind {
			case "None", "True", "False", "Ellipsis",
				"int", "float", "complex", "bytes":
				line = s.P.Line
			default:
				return docstringModule{}, false
			}
		default:
			return docstringModule{}, false
		}
		ec, ok := lineEndCol(lines, line)
		if !ok {
			return docstringModule{}, false
		}
		tail = append(tail, noOpStmt{
			Line:    uint32(line),
			EndLine: uint32(line),
			EndCol:  uint16(ec),
		})
	}
	return docstringModule{Doc: doc, Text: text, Tail: tail}, true
}

// buildDocstringModule emits the bytecode CPython generates for a
// module body of `"""text"""` followed by N no-op tail statements:
//
//	RESUME 0           ; synthetic prologue, Loc{0,1,0,0}
//	LOAD_CONST 0       ; docstring text — Loc on the docstring stmt
//	STORE_NAME 0       ; "__doc__"      — Loc on the docstring stmt
//	NOP                ; tail[0]      (when len(Tail) >= 2)
//	...
//	NOP                ; tail[N-2]
//	LOAD_CONST 1       ; None — Loc on the last stmt (docstring if
//	                  ;        Tail is empty, else tail[len-1])
//	RETURN_VALUE       ; Loc on the last stmt
//
// co_consts = [text, nil]; co_names = ["__doc__"]. Run-merging in
// the PEP 626 encoder produces bytes byte-identical to
// bytecode.DocstringBytecode + bytecode.DocstringLineTable.
func buildDocstringModule(d docstringModule, opts Options) (*bytecode.CodeObject, error) {
	syntheticLoc := bytecode.Loc{Line: 0, EndLine: 1}
	docLoc := locOf(d.Doc)

	seq := ir.NewInstrSeq()
	seq.FirstLineNo = opts.FirstLineNo
	block := seq.AddBlock()
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: syntheticLoc},
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 0, Loc: docLoc},
		ir.Instr{Op: bytecode.STORE_NAME, Arg: 0, Loc: docLoc},
	)
	lastLoc := docLoc
	if len(d.Tail) > 0 {
		for _, s := range d.Tail[:len(d.Tail)-1] {
			block.Instrs = append(block.Instrs, ir.Instr{
				Op:  bytecode.NOP,
				Arg: 0,
				Loc: locOf(s),
			})
		}
		lastLoc = locOf(d.Tail[len(d.Tail)-1])
	}
	block.Instrs = append(block.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: 1, Loc: lastLoc},
		ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: lastLoc},
	)

	return assemble.Assemble(seq, assemble.Options{
		Filename: opts.Filename,
		Name:     opts.Name,
		QualName: opts.QualName,
		Consts:   []any{d.Text, nil},
		Names:    []string{"__doc__"},
	})
}
