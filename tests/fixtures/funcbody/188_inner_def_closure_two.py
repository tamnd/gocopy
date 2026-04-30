def outer():
    a = 1
    b = 2
    def inner():
        return a + b
    return inner
