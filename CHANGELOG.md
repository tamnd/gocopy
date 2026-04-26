# Changelog

All notable changes to gocopy are recorded here. The format follows
[Keep a Changelog 1.1](https://keepachangelog.com/en/1.1.0/). Once
gocopy reaches 1.0 the project will follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html); until
then, expect minor version bumps to sometimes include breaking
changes.

## [Unreleased]

## [0.0.4] - 2026-04-26

`gocopy compile` accepts blank or comment-only lines anywhere in the
source: leading, trailing, or between no-op statements. v0.0.3
required the body to be on consecutive lines starting at line 1;
this rung lifts that constraint without enlarging the no-op token
set.

CPython's lowering once a statement is no longer on `prev_line + 1`:

- delta = 1 - ONE_LINE1 (already covered in v0.0.3).
- delta = 2 - ONE_LINE2.
- delta >= 3 - LONG entry: svarint(line_delta), end_line_delta=0,
  varint(start_col+1), varint(end_col+1).

A leading blank pushes the first statement off line 1, which
collapses to a single LONG entry covering the whole body.

Verified against `python3.14 -m py_compile` for gaps from one line
up through ten blank lines.

### Added

- `bytecode.NoOpStmt{Line, EndCol}` carries each statement's source
  line so the encoder can compute deltas.
- Private `appendVarint` and `appendSignedVarint` primitives in
  `bytecode`, implementing CPython's base-64 varint and zigzag
  svarint from `Objects/locations.md`.
- Five new fixtures: `014_pass_blank_pass.py` through
  `018_mixed_gaps.py`.

### Changed

- `bytecode.LineTableNoOps` now takes `[]NoOpStmt` instead of
  `[]byte`. The single-no-op helper still wraps it in one line.
- The classifier accepts blank/comment lines anywhere and records
  each statement's source line.

### Deferred

- Multiple statements on the same line (`pass; pass`); the encoder
  could already emit `ONE_LINE0` but the scanner has no semicolon
  parser, so this whole branch waits on real parsing.
- String / bytes literal as a top-level statement (docstring path).
- Wiring gopapy as the parser; still waiting on a gopapy v1.0.0.

## [0.0.3] - 2026-04-26

`gocopy compile` accepts multiple no-op statements on consecutive
lines. The N=1 case is v0.0.2; everything from `pass\npass\n` up
through five mixed constants on five lines now matches
`python3.14 -m py_compile` byte-for-byte.

CPython's lowering for an N-statement no-op body:

- bytecode: `RESUME` + (N-1) × `NOP` + `LOAD_CONST 0` + `RETURN_VALUE`
- consts tuple: `(None,)`
- line table: synthetic prologue + (N-1) × `ONE_LINE1`(1 unit) +
  one `ONE_LINE1`(2 units)

We mirror that exactly.

### Added

- `bytecode.NoOpBytecode(n)` and `bytecode.LineTableNoOps(endCols)`,
  the multi-statement generalisation of v0.0.2's single-no-op
  helpers. The single-no-op helper now wraps `LineTableNoOps`.
- `bytecode.NOP` opcode constant (CPython 3.14 opcode 27).
- Five new fixtures: `009_two_pass.py` through `013_five_consts.py`.

### Changed

- The classifier in `compiler` collects a slice of statement end
  columns instead of returning one. Body shape enum collapses
  `modSingleNoOp` into a general `modNoOps`.

### Deferred

- Blank or comment lines BETWEEN statements (the encoder will
  pick up `ONE_LINE0` / `ONE_LINE2` entries for the larger line
  deltas).
- String / bytes literal as a top-level statement (docstring path).
- Wiring gopapy as the parser; still waiting on a gopapy v1.0.0.

## [0.0.2] - 2026-04-26

`gocopy compile` now accepts a single `pass` statement, or a single
bare non-string constant expression statement, on top of v0.0.1's
empty-module path. Output stays byte-identical to
`python3.14 -m py_compile` for every shape in scope.

The bytecode and consts tuple are unchanged from the empty-module
case (CPython lowers all of these to the same RESUME / LOAD_CONST
None / RETURN_VALUE prologue). The only thing that moves is the
PEP 626 line table, which now picks up a ONE_LINE1 entry covering
the real statement.

### Added

