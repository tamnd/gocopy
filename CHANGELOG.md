# Changelog

All notable changes to gocopy are recorded here. The format follows
[Keep a Changelog 1.1](https://keepachangelog.com/en/1.1.0/). Once
gocopy reaches 1.0 the project will follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html); until
then, expect minor version bumps to sometimes include breaking
changes.

## [Unreleased]

## [0.0.11] - 2026-04-26

`gocopy compile` accepts a leading `-` on the right-hand side of a
`name = literal` assignment, for negative integer and float constants.

CPython's constant folder keeps both the original un-negated literal
and the folded negative in the consts tuple: `(N, None, -N)` with
`LOAD_CONST 2` pointing at the negated value. Immortal-range rules
apply: integers in [-5, -1] get FLAG_REF; below -5 they do not.

### Added

- `negLiteral{pos, neg any}` in `compiler/classify.go`.
- Negative-prefix detection in `tryParseAssign` (checked before the
  positive-float parser, which also accepts a leading `-`).
- `bytecode.AssignBytecodeAt(valueIdx, noneIdx, tailStmts)`: the
  general form of `AssignBytecode` with an explicit value const index.
- Five new fixtures: `053_assign_neg_one.py` through
  `057_assign_neg_float_sci.py`.

### Deferred

- `-0` (integer negative zero).
- Complex literal RHS (`x = 1j`).
- Multiple sequential assignments.
- Wiring gopapy as the parser.

## [0.0.10] - 2026-04-26

`gocopy compile` accepts a float literal on the right-hand side of a
leading `name = literal` assignment. Any value `strconv.ParseFloat`
accepts is valid — `1.0`, `3.14`, `0.0`, `1e100`, `1_000.5` — except
complex literals (trailing `j`/`J` are rejected).

CPython always uses `LOAD_CONST 0` for floats (no `LOAD_SMALL_INT`
path). `consts = (float_val, None)`. Marshal emits `TYPE_BINARY_FLOAT`
(0x67): 8 bytes, IEEE 754 double-precision, little-endian. Floats are
not immortal in CPython 3.14, so `FLAG_REF` is set only when the same
float appears more than once in the const-walk (never in the plain
assignment case).

### Added

- `marshal.emitObject` handles `float64` → `TYPE_BINARY_FLOAT` with
  an 8-byte little-endian double payload.
- `float64KeyType` / `float64Key()` in `marshal/writer.go`; `tupleKey`
  and `refCounter.tuple` updated to track `float64` consts.
- `parseFloatLiteral` in `compiler/classify.go`: accepts tokens with
  `.`, `e`, or `E`; rejects complex suffixes; strips underscores.
- Five new fixtures: `048_assign_float_one.py` through
  `052_assign_float_then_pass.py`.

### Deferred

- Complex literals (`1j`, `2.5j`).
- Negative float literals (unary minus not yet parsed).
- Docstring + assignment combo.
- Wiring gopapy as the parser.

## [0.0.9] - 2026-04-26

`gocopy compile` accepts a non-negative integer literal on the
right-hand side of a leading `name = literal` assignment. Decimal,
hex (`0x`), octal (`0o`), and binary (`0b`) literals, with optional
underscore separators, are all recognised. Values must fit in `int32`
[0, 2^31-1].

CPython uses `LOAD_SMALL_INT <val>` for 0..255 (value in oparg) and
`LOAD_CONST 0` for 256+. Both paths use `consts = (int_val, None)`.
Marshal emits `TYPE_INT` (0x69); integers in CPython's immortal
small-int cache range [−5, 256] get `FLAG_REF`, larger ones do not
(they appear only once in the walk).

### Added

- `LOAD_SMALL_INT Opcode = 94` in `bytecode`.
- `bytecode.AssignSmallIntBytecode(val, tailStmts)` for the 0..255
  path.
- `marshal.emitObject` handles `int64` with the immortal-range
  FLAG_REF rule.
- `parseIntLiteral` / `parseBaseLiteral` in `compiler/classify.go`.
- Eight new fixtures: `040_assign_int_zero.py` through
  `047_assign_int_hex_large.py`.

### Deferred

- Negative integer literals, integers > 2^31-1, floats, complex.
- Docstring + assignment combo.
- Wiring gopapy as the parser.

## [0.0.8] - 2026-04-26

