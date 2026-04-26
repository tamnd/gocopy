# Coverage

Per-feature compile status. Each row is a Python AST node or
construct. A row is `done` once gocopy emits byte-identical output
for at least one fixture exercising that node and the
`tests/run.sh` oracle diff stays at zero on the per-PR run.

| Feature | Status | Spec | Fixture |
|---|---|---|---|
| Empty module | done | 1159 | tests/fixtures/empty.py |
| Constant expression statement (None / True / False / int / float / str / bytes) | planned | 11xx | - |
| Module-level assignment | planned | 11xx | - |
| Arithmetic and unary | planned | 11xx | - |
| Comparisons and chains | planned | 11xx | - |
| Boolean ops and ternary | planned | 11xx | - |
| Sequence and dict literals | planned | 11xx | - |
| Subscript and attribute | planned | 11xx | - |
| Function calls | planned | 11xx | - |
| if / elif / else | planned | 11xx | - |
| while loops | planned | 11xx | - |
| for loops | planned | 11xx | - |
| Function definitions | planned | 11xx | - |
| Closures | planned | 11xx | - |
| Class definitions | planned | 11xx | - |
| Comprehensions | planned | 11xx | - |
| try / except / finally | planned | 11xx | - |
| with / async with | planned | 11xx | - |
| match / case | planned | 11xx | - |
| Generators / async | planned | 11xx | - |
| f-strings / t-strings | planned | 11xx | - |
| import / from-import | planned | 11xx | - |
| global / nonlocal / del / raise / assert | planned | 11xx | - |
| Type aliases / type parameters (PEP 695) | planned | 11xx | - |
| Constant folder | planned | 11xx | - |
| Jump-threading | planned | 11xx | - |
| Exception-table compaction | planned | 11xx | - |
| Line-table compaction | planned | 11xx | - |
| `gocopy compileall` | planned | 11xx | - |
| `gocopy dis` | planned | 11xx | - |

The full rung-by-rung plan lives in `notes/Spec/1100/1158_gocopy_roadmap.md`.
