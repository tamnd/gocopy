def a():
    x = 1
    def b():
        def c():
            return x
        return c
    return b
