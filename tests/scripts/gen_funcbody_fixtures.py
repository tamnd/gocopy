#!/usr/bin/env python3.14
"""
Spec 1574 / v0.7.10.13: systematic + multi-stmt funcbody fixtures.

Run from repo root:
    python3.14 tests/scripts/gen_funcbody_fixtures.py

Idempotent: overwrites existing files in tests/fixtures/funcbody/ that
match the names this script produces. Hand-curated fixtures (001-058)
are not touched. Real-world fixtures live in fixtures/realworld/ and
are also not touched.

The matrix here is the minimum spanning set that prevents the visitor
from sliding back into a `validate*`-style narrow-shape allowlist:

  Axis A  body Stmt kind        14 arms
  Axis B  RHS / test Expr kind  17 arms
  Axis C  composition depth     0..3
  Axis D  body length           1..3
  Axis E  argument shape        10 variants
  Axis F  scope linkage         6 variants

We do not enumerate the full cartesian product (too sparse). We emit
one fixture per row that the validators historically rejected.
"""

import os
import sys
import textwrap

ROOT = os.path.join(os.path.dirname(__file__), "..", "fixtures", "funcbody")
ROOT = os.path.abspath(ROOT)


def write(name: str, body: str) -> None:
    path = os.path.join(ROOT, name)
    src = textwrap.dedent(body).lstrip("\n")
    if not src.endswith("\n"):
        src += "\n"
    with open(path, "w") as fp:
        fp.write(src)


# ---------------------------------------------------------------- 100-119
# Multi-stmt body fundamentals. Every fixture has body length 2 or 3 to
# catch the historical `len(Body) == 1` validator gates.

write("100_multi_two_assigns.py", """
    def f():
        x = 1
        y = 2
        return x + y
""")

write("101_multi_three_assigns.py", """
    def f():
        x = 1
        y = 2
        z = 3
        return x + y + z
""")

write("102_multi_assign_then_aug.py", """
    def f():
        x = 1
        x += 2
        return x
""")

write("103_multi_two_aug.py", """
    def f(a):
        a += 1
        a -= 1
        return a
""")

write("104_multi_assign_pass_return.py", """
    def f():
        x = 1
        pass
        return x
""")

write("105_multi_pass_assign.py", """
    def f():
        pass
        x = 0
        return x
""")

write("106_multi_three_pass.py", """
    def f():
        pass
        pass
        pass
""")

write("107_multi_global_assign_return.py", """
    g = 0


    def f():
        global g
        g = 1
        return g
""")

write("108_multi_annassign_assign.py", """
    def f():
        x: int = 1
        y = x
        return y
""")

write("109_multi_assign_annassign.py", """
    def f():
        x = 0
        y: int = x
        return y
""")

write("110_multi_assign_assert.py", """
    def f(x):
        y = x + 1
        assert y > 0
        return y
""")

write("111_multi_aug_aug_aug.py", """
    def f(a):
        a += 1
        a *= 2
        a -= 3
        return a
""")

write("112_multi_assign_if_return.py", """
    def f(x):
        y = x + 1
        if y > 0:
            return y
        return 0
""")

write("113_multi_if_assign_return.py", """
    def f(x):
        if x > 0:
            x = x + 1
        return x
""")

write("114_multi_if_else_assign.py", """
    def f(x):
        if x > 0:
            y = 1
        else:
            y = 2
        return y
""")

write("115_multi_assign_while.py", """
    def f(it):
        x = 0
        while it:
            x = x + 1
        return x
""")

write("116_multi_assign_for.py", """
    def f(seq):
        s = 0
        for x in seq:
            s = s + x
        return s
""")

write("117_multi_three_aug_then_return.py", """
    def f(a, b):
        a += 1
        b += 2
        a += b
        return a
""")

write("118_multi_chain_assign.py", """
    def f():
        a = 1
        b = a
        c = b
        return c
""")

write("119_multi_assign_then_call.py", """
    def f(g):
        x = 1
        g(x)
        return x
""")

# ---------------------------------------------------------------- 120-139
# Expr matrix at depth 1 — every Expr kind as RHS of Return.

write("120_expr_const_int.py", """
    def f():
        return 42
""")

write("121_expr_const_str.py", """
    def f():
        return "hello"
""")

write("122_expr_const_bytes.py", """
    def f():
        return b"hi"
""")

write("123_expr_const_float.py", """
    def f():
        return 3.14
""")

write("124_expr_const_none.py", """
    def f():
        return None
""")

write("125_expr_const_true.py", """
    def f():
        return True
""")

write("126_expr_name.py", """
    def f(x):
        return x
""")

write("127_expr_binop_add.py", """
    def f(a, b):
        return a + b
""")

write("128_expr_binop_mul.py", """
    def f(a, b):
        return a * b
""")

