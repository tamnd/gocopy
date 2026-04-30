def power(base, exp):
    r = 1
    while exp > 0:
        r = r * base
        exp = exp - 1
    return r
