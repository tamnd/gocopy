def outer():
    n = 5
    def inner():
        return n
    return inner
