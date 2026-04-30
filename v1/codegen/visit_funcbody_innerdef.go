package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/assemble"
	"github.com/tamnd/gocopy/v1/ast"
	"github.com/tamnd/gocopy/v1/ir"
	"github.com/tamnd/gocopy/v1/symtable"
)

// validateFuncBodyInnerDef reports whether s is a `def` whose shape
// the v0.7.10.12 funcbody visitor accepts as a nested function inside
// another funcbody. The inner def must satisfy:
//
//   - 1..15-byte name, col ≤ 255.
//   - No decorators / Returns annotation / TypeParams.
//   - Args zero (no PosOnly / Args / KwOnly / Vararg / Kwarg /
//     Defaults). Multi-arg inner defs are deferred.
//   - Body shape is one of:
//
//     1. [Return(rhs)] where rhs validates as ordinary funcbody RHS.
//     2. [Nonlocal(names...), Assign(target = rhs)] — fixture 048
//     mutate-via-nonlocal pattern. Implicit None return is appended.
//
// Out of scope (deferred):
//
//   - Inner def with non-zero arity.
//   - Inner def with cells of its own (3-deep capture / grandchild
//     captures from outermost). v0.7.10.13 absorbs.
//   - Inner def with decorators / annotations / type params.
func validateFuncBodyInnerDef(s *ast.FunctionDef) bool {
	if s == nil || s.P.Col > 255 {
		return false
	}
	if len(s.Name) < 1 || len(s.Name) > 15 {
		return false
	}
	if len(s.DecoratorList) != 0 || s.Returns != nil || len(s.TypeParams) != 0 {
		return false
	}
	args := s.Args
	if args == nil {
		return false
	}
	if len(args.PosOnly) != 0 || len(args.Args) != 0 || len(args.KwOnly) != 0 ||
		args.Vararg != nil || args.Kwarg != nil ||
		len(args.Defaults) != 0 || len(args.KwOnlyDef) != 0 {
		return false
	}
	if len(s.Body) < 1 {
		return false
	}
	switch len(s.Body) {
	case 1:
		ret, ok := s.Body[0].(*ast.Return)
		if !ok || ret.Value == nil || ret.P.Col > 255 {
			return false
		}
		return validateFuncBodyAssignRHS(ret.Value)
	case 2:
		nl, ok := s.Body[0].(*ast.Nonlocal)
		if !ok || !validateFuncBodyGlobalNonlocal(nl.Names, nl.P) {
			return false
		}
		a, ok := s.Body[1].(*ast.Assign)
		if !ok || !validateFuncBodyAssign(a) {
			return false
		}
		return true
	}
	return false
}

