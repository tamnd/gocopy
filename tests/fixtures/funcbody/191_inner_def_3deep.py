def a():
    def b():
        def c():
            return 1
        return c
    return b
