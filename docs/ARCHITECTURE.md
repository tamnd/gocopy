# Architecture

A developer-facing tour of the pipeline. Each section drills down into
one stage of `source.py → .pyc`.

## Pipeline

```
source.py
  │
  ├── gopapy.ParseFile     →  ast.Module
  │
  ├── compiler.Compile     →  bytecode.CodeObject
  │     ├── walk AST scope by scope
  │     ├── emit opcodes (with inline cache placeholders)
  │     ├── compute jump targets (two-pass: labels then deltas)
  │     ├── linetable assembly (PEP 626 compact form)
  │     └── exceptiontable assembly (PEP 657 compact form)
  │
  ├── marshal.Marshal      →  []byte (CPython marshal stream)
  │     ├── two-pass: count refs, then emit with FLAG_REF / TYPE_REF
  │     └── small-form encoding (TYPE_SMALL_TUPLE / TYPE_SHORT_ASCII)
  │
  └── pyc.WriteFile        →  source.pyc (16-byte header + marshal stream)
```

## Packages

- `bytecode/`: opcode constants, inline-cache table, instruction
  representation, mutable `CodeObject` builder, line-table and
  exception-table encoders, `co_flags` constants.
- `compiler/`: AST walker; one entry point `Compile(*ast.Module,
  Options) (*bytecode.CodeObject, error)`. Internally splits per AST
  node kind. Symbol-table pass and scope tree live here.
- `marshal/`: `Marshal(*bytecode.CodeObject) ([]byte, error)`. The
  inverse of `goipy/marshal/reader.go`. Type-tag constants mirror
  goipy.
- `pyc/`: 16-byte header writer plus full-file `WriteFile`. Hash
  mode uses SipHash-1-3 to mirror CPython's `_imp.source_hash`.
- `cmd/gocopy/`: the CLI.

## v0.0.1 status

Only an empty module compiles. The whole point of v0.0.1 is to
exercise every package end-to-end on the smallest possible program
so that every subsequent feature is a localised change.

## Why bytes?

CPython's `.pyc` is a stable wire format read by every Python
interpreter on the planet. If gocopy emits something CPython
cannot import, it is broken. The byte-for-byte oracle
(`tests/run.sh`) is what catches that. Every PR must keep the
oracle at zero diff for every fixture.

## Why two-pass marshal

CPython's marshal writer adds `FLAG_REF` to the type byte iff
`Py_REFCNT(obj) > 1`. We don't have refcounts in Go, so the
writer makes one pass to count occurrences in the marshal walk
order, then a second pass to emit. Same byte sequence, different
mechanism.

## Working agreement

- One PR per version. Squash-merge.
- Every PR adds at least one `tests/fixtures/*.py` whose
  oracle diff goes from non-zero to zero.
- Voice: human, no AI register, no em-dashes.