write("129_expr_unary_neg.py", """
    def f(a):
        return -a
""")

write("130_expr_unary_not.py", """
    def f(a):
        return not a
""")

write("131_expr_compare_lt.py", """
    def f(a, b):
        return a < b
""")

write("132_expr_compare_chain.py", """
    def f(a, b, c):
        return a < b < c
""")

write("133_expr_boolop_and.py", """
    def f(a, b):
        return a and b
""")

write("134_expr_boolop_or.py", """
    def f(a, b):
        return a or b
""")

write("135_expr_call_zero.py", """
    def f(g):
        return g()
""")

write("136_expr_call_one.py", """
    def f(g, x):
        return g(x)
""")

write("137_expr_call_two.py", """
    def f(g, x, y):
        return g(x, y)
""")

write("138_expr_attr_load.py", """
    def f(o):
        return o.x
""")

write("139_expr_subscript_load.py", """
    def f(o, i):
        return o[i]
""")

# ---------------------------------------------------------------- 140-159
# Container / IfExp / NamedExpr / Lambda / Starred — composition

write("140_expr_tuple.py", """
    def f(a, b):
        return (a, b)
""")

write("141_expr_list.py", """
    def f(a, b):
        return [a, b]
""")

write("142_expr_dict.py", """
    def f(a, b):
        return {"a": a, "b": b}
""")

write("143_expr_set.py", """
    def f(a, b):
        return {a, b}
""")

write("144_expr_ifexp.py", """
    def f(a, b):
        return a if a > 0 else b
""")

write("145_expr_namedexpr.py", """
    def f(a):
        return (x := a + 1)
""")

write("146_expr_lambda.py", """
    def f():
        return lambda x: x + 1
""")

write("147_expr_starred_call.py", """
    def f(g, args):
        return g(*args)
""")

write("148_expr_double_starred_call.py", """
    def f(g, kw):
        return g(**kw)
""")

write("149_expr_call_kwargs.py", """
    def f(g, x):
        return g(a=x, b=1)
""")

write("150_expr_chained_attr.py", """
    def f(o):
        return o.x.y
""")

write("151_expr_chained_subscript.py", """
    def f(o):
        return o[0][1]
""")

write("152_expr_attr_call.py", """
    def f(o):
        return o.method()
""")

write("153_expr_subscript_call.py", """
    def f(o):
        return o[0]()
""")

write("154_expr_call_result_attr.py", """
    def f(g):
        return g().x
""")

write("155_expr_nested_binop.py", """
    def f(a, b, c):
        return a + b + c
""")

write("156_expr_nested_binop_paren.py", """
    def f(a, b, c):
        return a * (b + c)
""")

write("157_expr_negative_const.py", """
    def f():
        return -1
""")

write("158_expr_unary_in_compare.py", """
    def f(a):
        return -a < 0
""")

write("159_expr_call_in_binop.py", """
    def f(g, x):
        return g(x) + 1
""")

# ---------------------------------------------------------------- 160-179
# If-test variations — every Expr kind as If.test or While.test

write("160_if_name.py", """
    def f(x):
        if x:
            return 1
        return 0
""")

write("161_if_attr.py", """
    def f(o):
        if o.x:
            return 1
        return 0
""")

write("162_if_call.py", """
    def f(g):
        if g():
            return 1
        return 0
""")

write("163_if_subscript.py", """
    def f(o):
        if o[0]:
            return 1
        return 0
""")

write("164_if_not_name.py", """
    def f(x):
        if not x:
            return 1
        return 0
""")

write("165_if_and.py", """
    def f(a, b):
        if a and b:
            return 1
        return 0
""")

write("166_if_or.py", """
    def f(a, b):
        if a or b:
            return 1
        return 0
""")

write("167_if_compare_chain.py", """
    def f(a, b, c):
        if a < b < c:
            return 1
        return 0
""")

write("168_if_walrus.py", """
    def f(g):
        if (x := g()):
            return x
        return 0
""")

write("169_if_call_compare.py", """
    def f(g):
        if g() > 0:
            return 1
        return 0
""")

write("170_if_const_truthy.py", """
    def f():
        if 1:
            return 1
        return 0
""")

write("171_if_const_falsy.py", """
    def f():
        if 0:
            return 1
        return 0
""")

write("172_while_name.py", """
    def f(it):
        while it:
            it = it - 1
""")

write("173_while_compare.py", """
    def f(n):
        while n > 0:
            n = n - 1
""")

write("174_while_walrus.py", """
    def f(g):
        while (x := g()):
            pass
""")

write("175_while_not.py", """
    def f(x):
        while not x:
            x = 1
""")

write("176_while_and.py", """
    def f(a, b):
        while a and b:
            a = a - 1
""")

write("177_for_call_iter.py", """
    def f(seq):
        for x in iter(seq):
            x
""")

