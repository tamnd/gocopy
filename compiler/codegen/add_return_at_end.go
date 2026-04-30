package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/ir"
)

// addReturnAtEnd appends LOAD_CONST None + RETURN_VALUE (or just
// RETURN_VALUE when addNone is false) at NO_LOCATION to the
// trailing block of u.Seq. Mirrors CPython 3.14
// Python/codegen.c::_PyCodegen_AddReturnAtEnd verbatim:
//
//	int
//	_PyCodegen_AddReturnAtEnd(compiler *c, int addNone)
//	{
//	    if (addNone) {
//	        ADDOP_LOAD_CONST(c, NO_LOCATION, Py_None);
//	    }
//	    ADDOP(c, NO_LOCATION, RETURN_VALUE);
//	    return SUCCESS;
//	}
//
// gocopy NO_LOCATION = bytecode.Loc{} (zero-valued). The Python
// None constant is added to u.Consts via u.addConst, which
// dedupes by equality so callers may invoke this helper safely
// after their own const-pool emits.
//
// SOURCE: CPython 3.14 Python/codegen.c:6473
// _PyCodegen_AddReturnAtEnd.
func addReturnAtEnd(u *compileUnit, addNone bool) {
	if u == nil || u.Seq == nil {
		return
	}
	block := u.currentBlock()
	if block == nil {
		block = u.Seq.AddBlock()
	}
	if addNone {
		idx := u.addConst(nil)
		block.Instrs = append(block.Instrs, ir.Instr{
			Op: bytecode.LOAD_CONST, Arg: idx, Loc: bytecode.Loc{},
		})
	}
	block.Instrs = append(block.Instrs, ir.Instr{
		Op: bytecode.RETURN_VALUE, Arg: 0, Loc: bytecode.Loc{},
	})
}
