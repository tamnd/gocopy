# Coverage

Per-feature compile status. Each row is a Python AST node or
construct. A row is `done` once gocopy emits byte-identical output
for at least one fixture exercising that node and the
`tests/run.sh` oracle diff stays at zero on the per-PR run.

| Feature | Status | Version | Fixture |
|---|---|---|---|
| Empty module | done | v0.0.1 | tests/fixtures/001_empty.py |
| `pass` statement (single, line 1) | done | v0.0.2 | tests/fixtures/002_pass.py |
| Bare non-string constant expression statement (single, line 1) | done | v0.0.2 | tests/fixtures/003_const_none.py |
| Constant expression statement: string / bytes (docstring path) | planned | - | - |
| Multi-statement no-op bodies (`pass\npass`, etc.) | planned | - | - |
| Module-level assignment | planned | - | - |
| Arithmetic and unary | planned | - | - |
| Comparisons and chains | planned | - | - |
| Boolean ops and ternary | planned | - | - |
| Sequence and dict literals | planned | - | - |
| Subscript and attribute | planned | - | - |
| Function calls | planned | - | - |
| if / elif / else | planned | - | - |
| while loops | planned | - | - |
| for loops | planned | - | - |
| Function definitions | planned | - | - |
| Closures | planned | - | - |
| Class definitions | planned | - | - |
| Comprehensions | planned | - | - |
| try / except / finally | planned | - | - |
| with / async with | planned | - | - |
| match / case | planned | - | - |
| Generators / async | planned | - | - |
| f-strings / t-strings | planned | - | - |
| import / from-import | planned | - | - |
| global / nonlocal / del / raise / assert | planned | - | - |
| Type aliases / type parameters (PEP 695) | planned | - | - |
| Constant folder | planned | - | - |
| Jump-threading | planned | - | - |
| Exception-table compaction | planned | - | - |
| Line-table compaction | planned | - | - |
| `gocopy compileall` | planned | - | - |
| `gocopy dis` | planned | - | - |

The shipped plan lives in `CHANGELOG.md`; in-flight per-version
notes live under `changelog/v*.md`.
