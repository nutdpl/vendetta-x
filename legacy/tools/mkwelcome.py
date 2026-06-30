#!/usr/bin/env python3
"""Build welcome.pp: the compact "entry granted" banner a brand-new caller
sees right after the ACCESS GRANTED beats (server.welcomeNewUser in
server/server_connect.go). Deliberately small -- the big TDF wordmark already
played at connect; this is a quick, text-forward credential moment, just
framed with the same edge-dithered divider style as the rest of the board.

    python3 tools/mkwelcome.py
    # -> ../art/welcome.pp
"""
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import BMAGENTA, Canvas, dithered_divider  # noqa: E402

COLS = 80
OUT = os.path.join(ROOT, "art", "welcome.pp")
MIDDOT = bytes([0xFA]).decode("cp437")

random.seed(13)


def main():
    c = Canvas(COLS, 9)
    dithered_divider(c, 0, 2, COLS - 2, BMAGENTA, solid=0.7)
    c.raw_line(
        2,
        "  |13VENDETTA|05/|13X |08%s |07entry granted, |14|UH|07."
        % MIDDOT,
    )
    c.raw_line(
        3,
        "  |08dialed in from |07|UL|08 %s this is call |15#|UC|07" % MIDDOT,
    )
    dithered_divider(c, 4, 2, COLS - 2, BMAGENTA, solid=0.7)
    c.raw_line(
        6,
        "  |08running |15|BN|08 %s core v|15|VR|08 %s node 0 %s 486dx2/66"
        % (MIDDOT, MIDDOT, MIDDOT),
    )

    with open(OUT, "wb") as f:
        f.write(c.to_bytes())
    print("wrote", OUT)


if __name__ == "__main__":
    main()