// emitFuncBodyInnerDef lowers `def inner(): ...` as a non-last
// statement inside a funcbody. Mirrors CPython 3.14
// Python/codegen.c::codegen_visit_stmt_function_def +
// Python/codegen.c::codegen_make_closure for the zero-arg, zero-
// decorator surface.
//
// Layout:
//
//   - Locate the matching child symtable.Scope inside outer.Scope.
//   - Push a child compileUnit.
//   - Emit the inner's MAKE_CELL / COPY_FREE_VARS prologue (none for
//     the v0.7.10.12 surface — innerScope.Cells is always empty here
//     and frees come from outer captures only).
//   - Emit RESUME 0 at firstLineno.
//   - Emit body statements.
//   - For `[Return(value)]` body: visit value + RETURN_VALUE.
//   - For `[Nonlocal(...), Assign(t = const)]` body: emit Assign,
//     then the implicit `LOAD_CONST None + RETURN_VALUE` pair.
//   - addConst(nil) lazy: only if not already present.
//   - popChildUnit → innerCode.
//   - In outer block: build closure tuple if inner has frees
//     (LOAD_DEREF / LOAD_FAST_BORROW for each free in innerScope.Frees
//     order, then BUILD_TUPLE n), LOAD_CONST innerCode, MAKE_FUNCTION,
//     SET_FUNCTION_ATTRIBUTE 8 (closure) if frees present, STORE_FAST
//     / STORE_DEREF to the inner's name slot in outer.
//
// Locs follow CPython:
//
//   - All MAKE_FUNCTION-ish bytes carry innerDefRunLoc spanning
//     (innerDefCol, innerBodyEndCol) on innerDefLine.
//   - Closure-tuple operand loads carry each free's reference Loc
//     (the position in inner where the cell was bound). For our
//     surface the cell binding is the `def` line of the outer scope
//     where the name first lives — we use innerDefRunLoc as a tight
//     approximation, which the optimize_load_fast pass leaves
//     untouched at this granularity.
//
// SOURCE: CPython 3.14 Python/codegen.c::codegen_function +
// codegen_make_closure + Python/compile.c::compiler_make_cell.
func emitFuncBodyInnerDef(outer *compileUnit, s *ast.FunctionDef, lines [][]byte) error {
	innerScope := findFunctionChildScope(outer.Scope, s)
	if innerScope == nil {
		return ErrNotImplemented
	}

	// Reject inner-with-cells (3-deep capture). Surface is empty cells
	// for fixtures 048 and 051.
	if len(innerScope.Cells) != 0 {
		return ErrNotImplemented
	}

	defLine := s.P.Line
	if defLine < 1 {
		return ErrNotImplemented
	}

	innerLocalsPlusNames := innerScope.LocalsPlusNames()
	innerLocalsPlusKinds := innerScope.LocalsPlusKinds()
	if len(innerLocalsPlusNames) > 15 {
		return ErrNotImplemented
	}
	for _, n := range innerLocalsPlusNames {
		if len(n) > 15 {
			return ErrNotImplemented
		}
	}

	innerQual := outer.QualName + ".<locals>." + s.Name
	inner := outer.pushChildUnit(s.Name, innerQual, int32(defLine))
	inner.Scope = innerScope
	innerBlock := inner.Seq.AddBlock()

	// COPY_FREE_VARS prologue: emit BEFORE RESUME for closure inners.
	// MAKE_CELL prologue would precede this if inner had cells of its
	// own; the v0.7.10.12 surface rejects that case above.
	if n := len(innerScope.Frees); n > 0 {
		innerBlock.Instrs = append(innerBlock.Instrs, ir.Instr{
			Op: bytecode.COPY_FREE_VARS, Arg: uint32(n), Loc: bytecode.Loc{},
		})
	}
	resumeLoc := bytecode.Loc{
		Line: uint32(defLine), EndLine: uint32(defLine),
		Col: 0, EndCol: 0,
	}
	innerBlock.Instrs = append(innerBlock.Instrs,
		ir.Instr{Op: bytecode.RESUME, Arg: 0, Loc: resumeLoc},
	)

	// Body emit, plus implicit None return for the Nonlocal+Assign
	// shape.
	switch len(s.Body) {
	case 1:
		ret := s.Body[0].(*ast.Return)
		if _, err := emitFuncBodyReturn(inner, ret, lines); err != nil {
			return err
		}
	case 2:
		// body[0] is Nonlocal — no bytecode emit (symtable directive).
		assign := s.Body[1].(*ast.Assign)
		if err := emitFuncBodyAssign(inner, assign, lines); err != nil {
			return err
		}
		// Implicit `return None`. Loc spans the assign's target line —
		// CPython's propagate_line_numbers attributes this implicit
		// pair to the last source line of the body.
		target := assign.Targets[0].(*ast.Name)
		retLoc := bytecode.Loc{
			Line: uint32(target.P.Line), EndLine: uint32(target.P.Line),
			Col:    uint16(target.P.Col),
			EndCol: uint16(target.P.Col) + uint16(len(target.Id)),
		}
		noneIdx := inner.addConst(nil)
		blk := inner.currentBlock()
		blk.Instrs = append(blk.Instrs,
			ir.Instr{Op: bytecode.LOAD_CONST, Arg: noneIdx, Loc: retLoc},
			ir.Instr{Op: bytecode.RETURN_VALUE, Arg: 0, Loc: retLoc},
		)
	}

	if len(inner.Consts) == 0 {
		inner.addConst(nil)
	}

	innerArgs := s.Args
	flags := bytecode.CO_OPTIMIZED | bytecode.CO_NEWLOCALS | bytecode.CO_NESTED
	if innerArgs.Vararg != nil {
		flags |= bytecode.CO_VARARGS
	}
	if innerArgs.Kwarg != nil {
		flags |= bytecode.CO_VARKEYWORDS
	}

	innerCode, err := outer.popChildUnit(inner, assemble.Options{
		ArgCount:        int32(len(innerArgs.PosOnly) + len(innerArgs.Args)),
		PosOnlyArgCount: int32(len(innerArgs.PosOnly)),
		KwOnlyArgCount:  int32(len(innerArgs.KwOnly)),
		Flags:           flags,
		LocalsPlusNames: innerLocalsPlusNames,
		LocalsPlusKinds: innerLocalsPlusKinds,
		Filename:        outer.Filename,
		Name:            s.Name,
		QualName:        innerQual,
	})
	if err != nil {
		return err
	}

	// Build the def-emit Loc spanning (innerDefCol, innerBodyEndLine,
	// innerBodyEndCol). CPython's codegen_function uses LOC(s) for
	// every byte in the MAKE_FUNCTION cluster — that's the def
	// statement's full span: from def keyword (line, col) to the end
	// of the last body statement (endLine, endCol).
	innerBodyEndLine, innerBodyEndCol := innerDefBodyEnd(s, lines)
	innerDefRunLoc := bytecode.Loc{
		Line:    uint32(defLine),
		EndLine: uint32(innerBodyEndLine),
		Col:     uint16(s.P.Col),
		EndCol:  innerBodyEndCol,
	}

	outerBlock := outer.currentBlock()
	hasFree := len(innerScope.Frees) > 0
	if hasFree {
		// Push each free variable's parent-scope cell onto the stack
		// in innerScope.Frees order, then BUILD_TUPLE. CPython's
		// compiler_make_closure (Python/codegen.c) bypasses
		// compiler_nameop here: it loads the CELL OBJECT itself, not
		// the cell's value. For a name that is a cell in outer scope,
		// the cell lives in the fast slot (after MAKE_CELL), so we
		// emit LOAD_FAST. For a name that is itself free in outer
		// (i.e., a captured name passed through), the cell sits in
		// the free-vars area and we emit LOAD_DEREF. The slot index
		// in both cases is Symbol.Index.
		for _, free := range innerScope.Frees {
			sym, ok := outer.Scope.Symbols[free]
			if !ok {
				return ErrNotImplemented
			}
			var op bytecode.Opcode
			switch {
			case sym.Flags.HasAny(symtable.SymFree):
				op = bytecode.LOAD_DEREF
			case sym.Flags.HasAny(symtable.SymCell):
				op = bytecode.LOAD_FAST
			default:
				return ErrNotImplemented
			}
			outer.currentBlock().Instrs = append(outer.currentBlock().Instrs, ir.Instr{
				Op: op, Arg: uint32(sym.Index), Loc: innerDefRunLoc,
			})
		}
		outerBlock = outer.currentBlock()
		outerBlock.Instrs = append(outerBlock.Instrs, ir.Instr{
			Op: bytecode.BUILD_TUPLE, Arg: uint32(len(innerScope.Frees)), Loc: innerDefRunLoc,
		})
	}

	innerCodeIdx := outer.addConst(innerCode)
	outerBlock.Instrs = append(outerBlock.Instrs,
		ir.Instr{Op: bytecode.LOAD_CONST, Arg: innerCodeIdx, Loc: innerDefRunLoc},
		ir.Instr{Op: bytecode.MAKE_FUNCTION, Arg: 0, Loc: innerDefRunLoc},
	)
	if hasFree {
		outerBlock.Instrs = append(outerBlock.Instrs, ir.Instr{
			Op: bytecode.SET_FUNCTION_ATTRIBUTE, Arg: 8, Loc: innerDefRunLoc,
		})
	}
	// STORE the freshly-built function into outer's slot for s.Name.
	// When inner.Name is also a cell of outer (the inner is itself
	// captured by another nested scope), emitNameStore picks
	// STORE_DEREF; otherwise STORE_FAST.
	tgtLoc := innerDefRunLoc
	outer.emitNameStore(s.Name, tgtLoc)

	return nil
}

