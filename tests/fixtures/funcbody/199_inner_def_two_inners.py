def outer():
    def a():
        return 1
    def b():
        return 2
    return a, b
