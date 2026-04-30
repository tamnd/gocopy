def make_counter():
    n = 0
    def tick():
        nonlocal n
        n = n + 1
        return n
    return tick
