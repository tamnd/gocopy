<h1 align="center">gocopy</h1>

<p align="center">
  <b>Pure-Go compiler from Python 3.14 source to a CPython-compatible <code>.pyc</code>.</b><br>
  <sub>Built on <a href="https://github.com/tamnd/gopapy">gopapy</a> for parsing. No CPython at runtime.</sub>
</p>

---

`gocopy` reads Python 3.14 source and writes a `.pyc` that is byte-for-byte
identical to the output of `python3.14 -m py_compile`. Same magic, same flags,
same validation field, same marshal stream, same constant ordering, same line
table, same exception table.

The parser is `github.com/tamnd/gopapy`, which is already 100% AST-compatible
with CPython 3.14, so gocopy never has to second-guess the AST it consumed.
The marshal writer is the inverse of [goipy](https://github.com/tamnd/goipy)'s
reader: same wire format, opposite direction.

This is the bootstrap branch. Track scope and progress in
[`docs/COVERAGE.md`](docs/COVERAGE.md). For a tour of the pipeline see
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). The version-by-version plan
lives in [`notes/Spec/1100/1158_gocopy_roadmap.md`](https://github.com/tamnd/notes).

## Quick start

```sh
go build ./cmd/gocopy
: > /tmp/empty.py
./gocopy compile /tmp/empty.py -o /tmp/empty.pyc
python3.14 -c "import marshal, struct
with open('/tmp/empty.pyc','rb') as f: f.read(16); print(marshal.loads(f.read()))"
# <code object <module> at 0x..., file "/tmp/empty.py", line 1>
```

`gocopy compile FILE.py` defaults to `__pycache__/FILE.cpython-314.pyc`,
matching CPython's `py_compile`.

## Stability

Library API and CLI flags are not frozen until v0.2.0. After v0.2.0:

- **CLI surface stable.** `gocopy compile FILE.py [-o OUT.pyc]
  [--mode timestamp|hash|unchecked-hash] [--source-date-epoch N]`.
- **Library entry points stable.** `compiler.Compile`, `marshal.Marshal`,
  `pyc.WriteFile`, `bytecode.CodeObject`'s exported field set.
- **Module path is `github.com/tamnd/gocopy/v1`.** Future breaking changes
  move to `/v2`, so the import path itself enforces the contract.

Internal helpers under `internal/` are exempt and may move freely.

## Tests

```sh
go test ./...        # unit tests across bytecode, compiler, marshal, pyc
tests/run.sh         # end-to-end byte-diff against python3.14 -m py_compile
```

`tests/run.sh` requires `python3.14` on PATH; it diffs gocopy's output
against `python3.14 -m py_compile`'s output for every fixture under
`tests/fixtures/`.

## License

MIT. See [LICENSE](LICENSE).
