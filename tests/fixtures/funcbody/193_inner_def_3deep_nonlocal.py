def a():
    x = 1
    def b():
        nonlocal x
        x = x + 1
    return b
