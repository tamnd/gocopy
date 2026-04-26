#!/bin/sh
# Try to compile every .py under PYDIR with gocopy and count byte-identical
# results vs python3.14 -m py_compile. Informational only, not a hard CI
# gate in v0.1.x because most stdlib files use language features the
# compiler does not yet support.
#
# Usage: ./scripts/stdlib-compile.sh /path/to/python3.14/stdlib

set -eu

PYDIR="${1:-}"
if [ -z "$PYDIR" ]; then
    echo "usage: $0 PYDIR" >&2
    exit 2
fi
if [ ! -d "$PYDIR" ]; then
    echo "$0: $PYDIR is not a directory" >&2
    exit 2
fi

GOCOPY="${GOCOPY:-./bin/gocopy}"
if [ ! -x "$GOCOPY" ]; then
    echo "$0: $GOCOPY not found; build with 'go build -o bin/gocopy ./cmd/gocopy'" >&2
    exit 2
fi

total=0
ok=0
fail=0
unsupported=0

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

while IFS= read -r src; do
    total=$((total + 1))
    actual="$scratch/out.pyc"

    if "$GOCOPY" compile "$src" -o "$actual" >/dev/null 2>&1; then
        rm -rf "$(dirname "$src")/__pycache__/$(basename "$src" .py).cpython-314.pyc" 2>/dev/null || true
        if python3.14 -m py_compile "$src" 2>/dev/null; then
            expected="$(dirname "$src")/__pycache__/$(basename "$src" .py).cpython-314.pyc"
            if [ -f "$expected" ] && cmp "$expected" "$actual" >/dev/null 2>&1; then
                ok=$((ok + 1))
            else
                fail=$((fail + 1))
            fi
        fi
    else
        unsupported=$((unsupported + 1))
    fi
done <<EOF
$(find "$PYDIR" -type f -name '*.py')
EOF

echo "stdlib-compile summary:"
echo "  total       : $total"
echo "  byte-equal  : $ok"
echo "  diff        : $fail"
echo "  unsupported : $unsupported"
