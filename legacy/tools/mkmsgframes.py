#!/usr/bin/env python3
"""Build the message reader header (msgread.pp) and composer header
(msgedit.pp): unlike the destination screens, these are frames repainted on
every single message read/compose, so they stay compact -- a giant TDF
wordmark doesn't belong on something a caller sees hundreds of times a
session. But the border itself now carries the same per-cell gradient +
visible erosion as the rest of the board (magenta -> red across each rule,
cyan -> magenta -> red down the sides) instead of a flat single-color box,
which read as too plain next to everything else. The box width and
|XX\\<NN token alignment are unchanged so message bodies still line up.

    python3 tools/mkmsgframes.py
"""
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import DARKSH, DBL_H, LIGHT, MEDSH  # noqa: E402

TL, TR, BL, BR, MID_L, MID_R = 0xC9, 0xBB, 0xC8, 0xBC, 0xCC, 0xB9
SIDE = 0xBA

# Plain DOS palette indices for `|NN` pipe codes -- NOT dither.py's
# BCYAN/BMAGENTA/BRED etc, which are calibrated for Canvas's direct-ANSI
# emission (the `90 + (fg - 8)` bright-code formula) and mean something
# different as a literal pipe-code number. |14 is yellow, not cyan; |09 is
# light blue, not light red. Mixing the two conventions up is exactly the
# bug that produced a yellow-not-cyan rule here the first time around.
MAGENTA, PIPE_BCYAN, PIPE_BMAGENTA, PIPE_BRED = 5, 11, 13, 12


def cp437(b):
    return bytes([b]).decode("cp437")


def col_hue(t):
    """left -> right: cyan -> magenta -> red, for horizontal rules."""
    if t < 0.25:
        return PIPE_BCYAN
    elif t < 0.6:
        return MAGENTA
    elif t < 0.85:
        return PIPE_BMAGENTA
    return PIPE_BRED


def row_hue(t):
    """top -> bottom: cyan -> magenta -> red, for the side borders."""
    if t < 0.2:
        return PIPE_BCYAN
    elif t < 0.55:
        return MAGENTA
    elif t < 0.85:
        return PIPE_BMAGENTA
    return PIPE_BRED


def gradient_rule(width, rng, speckle=0.22):
    """A horizontal double-rule, gradient-colored left to right, with visible
    (not barely-there) shade-character erosion -- never a full gap, so the
    box stays intact as a working frame."""
    out = []
    for i in range(width):
        t = i / width
        fg = col_hue(t)
        if rng.random() < speckle:
            cp = rng.choice([MEDSH, DARKSH, LIGHT])
        else:
            cp = DBL_H
        out.append("|%02d%s" % (fg, cp437(cp)))
    return "".join(out)


def side(row_t):
    return "|%02d%s" % (row_hue(row_t), cp437(SIDE))


def msgread():
    rng = random.Random(23)
    w = 76  # interior width between the corner columns
    rows = 8  # row count used for the top->bottom gradient, corners included
    lines = [
        "|CL",
        "  |%02d%s%s|%02d%s" % (col_hue(0), cp437(TL), gradient_rule(w, rng), col_hue(1), cp437(TR)),
        "  %s |13|MG\\<52|14|MC\\>16 %s" % (side(1 / rows), side(1 / rows)),
        "  |%02d%s%s|%02d%s" % (col_hue(0), cp437(MID_L), gradient_rule(w, rng), col_hue(1), cp437(MID_R)),
        "  %s |05From    |08: |15|MF\\<58 %s" % (side(3 / rows), side(3 / rows)),
        "  %s |05To      |08: |15|MT\\<58 %s" % (side(4 / rows), side(4 / rows)),
        "  %s |05Subject |08: |15|MS\\<58 %s" % (side(5 / rows), side(5 / rows)),
        "  %s |05Date    |08: |15|MD\\<58 %s" % (side(6 / rows), side(6 / rows)),
        "  |%02d%s%s|%02d%s" % (col_hue(0), cp437(BL), gradient_rule(w, rng), col_hue(1), cp437(BR)),
        "|07",
        "",
    ]
    with open(os.path.join(ROOT, "art", "msgread.pp"), "wb") as f:
        f.write("\n".join(lines).encode("cp437"))
    print("wrote art/msgread.pp")


def msgedit():
    rng = random.Random(29)
    w = 78
    # Half-block tab flourish bracketing "c o m p o s e", recolored to the
    # board's identity, trailed by the same gradient-eroded rule as msgread.
    flourish_l = "|05▄▄▄ |13▄▄|14▄ |15c o m p o s e |13▄▄|14▄ "
    tail_w = w - 26  # visible width of flourish_l, pipe codes stripped
    lines = [
        "|07",
        "|08  " + flourish_l + gradient_rule(tail_w, rng),
        "|05   to   |08%s |15|MT" % cp437(0xFA),
        "|05   from |08%s |15|MF" % cp437(0xFA),
        "|05   subj |08%s |15|MS" % cp437(0xFA),
        "  " + gradient_rule(w, rng) + "|07",
        "",
    ]
    with open(os.path.join(ROOT, "art", "msgedit.pp"), "wb") as f:
        f.write("\n".join(lines).encode("cp437"))
    print("wrote art/msgedit.pp")


def main():
    msgread()
    msgedit()


if __name__ == "__main__":
    main()
