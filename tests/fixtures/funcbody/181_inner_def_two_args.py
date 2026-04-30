def outer():
    def inner(x, y):
        return x + y
    return inner
