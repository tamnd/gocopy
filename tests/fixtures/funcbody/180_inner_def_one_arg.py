def outer():
    def inner(x):
        return x
    return inner
