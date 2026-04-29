#!/bin/sh
# Oracle test: byte-diff every fixture's gocopy output vs python3.14 -m py_compile.
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

# run_oracle <fixture> <pycache_dir>
#   Touches the fixture to a fixed mtime, runs python3.14 -m py_compile
#   to populate <pycache_dir>/__pycache__/, runs gocopy on the same
#   source, byte-compares the two .pyc outputs.
run_oracle() {
    src="$1"
    pycache_dir="$2"
    total=$((total + 1))
    fix_mtime "$src"

    rm -rf "$pycache_dir/__pycache__"
    python3.14 -m py_compile "$src"
    expected="$pycache_dir/__pycache__/$(basename "$src" .py).cpython-314.pyc"

    actual="$(mktemp -t gocopy.XXXXXX)"
    bin/gocopy compile "$src" -o "$actual"

    if cmp "$expected" "$actual" >/dev/null; then
        echo "ok   $src"
    else
        echo "FAIL $src"
        echo "  expected:"
        xxd "$expected" | sed 's/^/    /'
        echo "  actual:"
        xxd "$actual" | sed 's/^/    /'
        fail=$((fail + 1))
    fi
    rm -f "$actual"
}

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
