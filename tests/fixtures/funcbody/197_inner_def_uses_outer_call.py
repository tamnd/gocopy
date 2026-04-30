def outer():
    def helper():
        return 1
    def inner():
        return helper()
    return inner
