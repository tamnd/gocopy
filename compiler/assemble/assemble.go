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
// for the final layout). v0.6.5 uses Source to copy these from the
// fixture's existing CodeObject so the round-trip can compare bytes.
//
// v0.6.6 onward fills these from the symtable scope and codegen
// options; Source becomes optional.
type Options struct {
	Source *bytecode.CodeObject
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
	}
	return co, nil
}
