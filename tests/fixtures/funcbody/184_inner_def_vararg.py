def outer():
    def inner(*args):
        return args
    return inner
