def pick(r, g, b):
    maxc = max(r, g, b)
    if r == maxc:
        h = b - g
    elif g == maxc:
        h = 2.0 + r - b
    else:
        h = 4.0 + g - r
    h = h / 6.0
    return h
