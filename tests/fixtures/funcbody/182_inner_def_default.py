def outer():
    def inner(x=1):
        return x
    return inner
