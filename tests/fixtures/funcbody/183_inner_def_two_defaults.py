def outer():
    def inner(x=1, y=2):
        return x + y
    return inner
