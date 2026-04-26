# Changelog

All notable changes to gocopy are recorded here. The format follows
[Keep a Changelog 1.1](https://keepachangelog.com/en/1.1.0/). Once
gocopy reaches 1.0 the project will follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html); until
then, expect minor version bumps to sometimes include breaking
changes.

## [Unreleased]

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

[Unreleased]: https://github.com/tamnd/gocopy/compare/v0.0.1...HEAD
[0.0.1]: https://github.com/tamnd/gocopy/releases/tag/v0.0.1
