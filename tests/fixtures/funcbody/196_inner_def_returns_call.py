def outer(g):
    def inner():
        return g()
    return inner