write("178_for_attr_iter.py", """
    def f(o):
        for x in o.items:
            x
""")

write("179_for_subscript_iter.py", """
    def f(o):
        for x in o[0]:
            x
""")

# ---------------------------------------------------------------- 180-199
# Inner def composition — nested args, defaults, closures, depth-3

write("180_inner_def_one_arg.py", """
    def outer():
        def inner(x):
            return x
        return inner
""")

write("181_inner_def_two_args.py", """
    def outer():
        def inner(x, y):
            return x + y
        return inner
""")

write("182_inner_def_default.py", """
    def outer():
        def inner(x=1):
            return x
        return inner
""")

write("183_inner_def_two_defaults.py", """
    def outer():
        def inner(x=1, y=2):
            return x + y
        return inner
""")

write("184_inner_def_vararg.py", """
    def outer():
        def inner(*args):
            return args
        return inner
""")

write("185_inner_def_kwarg.py", """
    def outer():
        def inner(**kw):
            return kw
        return inner
""")

write("186_inner_def_kwonly.py", """
    def outer():
        def inner(*, x=1):
            return x
        return inner
""")

write("187_inner_def_closure_capture.py", """
    def outer():
        n = 5
        def inner():
            return n
        return inner
""")

write("188_inner_def_closure_two.py", """
    def outer():
        a = 1
        b = 2
        def inner():
            return a + b
        return inner
""")

write("189_inner_def_closure_param.py", """
    def outer(p):
        def inner():
            return p
        return inner
""")

write("190_inner_def_closure_with_default.py", """
    def outer():
        n = 5
        def inner(k=2):
            return n + k
        return inner
""")

write("191_inner_def_3deep.py", """
    def a():
        def b():
            def c():
                return 1
            return c
        return b
""")

write("192_inner_def_3deep_closure.py", """
    def a():
        x = 1
        def b():
            def c():
                return x
            return c
        return b
""")

write("193_inner_def_3deep_nonlocal.py", """
    def a():
        x = 1
        def b():
            nonlocal x
            x = x + 1
        return b
""")

write("194_inner_def_annotation.py", """
    def outer():
        def inner(x: int) -> int:
            return x
        return inner
""")

write("195_inner_def_decorator.py", """
    def outer(deco):
        @deco
        def inner():
            return 1
        return inner
""")

write("196_inner_def_returns_call.py", """
    def outer(g):
        def inner():
            return g()
        return inner
""")

write("197_inner_def_uses_outer_call.py", """
    def outer():
        def helper():
            return 1
        def inner():
            return helper()
        return inner
""")

write("198_inner_def_pass.py", """
    def outer():
        def inner():
            pass
        return inner
""")

write("199_inner_def_two_inners.py", """
    def outer():
        def a():
            return 1
        def b():
            return 2
        return a, b
""")

# ---------------------------------------------------------------- 220-239
# Composition (Stmt × Expr × scope) — what spec 1571 originally targeted

write("220_global_in_compare_in_if.py", """
    g = 0


    def f():
        global g
        if g < 0:
            raise ValueError
""")

write("221_global_assign_in_if.py", """
    g = 0


    def f():
        global g
        if g < 0:
            g = 0
""")

write("222_walrus_in_while.py", """
    def f(it):
        while (x := next(it)):
            y = x
""")

write("223_walrus_in_if.py", """
    def f(g):
        if (x := g()):
            return x
        return 0
""")

write("224_compare_chain_in_return.py", """
    def f(a, b, c):
        return a < b < c
""")

write("225_boolop_in_compare_chain_in_return.py", """
    def f(a, b):
        return a > 0 and b < 10
""")

write("226_or_in_return.py", """
    def f(a, b):
        return a or b or 0
""")

write("227_ifexp_in_return.py", """
    def f(a, b):
        return a if a > 0 else b
""")

write("228_call_chain_in_return.py", """
    def f(g):
        return g()()
""")

write("229_attr_in_compare_in_if.py", """
    def f(o):
        if o.x > 0:
            return 1
        return 0
""")

write("230_subscript_in_compare_in_if.py", """
    def f(o):
        if o[0] > 0:
            return 1
        return 0
""")

write("231_call_in_assign.py", """
    def f(g):
        x = g(1, 2)
        return x
""")

write("232_aug_with_call_rhs.py", """
    def f(a, g):
        a += g(1)
        return a
""")

write("233_return_lambda_call.py", """
    def f():
        return (lambda x: x + 1)(1)
""")

write("234_return_tuple_of_calls.py", """
    def f(g, h):
        return g(), h()
""")

write("235_assign_dict_literal.py", """
    def f(a, b):
        d = {"a": a, "b": b}
        return d
""")

write("236_assign_list_literal.py", """
    def f(a, b):
        lst = [a, b, a + b]
        return lst
""")

