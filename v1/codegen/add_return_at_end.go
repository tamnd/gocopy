package codegen

import (
	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/v1/ir"
)

// addReturnAtEnd appends LOAD_CONST None + RETURN_VALUE (or just
// RETURN_VALUE when addNone is false) at NO_LOCATION on a fresh
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
// CPython's cfg_builder auto-splits a new block after every
// terminator (return / raise / unconditional jump), so AddReturnAtEnd
// always lands in a fresh block. gocopy's currentBlock() doesn't
// auto-split; addReturnAtEnd allocates a fresh trailing block to
// match. When the body already terminated, the new block has zero
// predecessors and removeUnreachable zeroes it. When the body fell
// through, the new block becomes the fallthrough successor and
// propagateLineNumbers backfills the loc from the predecessor's
// last instr.
//
// SOURCE: CPython 3.14 Python/codegen.c:6473
// _PyCodegen_AddReturnAtEnd.
func addReturnAtEnd(u *compileUnit, addNone bool) {
	if u == nil || u.Seq == nil {
		return
	}
	block := u.Seq.AddBlock()
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
