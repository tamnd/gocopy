def outer():
    def inner(**kw):
        return kw
    return inner
