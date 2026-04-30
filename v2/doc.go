// Package v2 is a fresh, line-by-line port of the CPython 3.14 compiler
// pipeline to idiomatic Go.
//
// Layout mirrors Python/*.c upstream — one Go package per CPython
// source file. There are no internal/ subfolders: every package is
// importable so per-port specs and tests can target each translation
// unit directly.
//
//	v2/preprocess/   ← cpython/Python/ast_preprocess.c
//	v2/future/       ← cpython/Python/future.c
//	v2/symtable/     ← cpython/Python/symtable.c
//	v2/codegen/      ← cpython/Python/codegen.c
//	v2/instrseq/     ← cpython/Python/instruction_sequence.c
//	v2/flowgraph/    ← cpython/Python/flowgraph.c
//	v2/assemble/     ← cpython/Python/assemble.c
//	v2/compile/      ← cpython/Python/compile.c (orchestrator)
//	v2/ast/          ← parser AST alias surface (parity with v1/ast/)
//	v2/lower/        ← parser→ast translation boundary (parity with v1/lower/)
//	v2/dump/         ← stage-dump + side-by-side diff tooling vs CPython
//
// v1/ contains the legacy gocopy compiler (frozen) and remains the
// shipping pipeline until v2/ reaches feature parity. See
// ~/notes/Spec/1500/1575_gocopy_v08x_full_cpython_port.md for the
// roadmap; per-file ports get their own spec under the same directory.
package v2
