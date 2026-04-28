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
  │     ├── classifyAST          (classify.go / classify_ast.go)
  │     │     identify the module shape and match it to a compile path
  │     ├── specialized paths    (compiler.go, func_body.go)
  │     │     each path covers one narrow module/body shape
  │     ├── emit opcodes         (inline cache placeholders included)
  │     ├── linetable assembly   (PEP 626 compact form)
  │     └── exceptiontable       (PEP 657, placeholder for now)
  │
  ├── marshal.Marshal      →  []byte (CPython marshal stream)
  │     ├── two-pass: count refs, then emit with FLAG_REF / TYPE_REF
  │     └── small-form encoding (TYPE_SMALL_TUPLE / TYPE_SHORT_ASCII)
  │
  └── pyc.WriteFile        →  source.pyc (16-byte header + marshal stream)
```

## Packages

- `bytecode/`: opcode constants, inline-cache table (CacheSize[]),
  instruction representation, mutable CodeObject builder, linetable and
  exceptiontable encoders, co_flags constants.
- `compiler/`: AST walker; one entry point `Compile(*ast.Module,
  Options) (*bytecode.CodeObject, error)`. Internally splits by shape:
  see "Classifier" and "Compile paths" below.
- `marshal/`: `Marshal(*bytecode.CodeObject) ([]byte, error)`. Type-tag
  constants mirror goipy.
- `pyc/`: 16-byte header writer plus full-file `WriteFile`. Hash mode
  uses SipHash-1-3 to mirror CPython's `_imp.source_hash`.
- `cmd/gocopy/`: the CLI.

## Classifier

`classifyAST` (classify_ast.go) walks the gopapy AST and returns a
`classification` struct (classify.go) that names the module shape and
carries the pre-parsed data each compile path needs.

Shapes handled today:

| modKind constant | What it covers |
|---|---|
| modEmpty | empty file |
| modPass | single pass / no-op |
| modDocstring | single docstring |
| modMultiNoOp | N no-op statements |
| modAssign | module-level literal assignment(s) |
| modAugAssign | module-level augmented assignment |
| modFuncDef | lone function definition (simple body) |
| modFuncBodyExpr | function definition with body of local assignments + return |
| modIf | module-level if/elif/else |
| modWhile | module-level while loop |
| modFor | module-level for loop |
| modClosure | closure definition |
| modExpr | module-level expression statement |
| modReturn | (inside function bodies) |
| modCall | function call at module level |
| modSubscript | subscript at module level |

If the AST does not match any shape, the classifier returns
`modUnsupported` and the compiler falls back to an empty module (the
caller gets an error in strict mode).

## Compile paths

Each modKind maps to a single compile function in compiler.go or a
dedicated file:

- **compiler.go** handles most module-level shapes directly, calling
  expression helpers for the RHS.
- **func_body.go** handles `modFuncBodyExpr`: a function whose body is
  zero or more local-assignment statements followed by one return
  statement. Uses `compileFuncBodyExpr`.

### compileFuncBodyExpr

The specialized function-body compiler lives in func_body.go. It
manages its own `funcState` struct (slots map, bytecode buffer, linetable
buffer, constant list) separate from the module-level state.

Expression support in `walkExpr` (as of v0.3.8):

- `*parser2.Name` (local) - LOAD_FAST_BORROW(slot)
- `*parser2.Name` (global) - LOAD_GLOBAL oparg=(nameIdx<<1)|0 + 4 cache words
- `*parser2.Constant` (int 0-255, None, True, False) - LOAD_SMALL_INT
  or LOAD_CONST
- `*parser2.BinOp` - LOAD_FAST_BORROW_LOAD_FAST_BORROW superinstruction
  (when both operands are name nodes with slots 0-15) or general
  LOAD_FAST_BORROW + walkExpr; then BINARY_OP + 5 cache words
- `*parser2.Compare` (single op) - same two-path choice as BinOp but
  COMPARE_OP + 1 cache word
- `*parser2.UnaryOp` (USub, Invert) - walkExpr(operand) + UNARY_NEGATIVE
  or UNARY_INVERT
- `*parser2.UnaryOp` (Not, compare operand) - COMPARE_OP(oparg+16) +
  UNARY_NOT, no TO_BOOL
- `*parser2.UnaryOp` (Not, general operand) - walkExpr(operand) + TO_BOOL
  (3 cache words) + UNARY_NOT
- `*parser2.Attribute` - LOAD_FAST_BORROW(obj) + LOAD_ATTR (no method bit);
  9 cache words split into 8+2 CU linetable entries
- `*parser2.Subscript` - LFLBLFLB or LOAD_FAST_BORROW + walkExpr; then
  BINARY_OP(NbGetItem) + 5 cache words
- `*parser2.Tuple` - LFLBLFLB for first two Name elements, walkExpr for
  rest; then BUILD_TUPLE
- `*parser2.Call` (global fn) - LOAD_GLOBAL(NULL|fn) + args + CALL(nargs)
- `*parser2.Call` (method) - LOAD_FAST_BORROW(obj) + LOAD_ATTR(method bit) +
  args + CALL(nargs)

Statement support in the stmt loop (as of v0.3.8):

- Return with constant - LOAD_SMALL_INT/LOAD_CONST + RETURN_VALUE, one
  combined 2-CU linetable entry
- Return with expression - walkExpr + RETURN_VALUE, separate entries
- Return with ternary (`return Body if Test else OrElse`) - condition
  block + true branch RETURN_VALUE + false branch RETURN_VALUE
- if-return (`if cond: return expr`) - condition block + then RETURN_VALUE,
  POP_JUMP_IF_FALSE forward to fallthrough
- if/elif/else-return chains - multiple consecutive isIfReturn stmts,
  or final isIfReturn with orelse=[Return]
- if-assign (`if cond: target = expr`) - condition block + LOAD_FAST/
  walkExpr + STORE_FAST, POP_JUMP_IF_FALSE forward
- if-return / if-assign with `is None` / `is not None` condition -
  LOAD_FAST_BORROW + POP_JUMP_IF_NOT_NONE or POP_JUMP_IF_NONE (1 cache word)
  + NOT_TAKEN; integers not stored in co_consts for these functions
- float constants - LOAD_CONST idx (always in co_consts; no LOAD_SMALL_FLOAT)
- large int constants (> 255) - LOAD_CONST idx
- global name reads - LOAD_GLOBAL oparg=(nameIdx<<1)|0 + 4 cache words
- Plain assignment (name = expr) - LOAD_FAST for bare Name RHS; walkExpr
  for other expressions; then STORE_FAST
- Augmented assignment (name op= rhs) - LFLBLFLB or LOAD_FAST_BORROW +
  walkExpr(rhs), then BINARY_OP(NbInplace*) + STORE_FAST

## Linetable (PEP 626 compact form)

CPython 3.14 uses a two-byte-per-entry short format where each entry is:

```
byte 0: code = (startCol / 8) | (delta_lines << 3)
byte 1: localStart | (endCol - startCol) << 3
```

with ONE_LINE (3 bytes) and LONG (5 bytes) variants for larger spans.
The function-body compiler emits entries through `emit` (first on line)
and `emitSame` (subsequent on same line). `scanEndCol` extends column
spans past closing parentheses that the AST does not include.

## Inline caches

Every opcode that has inline caches (BINARY_OP, COMPARE_OP, TO_BOOL,
LOAD_GLOBAL, CALL, etc.) needs its cache slots emitted as zero-word
pairs. `bytecode.CacheSize[opcode]` gives the number of 2-byte cache
words for each opcode.

## co_consts rules

Module-level code objects: all literals appear in co_consts in
encounter order.

Function-body code objects: only the first integer constant is stored
in co_consts. Subsequent integer constants use LOAD_SMALL_INT without
a co_consts entry. None/True/False are stored in co_consts on first
occurrence. The `funcState.intConstSeen` flag tracks whether any
integer has been stored yet.

## Why bytes?

CPython's `.pyc` is a stable wire format read by every Python
interpreter on the planet. If gocopy emits something CPython cannot
import, it is broken. The byte-for-byte oracle (`tests/run.sh`) catches
that. Every PR must keep the oracle at zero diff for every fixture.

## Why two-pass marshal

CPython's marshal writer adds `FLAG_REF` to the type byte iff the
object has more than one reference. We do not have refcounts in Go, so
the writer makes one pass to count occurrences in the marshal walk
order, then a second pass to emit. Same byte sequence, different
mechanism.

## Working agreement

- One PR per version. Squash-merge.
- Every PR adds at least one `tests/fixtures/*.py` whose oracle diff
  goes from non-zero to zero.
- Voice: human, no AI register, no em-dashes.