`gocopy compile` accepts two more right-hand sides on a leading
`name = literal` assignment: the `...` literal (Python's `Ellipsis`
singleton) and a plain-ASCII bytes literal (`b"hi"`, `b""`). The
bytecode shape is identical to v0.0.7's assignment lowering; only
the marshal layer learns two new const types.

`Ellipsis` marshals as a single `TYPE_ELLIPSIS` (`0x2e`) byte with
no `FLAG_REF`. Bytes consts route through the existing
`TYPE_STRING` bytestring path, which already handles the empty-bytes
singleton (always `FLAG_REF`, dedups with empty `localspluskinds` /
`exctable`) and the non-empty case (no `FLAG_REF` unless the same
content appears twice in the walk).

### Added

- `bytecode.Ellipsis` and `bytecode.EllipsisType`: the gocopy
  sentinel for Python's `Ellipsis`.
- `marshal.emitObject` cases for `[]byte` and
  `bytecode.EllipsisType`, plus the matching `tupleKey` and
  `refCounter` recursion so the empty-bytes singleton dedups
  correctly when a const is also `b""`.
- `tryParseAssign` accepts `...` and `b"..."` on the right-hand
  side. Identifier rules are unchanged.
- Five new fixtures: `035_assign_ellipsis.py` through
  `039_assign_ellipsis_then_pass.py`.

### Deferred

- Right-hand side integer / float / complex literals.
- Docstring + assignment combo (needs a wider bytecode shape).
- Non-ASCII bytes (parser still rejects backslashes).
- Multi-target / augmented / expression assignment.
- Wiring gopapy as the parser; still waiting on a gopapy v1.0.0.

## [0.0.7] - 2026-04-26

`gocopy compile` accepts a leading `name = literal` assignment where
literal is `None`, `True`, `False`, or a plain-ASCII string literal,
optionally followed by the same no-op tail v0.0.4..0.0.6 already
accepted. Output stays byte-identical to `python3.14 -m py_compile`.

CPython's lowering:

    RESUME 0
    LOAD_CONST <value>
    STORE_NAME <name>
    [no-op tail]
    LOAD_CONST <None>
    RETURN_VALUE

`consts` is `(value, None)` when value is non-None and `(None,)`
when value is None itself. `names` is `(name,)` — the first
non-empty names tuple gocopy emits.

The line table gains a new SHORT0 entry primitive for the
STORE_NAME slot. SHORT0 encodes (start_col, end_col) in one payload
byte; CPython uses it for STORE_NAME because the target name span
always fits.

### Added

- `bytecode.AssignBytecode` and `bytecode.AssignLineTable` for the
  assignment shape.
- A `name = literal` parser branch in `compiler.classify`.
- Six new fixtures: `029_assign_none.py` through
  `034_assign_long_name.py`.

### Changed

- `compiler.classify` returns a fourth body shape `modAssign`.

### Deferred

- Right-hand side `Ellipsis`, bytes literals, integers, floats.
- Multi-target / augmented / expression assignment.
- Assignment after a leading docstring or after other statements.
- Wiring gopapy as the parser; still waiting on a gopapy v1.0.0.

## [0.0.6] - 2026-04-26

`gocopy compile` accepts triple-quoted docstrings that span multiple
source lines. v0.0.5 only handled single-line docstrings; this rung
adds the LONG line-table entry CPython emits when the docstring's
end line differs from its start line, plus the matching marshal
change so non-identifier-shaped string consts come out un-interned.

The PEP 626 LONG payload for a multi-line statement at column 0:

    header: 0xf0..0xf3 (LONG, length 1..4 code units)
    svarint(line_delta)
    varint(end_line_delta)
    varint(start_col + 1)
    varint(end_col + 1)

