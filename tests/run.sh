#!/bin/sh
# Oracle test: byte-diff every fixture's gocopy output vs python3.14 -m py_compile.
#
# Usage:
#   tests/run.sh                              # run every fixture
#   tests/run.sh tests/fixtures/foo/123_x.py  # run one fixture (verbose)
#
# In single-fixture mode the script always prints both the expected and
# actual disassembly side-by-side, even when the byte comparison passes
# — useful for inspecting opcode sequences while iterating on a shape.
#
# We use TIMESTAMP mode so the validation field comes from the source file's
# mtime + size on disk (not a hash). To keep the test deterministic we set
# the fixture's mtime to a fixed point before invoking either compiler.
#
# SOURCE_DATE_EPOCH must NOT be set: when it is, py_compile switches to
# UNCHECKED_HASH mode and the validation field becomes a SipHash, defeating
# the purpose of the touch step here.

set -eu

cd "$(dirname "$0")/.."

if ! command -v python3.14 >/dev/null 2>&1; then
    echo "tests/run.sh: python3.14 not on PATH; skipping" >&2
    exit 0
fi

unset SOURCE_DATE_EPOCH

go build -o bin/gocopy ./cmd/gocopy

# Fixed mtime: 2020-01-01 00:00:00 UTC. Locale-independent via -d on GNU
# touch, fall back to BSD -t on macOS.
fix_mtime() {
    if touch -d '2020-01-01T00:00:00Z' "$1" 2>/dev/null; then
        return
    fi
    touch -t '202001010000.00' "$1"
}

fail=0
total=0
verbose=0

# run_oracle <fixture> <pycache_dir>
#   Touches the fixture to a fixed mtime, runs python3.14 -m py_compile
#   to populate <pycache_dir>/__pycache__/, runs gocopy on the same
#   source, byte-compares the two .pyc outputs.
#
#   When $verbose=1 the helper prints the expected disassembly (and the
#   actual disassembly when gocopy succeeds, plus a unified diff between
#   them) regardless of whether the byte comparison passes.
run_oracle() {
    src="$1"
    pycache_dir="$2"
    total=$((total + 1))
    fix_mtime "$src"

    rm -rf "$pycache_dir/__pycache__"
    python3.14 -m py_compile "$src"
    expected="$pycache_dir/__pycache__/$(basename "$src" .py).cpython-314.pyc"

    actual="$(mktemp -t gocopy.XXXXXX)"
    gocopy_log="$(mktemp -t gocopy.log.XXXXXX)"
    # Don't let `set -e` bail on a gocopy compile failure — we want to
    # count it as a fixture failure and continue. Fixture corpus expansion
    # (spec 1574) deliberately includes fixtures that current visitor
    # arms reject; the harness must report all of them in one run, not
    # exit at the first.
    if ! bin/gocopy compile "$src" -o "$actual" >"$gocopy_log" 2>&1; then
        echo "FAIL $src (gocopy compile error)"
        fail=$((fail + 1))
        if [ "$verbose" -eq 1 ]; then
            print_source "$src"
            print_dis "expected" "$expected"
            echo "  | --- gocopy compile output ---"
            sed 's/^/  | /' "$gocopy_log"
        fi
        rm -f "$actual" "$gocopy_log"
        return 0
    fi
    rm -f "$gocopy_log"

    if cmp "$expected" "$actual" >/dev/null; then
        echo "ok   $src"
        if [ "$verbose" -eq 1 ]; then
            print_source "$src"
            print_dis "bytecode (matches expected)" "$expected"
        fi
    else
        echo "FAIL $src"
        fail=$((fail + 1))
        if [ "$verbose" -eq 1 ]; then
            print_source "$src"
            print_dis "expected" "$expected"
            print_dis "actual"   "$actual"
            echo "  | --- diff (expected vs actual) ---"
            diff_dis "$expected" "$actual"
        else
            diff_dis "$expected" "$actual"
        fi
    fi
    rm -f "$actual"
}

# print_source <fixture-path>
#   Prints the fixture's source code with line numbers and a `  | `
#   indent so it lines up with the dis output below it.
print_source() {
    echo "  | --- source ($1) ---"
    awk '{ printf "  | %3d  %s\n", NR, $0 }' "$1"
}

# print_dis <label> <pyc-path>
#   Disassembles a pyc file via tests/scripts/dis_pyc.py and prints it
#   under a labelled header. Memory addresses are normalised so
#   reruns produce identical output.
print_dis() {
    echo "  | --- $1 ---"
    python3.14 tests/scripts/dis_pyc.py "$2" 2>&1 |
        sed 's/at 0x[0-9a-f]*/at 0xADDR/g' |
        sed 's/^/  | /'
}

# diff_dis <expected.pyc> <actual.pyc>
#   Disassembles both pyc files via tests/scripts/dis_pyc.py and prints
#   a unified diff of the listings. Memory addresses (`at 0xNNN`) are
#   stripped since they vary by load address and are not the
#   divergence we care about. Output is indented for readability and
#   (in batch mode) capped at 80 lines so a single failing fixture
#   cannot drown the summary; in verbose mode the cap is removed.
diff_dis() {
    exp_dis="$(mktemp -t gocopy.exp.XXXXXX)"
    act_dis="$(mktemp -t gocopy.act.XXXXXX)"
    python3.14 tests/scripts/dis_pyc.py "$1" 2>&1 |
        sed 's/at 0x[0-9a-f]*/at 0xADDR/g' >"$exp_dis"
    python3.14 tests/scripts/dis_pyc.py "$2" 2>&1 |
        sed 's/at 0x[0-9a-f]*/at 0xADDR/g' >"$act_dis"
    if [ "$verbose" -eq 1 ]; then
        diff -u --label expected --label actual "$exp_dis" "$act_dis" |
            sed 's/^/  | /'
    else
        diff -u --label expected --label actual "$exp_dis" "$act_dis" |
            head -80 |
            sed 's/^/  | /'
    fi
    rm -f "$exp_dis" "$act_dis"
}

if [ "$#" -ge 1 ]; then
    # Single-fixture verbose mode. The argument is a path to a .py
    # fixture (relative to the repo root or absolute).
    src="$1"
    case "$src" in
        /*) ;;
        *)  src="$(pwd)/$src" ;;
    esac
    if [ ! -f "$src" ]; then
        echo "tests/run.sh: fixture not found: $1" >&2
        exit 2
    fi
    verbose=1
    run_oracle "$src" "$(dirname "$src")"
    rm -rf "$(dirname "$src")/__pycache__"
    exit "$fail"
fi

# Top-level fixtures.
for src in tests/fixtures/*.py; do
    run_oracle "$src" "tests/fixtures"
done

# v0.7.10.x funcbody fixtures (flat second pass; spec 1559).
for src in tests/fixtures/funcbody/*.py; do
    run_oracle "$src" "tests/fixtures/funcbody"
done

rm -rf tests/fixtures/__pycache__ tests/fixtures/funcbody/__pycache__

echo "---"
echo "$((total - fail))/$total fixtures byte-identical"
[ "$fail" -eq 0 ]