write("237_aug_in_for_body.py", """
    def f(seq):
        s = 0
        for x in seq:
            s += x
        return s
""")

write("238_assign_in_while_body.py", """
    def f(n):
        s = 0
        while n > 0:
            s = s + n
            n = n - 1
        return s
""")

write("239_nested_if_in_for.py", """
    def f(seq):
        for x in seq:
            if x > 0:
                return x
        return 0
""")

# ---------------------------------------------------------------- 250-279
# Real-world stdlib slices

write("250_stdlib_keyword_iskeyword.py", """
    def iskeyword(s):
        return s in {"if", "else", "while", "for"}
""")

write("251_stdlib_struct_calcsize_short.py", """
    def calcsize(fmt):
        if fmt == "i":
            return 4
        if fmt == "q":
            return 8
        return 0
""")

write("252_stdlib_max_two.py", """
    def maxof(a, b):
        if a > b:
            return a
        return b
""")

write("253_stdlib_min_two.py", """
    def minof(a, b):
        if a < b:
            return a
        return b
""")

write("254_stdlib_clamp.py", """
    def clamp(v, lo, hi):
        if v < lo:
            return lo
        if v > hi:
            return hi
        return v
""")

write("255_stdlib_sign.py", """
    def sign(x):
        if x > 0:
            return 1
        if x < 0:
            return -1
        return 0
""")

write("256_stdlib_abs.py", """
    def absolute(x):
        if x < 0:
            return -x
        return x
""")

write("257_stdlib_factorial.py", """
    def fact(n):
        r = 1
        for i in range(1, n + 1):
            r = r * i
        return r
""")

write("258_stdlib_power.py", """
    def power(base, exp):
        r = 1
        while exp > 0:
            r = r * base
            exp = exp - 1
        return r
""")

write("259_stdlib_gcd.py", """
    def gcd(a, b):
        while b:
            a, b = b, a % b
        return a
""")

write("260_stdlib_fib.py", """
    def fib(n):
        a = 0
        b = 1
        for _ in range(n):
            a, b = b, a + b
        return a
""")

write("261_stdlib_count.py", """
    def count(seq, target):
        n = 0
        for x in seq:
            if x == target:
                n = n + 1
        return n
""")

write("262_stdlib_sumof.py", """
    def sumof(seq):
        s = 0
        for x in seq:
            s = s + x
        return s
""")

write("263_stdlib_contains.py", """
    def contains(seq, target):
        for x in seq:
            if x == target:
                return True
        return False
""")

write("264_stdlib_first.py", """
    def first(seq):
        for x in seq:
            return x
        return None
""")

write("265_stdlib_last_value.py", """
    def last(seq):
        x = None
        for v in seq:
            x = v
        return x
""")

write("266_stdlib_reverse_pairs.py", """
    def swap(p):
        a, b = p
        return b, a
""")

write("267_stdlib_dict_get_or.py", """
    def get_or(d, k, default):
        if k in d:
            return d[k]
        return default
""")

write("268_stdlib_str_repeat.py", """
    def repeat(s, n):
        out = ""
        while n > 0:
            out = out + s
            n = n - 1
        return out
""")

write("269_stdlib_pair_eq.py", """
    def pair_eq(a, b):
        return a[0] == b[0] and a[1] == b[1]
""")

write("270_stdlib_normalize_neg.py", """
    def norm(x):
        if x < 0:
            x = -x
        return x
""")

write("271_stdlib_is_in_range.py", """
    def in_range(v, lo, hi):
        return lo <= v <= hi
""")

write("272_stdlib_safe_div.py", """
    def safe_div(a, b):
        if b == 0:
            return 0
        return a / b
""")

write("273_stdlib_classify.py", """
    def classify(x):
        if x > 0:
            return "pos"
        elif x < 0:
            return "neg"
        else:
            return "zero"
""")

write("274_stdlib_count_while.py", """
    def count_down(n):
        count = 0
        while n > 0:
            count = count + 1
            n = n - 1
        return count
""")

write("275_stdlib_walrus_loop.py", """
    def consume(it):
        while (x := next(it, None)) is not None:
            pass
""")

write("276_stdlib_assert_pre.py", """
    def safe(x, lo, hi):
        assert lo <= hi
        if x < lo:
            return lo
        if x > hi:
            return hi
        return x
""")

write("277_stdlib_global_counter.py", """
    counter = 0


    def tick():
        global counter
        counter = counter + 1
        return counter
""")

write("278_stdlib_nonlocal_counter.py", """
    def make_counter():
        n = 0
        def tick():
            nonlocal n
            n = n + 1
            return n
        return tick
""")

write("279_stdlib_default_factory.py", """
    def factory(default=10):
        def get():
            return default
        return get
""")


print(f"wrote fixtures into {ROOT}", file=sys.stderr)
