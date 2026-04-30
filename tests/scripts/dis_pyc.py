#!/usr/bin/env python3.14
# Disassemble a .pyc file produced by py_compile / gocopy.
#
# Usage: dis_pyc.py <path/to/file.pyc>
#
# Strips the 16-byte CPython 3.14 pyc header (magic + flags +
# timestamp/hash + source size) and unmarshals the trailing code
# object, then prints `dis.dis` recursively. Used by tests/run.sh to
# render a human-readable diff when an oracle byte-comparison fails —
# raw hex is unhelpful, opcode listings expose the real divergence.
import dis
import marshal
import sys


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("usage: dis_pyc.py <path-to-pyc>", file=sys.stderr)
        return 2
    with open(argv[1], "rb") as f:
        f.read(16)
        code = marshal.load(f)
    dis.dis(code, depth=None, show_caches=False)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