// innerDefBodyEnd returns (endLine, endCol) of the inner def's last
// body element. CPython's AST carries end_lineno/end_col_offset on
// FunctionDef directly; gocopy's gopapy AST does not, so we recover
// the span from the last body statement.
//
// Surface body shapes for the v0.7.10.12 funcbody visitor:
//
//   - [Return(value)]: span ends at value's end-col on ret.P.Line.
//   - [Nonlocal(...), Assign(target = rhs)]: span ends at rhs's
//     end-col on assign.P.Line.
//
// Both shapes are validated by validateFuncBodyInnerDef before we
// reach here. CPython attributes MAKE_FUNCTION (and the whole
// closure-cluster) to LOC(s) which spans through this end.
func innerDefBodyEnd(s *ast.FunctionDef, lines [][]byte) (int, uint16) {
	last := s.Body[len(s.Body)-1]
	switch v := last.(type) {
	case *ast.Return:
		if v.Value == nil {
			return v.P.Line, uint16(v.P.Col) + uint16(len("return"))
		}
		return v.P.Line, uint16(astExprEndCol(lines, v.P.Line, v.Value))
	case *ast.Assign:
		return v.P.Line, uint16(astExprEndCol(lines, v.P.Line, v.Value))
	}
	return s.P.Line, uint16(s.P.Col) + uint16(len("def ")+len(s.Name)+len("():"))
}
