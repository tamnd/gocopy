def get_or(d, k, default):
    if k in d:
        return d[k]
    return default
