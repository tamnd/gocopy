def count(seq, target):
    n = 0
    for x in seq:
        if x == target:
            n = n + 1
    return n
