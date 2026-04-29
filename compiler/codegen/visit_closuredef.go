package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
)

// closureDefModule captures the modClosureDef shape: a top-level
// `def f(outerArg): def g(): return outerArg; return g` definition
// where f, outerArg, and g are 1..15-char ASCII identifiers, the
// outer body is exactly two stmts (inner def + `return innerName`),
// the inner body is exactly one `return outerArgName`, and there
// are no decorators, annotations, defaults, or variadic params
// anywhere. Mirrors `compiler/classify_ast.go::extractClosure` byte-
// for-byte.
type closureDefModule struct {
	OuterFuncName   string
	ArgName         string
	InnerFuncName   string
	OuterDefLine    int
	InnerDefLine    int
	InnerRetLine    int
	OuterRetLine    int
	InnerDefCol     byte
	InnerBodyEndCol byte
	InnerFreeArgCol byte
	InnerFreeArgEnd byte
	InnerRetKwCol   byte
	OuterRetArgCol  byte
	OuterRetArgEnd  byte
	OuterRetKwCol   byte
}

// classifyClosureDefModule recognises a single-statement module
// whose only stmt is `*ast.FunctionDef` matching the modClosureDef
// shape.
func classifyClosureDefModule(mod *ast.Module, _ []byte) (closureDefModule, bool) {
	if len(mod.Body) != 1 {
		return closureDefModule{}, false
	}
	outer, ok := mod.Body[0].(*ast.FunctionDef)
	if !ok {
		return closureDefModule{}, false
	}
	if len(outer.DecoratorList) != 0 || outer.Returns != nil || len(outer.TypeParams) != 0 {
		return closureDefModule{}, false
	}
	args := outer.Args
	if args == nil || len(args.PosOnly) != 0 || len(args.KwOnly) != 0 ||
		args.Vararg != nil || args.Kwarg != nil || len(args.Defaults) != 0 {
		return closureDefModule{}, false
	}
	if len(args.Args) != 1 {
		return closureDefModule{}, false
	}
	arg := args.Args[0]
	if arg.Annotation != nil || arg.P.Col > 255 || len(arg.Name) < 1 || len(arg.Name) > 15 {
		return closureDefModule{}, false
	}
	if len(outer.Name) < 1 || len(outer.Name) > 15 || outer.P.Col > 255 {
		return closureDefModule{}, false
	}
	if len(outer.Body) != 2 {
		return closureDefModule{}, false
	}

	innerDef, isInnerDef := outer.Body[0].(*ast.FunctionDef)
	if !isInnerDef {
		return closureDefModule{}, false
	}
	if len(innerDef.DecoratorList) != 0 || innerDef.Returns != nil || len(innerDef.TypeParams) != 0 {
		return closureDefModule{}, false
	}
	innerArgs := innerDef.Args
	if innerArgs == nil || len(innerArgs.Args) != 0 || len(innerArgs.PosOnly) != 0 ||
		len(innerArgs.KwOnly) != 0 || innerArgs.Vararg != nil || innerArgs.Kwarg != nil {
		return closureDefModule{}, false
	}
	if len(innerDef.Name) < 1 || len(innerDef.Name) > 15 || innerDef.P.Col > 255 {
		return closureDefModule{}, false
	}
	if len(innerDef.Body) != 1 {
		return closureDefModule{}, false
	}
	innerRet, isInnerRet := innerDef.Body[0].(*ast.Return)
	if !isInnerRet || innerRet.Value == nil {
		return closureDefModule{}, false
	}
	innerRetName, isInnerName := innerRet.Value.(*ast.Name)
	if !isInnerName || innerRetName.Id != arg.Name {
		return closureDefModule{}, false
	}
	if innerRet.P.Col > 255 || innerRetName.P.Col > 255 {
		return closureDefModule{}, false
	}

	outerRet, isOuterRet := outer.Body[1].(*ast.Return)
	if !isOuterRet || outerRet.Value == nil {
		return closureDefModule{}, false
	}
	outerRetName, isOuterName := outerRet.Value.(*ast.Name)
	if !isOuterName || outerRetName.Id != innerDef.Name {
		return closureDefModule{}, false
	}
	if outerRet.P.Col > 255 || outerRetName.P.Col > 255 {
		return closureDefModule{}, false
	}

	innerBodyEndCol := byte(innerRetName.P.Col) + byte(len(innerRetName.Id))
	outerRetArgEnd := byte(outerRetName.P.Col) + byte(len(outerRetName.Id))

	return closureDefModule{
		OuterFuncName:   outer.Name,
		ArgName:         arg.Name,
		InnerFuncName:   innerDef.Name,
		OuterDefLine:    outer.P.Line,
		InnerDefLine:    innerDef.P.Line,
		InnerRetLine:    innerRet.P.Line,
		OuterRetLine:    outerRet.P.Line,
		InnerDefCol:     byte(innerDef.P.Col),
		InnerBodyEndCol: innerBodyEndCol,
		InnerFreeArgCol: byte(innerRetName.P.Col),
		InnerFreeArgEnd: innerBodyEndCol,
		InnerRetKwCol:   byte(innerRet.P.Col),
		OuterRetArgCol:  byte(outerRetName.P.Col),
		OuterRetArgEnd:  outerRetArgEnd,
		OuterRetKwCol:   byte(outerRet.P.Col),
	}, true
}

