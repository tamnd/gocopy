def outer(deco):
    @deco
    def inner():
        return 1
    return inner
