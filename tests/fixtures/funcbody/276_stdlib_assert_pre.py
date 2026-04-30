def safe(x, lo, hi):
    assert lo <= hi
    if x < lo:
        return lo
    if x > hi:
        return hi
    return x