// buildClosureDefModule emits the triple CodeObject (inner function
// + outer function + module wrapper) CPython 3.14 generates for
// `def f(x): def g(): return x; return g` at module scope. Mirrors
// `compiler/compiler.go::compileClosure` byte-for-byte using the
// existing `bytecode.Closure*` and `bytecode.FuncDefModule*` helpers.
//
// Inner `g`: zero args, captures outerArg as a free variable. Flags
// 0x13 (CO_OPTIMIZED|CO_NEWLOCALS|CO_NESTED). LocalsKindFree.
// QualName = "<outerFuncName>.<locals>.<innerFuncName>".
//
// Outer `f`: one arg promoted to a cell (LocalsKindArgCell), inner
// func name as a plain local (LocalsKindLocal). Flags 0x03.
// StackSize 2. co_consts=[innerCode] (no None — all paths return
// explicitly).
//
// Module: standard MAKE_FUNCTION + STORE_NAME wrapper, identical
// to modFuncDef. co_consts=[outerCode, nil], co_names=[outerFuncName].
func buildClosureDefModule(c closureDefModule, opts Options) (*bytecode.CodeObject, error) {
	if c.OuterFuncName == "" || c.ArgName == "" || c.InnerFuncName == "" {
		return nil, errors.New("codegen.buildClosureDefModule: empty outer/arg/inner name")
	}

	innerCode := &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0x13,
		Bytecode:        bytecode.ClosureInnerBytecode(),
		Consts:          []any{nil},
		Names:           []string{},
		LocalsPlusNames: []string{c.ArgName},
		LocalsPlusKinds: []byte{bytecode.LocalsKindFree},
		Filename:        opts.Filename,
		Name:            c.InnerFuncName,
		QualName:        c.OuterFuncName + ".<locals>." + c.InnerFuncName,
		FirstLineNo:     int32(c.InnerDefLine),
		LineTable: bytecode.ClosureInnerLineTable(
			c.InnerDefLine, c.InnerRetLine,
			c.InnerFreeArgCol, c.InnerFreeArgEnd, c.InnerRetKwCol),
		ExcTable: []byte{},
	}

	outerCode := &bytecode.CodeObject{
		ArgCount:        1,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       2,
		Flags:           0x03,
		Bytecode:        bytecode.ClosureOuterBytecode(),
		Consts:          []any{innerCode},
		Names:           []string{},
		LocalsPlusNames: []string{c.ArgName, c.InnerFuncName},
		LocalsPlusKinds: []byte{bytecode.LocalsKindArgCell, bytecode.LocalsKindLocal},
		Filename:        opts.Filename,
		Name:            c.OuterFuncName,
		QualName:        c.OuterFuncName,
		FirstLineNo:     int32(c.OuterDefLine),
		LineTable: bytecode.ClosureOuterLineTable(
			c.OuterDefLine, c.InnerDefLine, c.InnerRetLine, c.OuterRetLine,
			c.InnerDefCol, c.InnerBodyEndCol,
			c.OuterRetArgCol, c.OuterRetArgEnd, c.OuterRetKwCol),
		ExcTable: []byte{},
	}

	return &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0,
		Bytecode:        bytecode.FuncDefModuleBytecode(0),
		Consts:          []any{outerCode, nil},
		Names:           []string{c.OuterFuncName},
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        opts.Filename,
		Name:            opts.Name,
		QualName:        opts.QualName,
		FirstLineNo:     int32(c.OuterDefLine),
		LineTable: bytecode.FuncDefModuleLineTable(
			c.OuterDefLine, c.OuterRetLine, c.OuterRetArgEnd),
		ExcTable: []byte{},
	}, nil
}
