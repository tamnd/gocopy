# Roadmap

The goal is byte-identical `.pyc` output for every `.py` file in the
CPython 3.14 standard library. We get there one construct at a time,
keeping `tests/run.sh` at zero diff throughout.

## Where we are

v0.1.20, 161 fixtures. The compiler handles:

- Module-level: empty modules, pass, no-ops, docstrings, literal
  assignments (all types), augmented assignments, arithmetic/unary/
  compare/boolean/ternary expressions, collection literals, subscript,
  attribute, function calls, if/elif/else, while, for, function
  definitions (simple bodies), closures.
- Function bodies (specialized path): params + local assignments,
  return with name/constant/binop/compare/augassign/not.

## Milestones

### v0.2 - function calls in function bodies

Adds CALL opcode inside `compileFuncBodyExpr`. A call like
`return len(a)` or `return max(a, b)` emits PUSH_NULL + LOAD_GLOBAL +
LOAD_FAST_BORROW... + CALL. The global name list for the inner code
object must be built correctly. Attribute calls (`a.method()`) and
subscript calls (`a[0]()`) follow in sub-releases.

Releases: function calls with simple positional args, keyword args,
`*args`/`**kwargs` forwarding, attribute calls, subscript calls.

### v0.3 - control flow in function bodies

Adds if/elif/else, while, and for inside function bodies. Requires a
label/patch mechanism for forward jumps (already exists at module level;
needs to be wired into the func-body path). After this milestone a
function like:

```python
def clamp(v, lo, hi):
    if v < lo:
        return lo
    if v > hi:
        return hi
    return v
```

compiles correctly.

### v0.4 - complete function signatures

Default argument values, keyword-only parameters, `*args`, `**kwargs`,
positional-only separator `/`, PEP 695 type parameter syntax, and
function annotations. After this milestone any top-level function in
stdlib that does not reference class machinery compiles.

### v0.5 - class definitions

`class C:` bodies, method definitions, `super()`, inheritance,
`__slots__`, class variables, properties, static/class methods, and
`__init_subclass__`. This is the largest milestone; the class body
needs its own scope in the symbol table.

### v0.6 - exception handling

`try/except/finally/else`, `raise`, `raise from`, bare `raise`,
`assert`, and the exception-table compaction pass. After this milestone
most of the `exceptions.py`, `contextlib.py`, and `abc.py` modules
compile.

### v0.7 - context managers and comprehensions

`with` and `async with`, list/set/dict comprehensions, generator
expressions, `yield`, `yield from`, and `async for`. Comprehensions
create nested code objects and need their own scope.

### v0.8 - async/await

`async def`, `await`, `async for`, `async with`. Adds CO_COROUTINE and
CO_ASYNC_GENERATOR flags to the code-object builder.

### v0.9 - import system

`import foo`, `from foo import bar`, relative imports, `__future__`
imports, `importlib` helpers. IMPORT_NAME and IMPORT_FROM opcodes.

### v1.0 - remaining language

f-strings (FORMAT_VALUE / BUILD_STRING / CONVERT_VALUE),
t-strings (PEP 750), `match/case` (MATCH_* opcodes), `global`,
`nonlocal`, `del`, type aliases (PEP 695), walrus operator (`:=`),
and the optimizer passes (constant folding, jump threading, dead-code
elimination).

### v1.1 - full stdlib corpus

`gocopy compileall` over the entire CPython 3.14 Lib/ tree with zero
diff against a CPython-compiled run. Requires all of v0.2-v1.0 plus
any edge cases the corpus uncovers.

## Fixture count targets

| Version | Fixtures | New constructs |
|---------|----------|----------------|
| v0.1.20 | 161 | not operator in func bodies |
| v0.2.x | ~200 | function calls in func bodies |
| v0.3.x | ~250 | control flow in func bodies |
| v0.4.x | ~300 | full function signatures |
| v0.5.x | ~400 | class definitions |
| v0.6.x | ~460 | exception handling |
| v0.7.x | ~530 | context managers, comprehensions |
| v0.8.x | ~580 | async/await |
| v0.9.x | ~630 | imports |
| v1.0.x | ~700 | remaining language |
| v1.1.x | 1000+ | full stdlib corpus |

## Working agreement

- One PR per version. Squash-merge.
- Every PR adds at least one fixture whose oracle diff goes from
  non-zero to zero.
- The oracle (`tests/run.sh`) must stay at zero diff at every release.
- Commit messages, changelogs, and this doc: human voice, no AI
  register, no em-dashes.
