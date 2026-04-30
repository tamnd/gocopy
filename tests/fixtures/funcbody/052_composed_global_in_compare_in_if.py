g = 0


def f():
    global g
    if g < 0:
        raise ValueError
