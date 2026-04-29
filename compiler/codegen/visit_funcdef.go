package codegen

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ast"
)

// funcDefModule captures the modFuncDef shape: a top-level
// `def f(arg): return arg` definition where f and arg are 1..15-char
// ASCII identifiers, the body is exactly one `return <argName>`
// statement, and there are no decorators, annotations, type-params,
// defaults, or variadic parameters. Mirrors
// `compiler/classify_ast.go::extractFuncDef` byte-for-byte.
type funcDefModule struct {
	FuncName string
	ArgName  string
	DefLine  int
	BodyLine int
	ArgCol   byte
	ArgEnd   byte
	RetKwCol byte
}

// classifyFuncDefModule recognises a single-statement module whose
// only stmt is `*ast.FunctionDef` matching the modFuncDef shape.
func classifyFuncDefModule(mod *ast.Module, _ []byte) (funcDefModule, bool) {
	if len(mod.Body) != 1 {
		return funcDefModule{}, false
	}
	fd, ok := mod.Body[0].(*ast.FunctionDef)
	if !ok {
		return funcDefModule{}, false
	}
	if len(fd.DecoratorList) != 0 || fd.Returns != nil || len(fd.TypeParams) != 0 {
		return funcDefModule{}, false
	}
	args := fd.Args
	if args == nil || len(args.PosOnly) != 0 || len(args.KwOnly) != 0 ||
		args.Vararg != nil || args.Kwarg != nil || len(args.Defaults) != 0 {
		return funcDefModule{}, false
	}
	if len(args.Args) != 1 {
		return funcDefModule{}, false
	}
	arg := args.Args[0]
	if arg.Annotation != nil || arg.P.Col > 255 || len(arg.Name) < 1 || len(arg.Name) > 15 {
		return funcDefModule{}, false
	}
	if len(fd.Name) < 1 || len(fd.Name) > 15 || fd.P.Col > 255 {
		return funcDefModule{}, false
	}
	if len(fd.Body) != 1 {
		return funcDefModule{}, false
	}
	ret, isReturn := fd.Body[0].(*ast.Return)
	if !isReturn || ret.Value == nil {
		return funcDefModule{}, false
	}
	retName, isName := ret.Value.(*ast.Name)
	if !isName || retName.Id != arg.Name {
		return funcDefModule{}, false
	}
	if ret.P.Col > 255 || retName.P.Col > 255 {
		return funcDefModule{}, false
	}
	argCol := byte(retName.P.Col)
	return funcDefModule{
		FuncName: fd.Name,
		ArgName:  arg.Name,
		DefLine:  fd.P.Line,
		BodyLine: ret.P.Line,
		ArgCol:   argCol,
		ArgEnd:   argCol + byte(len(retName.Id)),
		RetKwCol: byte(ret.P.Col),
	}, true
}

// buildFuncDefModule emits the dual CodeObject (inner function +
// outer module wrapper) CPython 3.14 generates for
// `def f(arg): return arg` at module scope. Mirrors
// `compiler/compiler.go::compileFuncDef` byte-for-byte using the
// existing `bytecode.FuncReturnArg*` and `bytecode.FuncDefModule*`
// helpers — no IR/assembler routing for the inner code object yet.
//
// Inner function `f`:
//
//	RESUME 0
//	LOAD_FAST_BORROW 0   ; the single arg
//	RETURN_VALUE 0
//
// ArgCount=1, StackSize=1, Flags=0x3 (CO_OPTIMIZED|CO_NEWLOCALS).
// LocalsPlusNames=[argName], LocalsPlusKinds=[LocalsKindArg=0x26].
// Name=QualName=funcName (no `<module>.` prefix at module scope).
// FirstLineNo=defLine.
//
// Outer module:
//
//	RESUME 0
//	LOAD_CONST 0         ; the inner code object
//	MAKE_FUNCTION 0
//	STORE_NAME 0         ; funcName
//	LOAD_CONST 1         ; None
//	RETURN_VALUE 0
//
// co_consts=[funcCode, nil]; co_names=[funcName]; StackSize=1;
// FirstLineNo=defLine (matches classifier).
func buildFuncDefModule(f funcDefModule, opts Options) (*bytecode.CodeObject, error) {
	if f.FuncName == "" || f.ArgName == "" {
		return nil, errors.New("codegen.buildFuncDefModule: empty func or arg name")
	}

	funcCode := &bytecode.CodeObject{
		ArgCount:        1,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0x3,
		Bytecode:        bytecode.FuncReturnArgBytecode(0),
		Consts:          []any{nil},
		Names:           []string{},
		LocalsPlusNames: []string{f.ArgName},
		LocalsPlusKinds: []byte{bytecode.LocalsKindArg},
		Filename:        opts.Filename,
		Name:            f.FuncName,
		QualName:        f.FuncName,
		FirstLineNo:     int32(f.DefLine),
		LineTable:       bytecode.FuncReturnArgLineTable(f.DefLine, f.BodyLine, f.ArgCol, f.ArgEnd, f.RetKwCol),
		ExcTable:        []byte{},
	}

	return &bytecode.CodeObject{
		ArgCount:        0,
		PosOnlyArgCount: 0,
		KwOnlyArgCount:  0,
		StackSize:       1,
		Flags:           0,
		Bytecode:        bytecode.FuncDefModuleBytecode(0),
		Consts:          []any{funcCode, nil},
		Names:           []string{f.FuncName},
		LocalsPlusNames: []string{},
		LocalsPlusKinds: []byte{},
		Filename:        opts.Filename,
		Name:            opts.Name,
		QualName:        opts.QualName,
		FirstLineNo:     int32(f.DefLine),
		LineTable:       bytecode.FuncDefModuleLineTable(f.DefLine, f.BodyLine, f.ArgEnd),
		ExcTable:        []byte{},
	}, nil
}
