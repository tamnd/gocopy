def outer(p):
    def inner():
        return p
    return inner
