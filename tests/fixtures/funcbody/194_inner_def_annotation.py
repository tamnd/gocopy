def outer():
    def inner(x: int) -> int:
        return x
    return inner
