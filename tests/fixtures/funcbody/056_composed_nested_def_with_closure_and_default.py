def outer():
    n = 5
    def inner(k=2):
        return n + k
    return inner
