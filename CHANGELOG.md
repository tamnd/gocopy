# Changelog

All notable changes to gocopy are recorded here. The format follows
[Keep a Changelog 1.1](https://keepachangelog.com/en/1.1.0/). Once
gocopy reaches 1.0 the project will follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html); until
then, expect minor version bumps to sometimes include breaking
changes.

## [Unreleased]

## [0.1.0] - 2026-04-26

Initial release. `gocopy compile` produces a CPython 3.14 `.pyc`
that is byte-for-byte identical to `python3.14 -m py_compile` for an
empty Python source file. Establishes the parser → compiler →
marshal → pyc pipeline, the oracle test harness, and the
multi-platform release flow.

The fixture set is one file: `tests/fixtures/empty.py` (zero
bytes). The CI oracle job byte-diffs gocopy's output against
`python3.14 -m py_compile`'s output of the same fixture and fails
on any difference. Subsequent releases extend the fixture set one
language feature at a time per the roadmap in
`notes/Spec/1100/1158_gocopy_roadmap.md`.

### Added

- `gocopy compile FILE.py [-o OUT.pyc]` CLI. Defaults the output
  path to `__pycache__/FILE.cpython-314.pyc` to mirror
  `py_compile`. Supports `--mode timestamp` (default),
  `--mode hash`, and `--mode unchecked-hash` for the validation
  field.
- `gocopy version` prints the gocopy version string.
- `bytecode` package with the `RESUME`, `LOAD_CONST`, and
  `RETURN_VALUE` opcode constants the empty module needs, plus the
  full 256-entry inline-cache size table sourced from
  goipy's `op` package (the canonical opcode source in this
  ecosystem).
- `compiler.Compile` for empty modules. Non-empty modules return
  `compiler: v0.1.0 only supports empty modules`. v0.1.1 lifts that.
- `marshal.Marshal` covering the type tags an empty Code object
  uses (`TYPE_CODE`, `TYPE_NONE`, `TYPE_INT`, `TYPE_STRING`,
  `TYPE_SMALL_TUPLE`, `TYPE_SHORT_ASCII_INTERNED`, `TYPE_REF`).
  Two-pass writer correctly stamps `FLAG_REF` on objects that
  appear more than once in the stream and emits `TYPE_REF` for
  back-references, matching CPython's `Py_REFCNT > 1` heuristic.
- `pyc.WriteFile` with timestamp, checked-hash, and unchecked-hash
  invalidation modes. Hash mode uses SipHash-1-3 with `k0 =
  magic_number_token` and `k1 = 0`, matching CPython's
  `_imp.source_hash`.
- CI: `go test ./...` + oracle byte-diff against
  `python3.14 -m py_compile` on linux/macOS, plus a
  cross-platform `go vet` on windows.
- Release CI: tag-driven cross-platform builds for
  linux/darwin/windows × amd64/arm64. Mirrors the gopapy/goipy
  release flow.

### Known limitations

- Non-empty Python source returns an explicit error. The roadmap
  in `notes/Spec/1100/1158_gocopy_roadmap.md` lifts this one
  feature at a time across v0.1.1 → v0.1.23.