- `bytecode.LineTableEmpty` and `bytecode.LineTableSingleNoOp`,
  two small PEP 626 emitters covering the empty-module and
  single-no-op forms. Both are commented with the byte-level
  meaning of each field so the bytes can be cross-checked against
  CPython's `Objects/locations.md` directly.
- A classifier in `compiler` that recognises a single no-op
  statement (`pass`, `None`, `True`, `False`, `...`, integer /
  float / complex literal) on line 1, with optional trailing
  comments and trailing blank or comment-only lines.
- Seven new fixtures under `tests/fixtures/`: `002_pass.py`
  through `008_const_float.py`. Oracle byte-diff against
  `python3.14 -m py_compile` is zero on every one.

### Changed

- `compiler.ErrNotEmptyModule` is renamed to
  `compiler.ErrUnsupportedSource`. Returned for any module body
  the v0.0.x rungs have not yet learned to compile.

### Deferred

- Multi-statement bodies, string / bytes docstrings, and wiring
  `github.com/tamnd/gopapy/v1` as the parser. The gopapy module
  path is `/v1` but no `v1.x.x` tag exists yet, so consumption
  waits on a gopapy v1.0.0 cut.

## [0.0.1] - 2026-04-26

First public cut. `gocopy compile FILE.py` produces a CPython 3.14
`.pyc` that is byte-for-byte identical to
`python3.14 -m py_compile FILE.py` for an empty Python source file.

The point of v0.0.1 is the plumbing, not the language coverage. One
fixture (`tests/fixtures/001_empty.py`, zero bytes), one oracle
diff, every package wired end to end. Every feature the roadmap
lifts after this is a localised change rather than a re-bootstrap.

### Added

- `gocopy compile FILE.py [-o OUT.pyc]` CLI. Defaults the output
  path to `__pycache__/FILE.cpython-314.pyc` to match `py_compile`.
  `--mode timestamp` (default), `--mode hash`, and
  `--mode unchecked-hash` cover the three validation-field
  variants. `--source-date-epoch N` (and the `SOURCE_DATE_EPOCH`
  env var) override the source mtime baked into timestamp mode,
  for reproducible builds.
- `gocopy version` and `gocopy help`.
- `bytecode` package with the `RESUME`, `LOAD_CONST`, and
  `RETURN_VALUE` opcode constants the empty module needs, plus the
  full 256-entry inline-cache size table sourced from goipy's
  `op` package. goipy is the canonical opcode source in this
  ecosystem; gocopy follows it.
- `compiler.Compile` for empty modules. A bare whitespace-and-
  comments source counts as empty; anything else returns
  `compiler: v0.0.1 only supports empty modules`. v0.0.2 lifts
  that by wiring in the gopapy AST.
- `marshal.Marshal` covering the type tags an empty Code object
  actually uses: `TYPE_CODE`, `TYPE_NONE`, `TYPE_INT`,
  `TYPE_STRING`, `TYPE_SMALL_TUPLE`, `TYPE_SHORT_ASCII_INTERNED`,
  `TYPE_REF`. Two-pass writer mirrors CPython's `Py_REFCNT > 1`
  heuristic for `FLAG_REF` and back-references, since Go has no
  refcounts to copy directly.
- `pyc.WriteFile` with all three invalidation modes. Hash mode
  uses SipHash-1-3 with `k0 = magic_number_token`, `k1 = 0`,
  matching `_imp.source_hash` (verified against vectors observed
  on Python 3.14.4).
- CI: `go test ./...`, the oracle byte-diff against
  `python3.14 -m py_compile`, an informational stdlib-compile
  counter, and a Windows `go vet`/`go build` job.
- Release CI: tag-driven cross-platform builds for
  linux / darwin / windows × amd64 / arm64. Mirrors the
  gopapy/goipy flow.

### Deferred to the next release

Anything that isn't an empty module. v0.0.2 wires in the gopapy
AST and starts adding real top-level statements.

[Unreleased]: https://github.com/tamnd/gocopy/compare/v0.0.4...HEAD
[0.0.4]: https://github.com/tamnd/gocopy/releases/tag/v0.0.4
[0.0.3]: https://github.com/tamnd/gocopy/releases/tag/v0.0.3
[0.0.2]: https://github.com/tamnd/gocopy/releases/tag/v0.0.2
[0.0.1]: https://github.com/tamnd/gocopy/releases/tag/v0.0.1