The trailing tail of t no-op statements after a multi-line docstring
keeps the v0.0.5 rule (each entry's line delta is computed from the
previous statement's start line, not its end line) and still adds
`max(0, t-1)` NOPs.

The marshal change tracks CPython's `intern_string_constants`: a
string const is emitted as `TYPE_SHORT_ASCII_INTERNED | FLAG_REF`
only when every byte is ASCII alphanumeric or underscore. Anything
with a space, newline, or punctuation goes out as plain
`TYPE_SHORT_ASCII` with no ref flag.

### Added

- `bytecode.DocstringLineTable` takes a `docEndLine` parameter and
  emits a LONG entry when the docstring spans multiple lines.
- A multi-line triple-quoted-string scanner in `compiler.classify`
  for plain-ASCII bodies across many source lines, honouring the
  optional `b`/`B` bytes prefix.
- Three new fixtures: `026_docstring_multi.py`,
  `027_docstring_three_line.py`, and
  `028_docstring_multi_with_tail.py`.

### Changed

- `marshal` decides string-const interning per byte instead of always
  interning. Empty strings are also non-interned now.
- `compiler.classify` records `docEndLine` alongside the docstring
  text so the bytecode layer can pick the right line-table entry.

### Deferred

- Backslash escapes inside string literals.
- Triple-quoted docstrings whose body contains the matching quote
  character or a `#` that should not be treated as a comment.
- Module-level assignments and other expression statements.
- Raw, f-, and t-strings, plus the remaining prefix combos.
- Non-ASCII docstring contents and strings longer than 255 bytes.
- Wiring gopapy as the parser; still waiting on a gopapy v1.0.0.

## [0.0.5] - 2026-04-26

`gocopy compile` accepts a leading single-line ASCII string literal
as the module docstring, plus string and bytes literals as no-op
statements anywhere else. Output still matches `python3.14 -m
py_compile` byte-for-byte.

CPython lowers a leading string literal to:

    RESUME 0
    LOAD_CONST <docstring>
    STORE_NAME __doc__
    [trailing-tail body]
    LOAD_CONST None
    RETURN_VALUE

`consts` becomes `(docstring, None)` and `names` becomes
`('__doc__',)`. The docstring's line table entry covers length 4
when the docstring is the only statement and length 2 otherwise; the
trailing tail of t no-op statements adds `max(0, t-1)` NOPs because
the last tail statement's line entry absorbs the implicit
`LOAD_CONST None / RETURN_VALUE` pair.

Bytes literals and any non-leading string literal are no-ops:
CPython drops the value and the body collapses to the v0.0.4 no-op
shape.

### Added

- `bytecode.STORE_NAME` opcode (CPython 3.14 opcode 116).
- `bytecode.DocstringBytecode(t)` and `bytecode.DocstringLineTable(
  docLine, docCol, tail)`.
- A string-literal scanner: single, double, and triple-quoted ASCII
  literals with no backslash escapes and no embedded matching
  quote, plus the same shapes prefixed with `b` or `B`.
- `marshal.emitObject` learns the `string` case (encoded as
  `TYPE_SHORT_ASCII_INTERNED | FLAG_REF`).
- Seven new fixtures: `019_docstring.py` through
  `025_docstring_two_tail.py`.

### Changed

- `compiler.classify` returns three shapes (`modEmpty`, `modNoOps`,
  `modDocstring`); the docstring shape carries the docstring text
  and the no-op tail.
- `compiler.module` takes the consts and names tuples as
  parameters so the docstring shape can supply its own
  `(docstring, None)` and `('__doc__',)`.

### Deferred

- Backslash escapes inside string literals.
- Triple-quoted docstrings spanning multiple source lines.
- Raw, f-, and t-strings, plus the remaining prefix combos.
- Non-ASCII docstring contents and strings longer than 255 bytes.
- Wiring gopapy as the parser; still waiting on a gopapy v1.0.0.

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

[Unreleased]: https://github.com/tamnd/gocopy/compare/v0.0.11...HEAD
[0.0.11]: https://github.com/tamnd/gocopy/compare/v0.0.10...v0.0.11
[0.0.10]: https://github.com/tamnd/gocopy/compare/v0.0.9...v0.0.10
[0.0.9]: https://github.com/tamnd/gocopy/releases/tag/v0.0.9
[0.0.8]: https://github.com/tamnd/gocopy/releases/tag/v0.0.8
[0.0.7]: https://github.com/tamnd/gocopy/releases/tag/v0.0.7
[0.0.6]: https://github.com/tamnd/gocopy/releases/tag/v0.0.6
[0.0.5]: https://github.com/tamnd/gocopy/releases/tag/v0.0.5
[0.0.4]: https://github.com/tamnd/gocopy/releases/tag/v0.0.4
[0.0.3]: https://github.com/tamnd/gocopy/releases/tag/v0.0.3
[0.0.2]: https://github.com/tamnd/gocopy/releases/tag/v0.0.2
[0.0.1]: https://github.com/tamnd/gocopy/releases/tag/v0.0.1
