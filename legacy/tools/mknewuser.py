#!/usr/bin/env python3
"""Build newuser.pp: the signup form header shown once before the handle /
location / tagline / group prompts (server.newUser in server/main.go). Same
tier as welcome.pp -- a one-time but not destination moment, so it keeps a
compact, text-forward layout with the board's dithered divider style instead
of a full logo treatment.

    python3 tools/mknewuser.py
    # -> ../art/newuser.pp
"""
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import BMAGENTA, Canvas, dithered_divider  # noqa: E402

COLS = 80
OUT = os.path.join(ROOT, "art", "newuser.pp")
MIDDOT = bytes([0xFA]).decode("cp437")

random.seed(31)


def main():
    c = Canvas(COLS, 10)
    c.raw_line(
        0,
        "|08  ▄▄ |15n e w   u s e r   a p p l i c a t i o n |05▄▄",
    )
    dithered_divider(c, 1, 2, COLS - 2, BMAGENTA, solid=0.7)
    c.raw_line(3, "|05  handle  |08%s |15|UH" % MIDDOT)
    c.raw_line(4, "|05  from    |08%s |15" % MIDDOT)
    c.raw_line(5, "|05  tagline |08%s |15" % MIDDOT)
    c.raw_line(6, "|05  group   |08%s |15" % MIDDOT)
    dithered_divider(c, 7, 2, COLS - 2, BMAGENTA, solid=0.7)
    c.raw_line(
        8,
        "|08  no nup %s Vendetta/X just wants to know who is knocking. fill it in.|07"
        % MIDDOT,
    )

    with open(OUT, "wb") as f:
        f.write(c.to_bytes())
    print("wrote", OUT)


if __name__ == "__main__":
    main()
