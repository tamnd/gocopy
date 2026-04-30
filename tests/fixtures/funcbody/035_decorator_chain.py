def d1(f): return f
def d2(f): return f
@d1
@d2
def g():
    return 1
