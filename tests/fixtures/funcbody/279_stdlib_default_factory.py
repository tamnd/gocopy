def factory(default=10):
    def get():
        return default
    return get
