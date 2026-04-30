// Package assemble turns an instruction-sequence IR plus a CFG
// into a fully-formed *bytecode.CodeObject. It is the back end of
// gocopy's v0.6.x compiler pipeline, mirroring CPython 3.14
// Python/assemble.c.
//
// At v0.6.5 no compiler shape routes through Assemble; the package
// is exercised solely by the round-trip test
// (Decode → Build → Linearize → Assemble → byte-equal). v0.6.6
// starts wiring shapes through.
package assemble

import (
	"errors"

	"github.com/tamnd/gocopy/bytecode"
	"github.com/tamnd/gocopy/compiler/flowgraph"
	"github.com/tamnd/gocopy/compiler/ir"
)

// Options carries metadata fields the IR does not yet hold (the
// CodeObject's filename, name, qualname, flags, argcount tuple,
// names/locals/consts when the IR's containers are not authoritative
// for the final layout).
//
// Two ways to populate the metadata:
//
//   - Source != nil: every metadata field is copied from Source.
//     This is the v0.6.5 round-trip path: the fixture's CodeObject
//     supplies filename/name/qualname/flags/consts/etc., and the
//     assembler only re-derives the byte-level fields.
//   - Source == nil: the explicit fields below populate the
//     CodeObject. This is the v0.6.6+ codegen path; the driver
//     hands codegen-built consts/names directly.
type Options struct {
	Source *bytecode.CodeObject

	Filename string
	Name     string
	QualName string
	Flags    uint32

	ArgCount        int32
	PosOnlyArgCount int32
	KwOnlyArgCount  int32

	Consts          []any
	Names           []string
	LocalsPlusNames []string
	LocalsPlusKinds []byte
}

// Assemble materializes a CodeObject from an InstrSeq.
//
// The IR's bytecode is emitted via ir.Encode. The line table is
// emitted via EncodeLineTable over per-instruction Loc data. The
// stack size is computed by abstract interpretation over the CFG
// recovered from seq. The exception table is empty at v0.6.5;
// v0.6.10 replaces the empty case with real region encoding.
//
// Other CodeObject fields (filename, name, qualname, flags, the
// argcount tuple, consts, names, localsplus*) come from
// opts.Source when provided; v0.6.5's round-trip always provides
// it. v0.6.6+ paths populate these from the symtable.
func Assemble(seq *ir.InstrSeq, opts Options) (*bytecode.CodeObject, error) {
	if seq == nil {
		return nil, errors.New("assemble.Assemble: nil InstrSeq")
	}
	cfg, err := flowgraph.Build(seq)
	if err != nil {
		return nil, err
	}
	bcode, _, err := ir.Encode(seq)
	if err != nil {
		return nil, err
	}
	ltab := EncodeLineTable(seq)
	etab := EncodeExcTable(nil)
	stack := StackDepth(cfg)
	// Mirror CPython 3.14 Objects/codeobject.c::_PyCode_Validate
	// (lines 517-519): every code object's stacksize is bumped to
	// at least 1. Functions that emit only zero-effect ops (e.g.
	// bare `raise`) would otherwise compute stacksize=0.
	if stack == 0 {
		stack = 1
	}

	co := &bytecode.CodeObject{
		Bytecode:    bcode,
		LineTable:   ltab,
		ExcTable:    etab,
		StackSize:   stack,
		FirstLineNo: seq.FirstLineNo,
	}
	if src := opts.Source; src != nil {
		co.ArgCount = src.ArgCount
		co.PosOnlyArgCount = src.PosOnlyArgCount
		co.KwOnlyArgCount = src.KwOnlyArgCount
		co.Flags = src.Flags
		co.Consts = src.Consts
		co.Names = src.Names
		co.LocalsPlusNames = src.LocalsPlusNames
		co.LocalsPlusKinds = src.LocalsPlusKinds
		co.Filename = src.Filename
		co.Name = src.Name
		co.QualName = src.QualName
	} else {
		co.ArgCount = opts.ArgCount
		co.PosOnlyArgCount = opts.PosOnlyArgCount
		co.KwOnlyArgCount = opts.KwOnlyArgCount
		co.Flags = opts.Flags
		co.Consts = opts.Consts
		co.Names = opts.Names
		co.LocalsPlusNames = opts.LocalsPlusNames
		co.LocalsPlusKinds = opts.LocalsPlusKinds
		co.Filename = opts.Filename
		co.Name = opts.Name
		co.QualName = opts.QualName
		if co.Names == nil {
			co.Names = []string{}
		}
		if co.LocalsPlusNames == nil {
			co.LocalsPlusNames = []string{}
		}
		if co.LocalsPlusKinds == nil {
			co.LocalsPlusKinds = []byte{}
		}
	}
	return co, nil
}
