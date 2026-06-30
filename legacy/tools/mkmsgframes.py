#!/usr/bin/env python3
"""Build the message reader header (msgread.pp) and composer header
(msgedit.pp): unlike the destination screens, these are frames repainted on
every single message read/compose, so they keep the board's recolored,
lightly-eroded box-drawing border instead of the full logo+bar treatment --
a giant TDF wordmark doesn't belong on a frame a caller sees hundreds of
times a session. The box width and |XX\\<NN token alignment are unchanged
from the original so message bodies still line up.

    python3 tools/mkmsgframes.py
"""
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root

DBL_H, MEDSH, DARKSH = 0xCD, 0xB1, 0xB2
TL, TR, BL, BR, MID_L, MID_R = 0xC9, 0xBB, 0xC8, 0xBC, 0xCC, 0xB9
SGL_H = 0xC4

random.seed(23)


def cp437(b):
    return bytes([b]).decode("cp437")


def eroded_rule(width, rng):
    """A horizontal double-rule with occasional shade-character grain --
    subtle, never a gap, so the box stays visually intact as a working frame."""
    out = []
    for _ in range(width):
        if rng.random() < 0.92:
            out.append(cp437(DBL_H))
        else:
            out.append(cp437(rng.choice([MEDSH, DARKSH])))
    return "".join(out)


def msgread():
    rng = random.Random(23)
    w = 78
    lines = [
        "|CL",
        "  |11%s%s%s" % (cp437(TL), eroded_rule(w - 2, rng), cp437(TR)),
        "  |11%s |13|MG\\<52|14|MC\\>16 |11%s" % (cp437(0xBA), cp437(0xBA)),
        "  |11%s%s%s" % (cp437(MID_L), eroded_rule(w - 2, rng), cp437(MID_R)),
        "  |11%s |05From    |08: |15|MF\\<58 |11%s" % (cp437(0xBA), cp437(0xBA)),
        "  |11%s |05To      |08: |15|MT\\<58 |11%s" % (cp437(0xBA), cp437(0xBA)),
        "  |11%s |05Subject |08: |15|MS\\<58 |11%s" % (cp437(0xBA), cp437(0xBA)),
        "  |11%s |05Date    |08: |15|MD\\<58 |11%s" % (cp437(0xBA), cp437(0xBA)),
        "  |11%s%s%s" % (cp437(BL), eroded_rule(w - 2, rng), cp437(BR)),
        "|07",
        "",
    ]
    with open(os.path.join(ROOT, "art", "msgread.pp"), "wb") as f:
        f.write("\n".join(lines).encode("cp437"))
    print("wrote art/msgread.pp")


def msgedit():
    rng = random.Random(29)
    w = 78
    # Half-block tab flourish bracketing "c o m p o s e", same shape as the
    # original, recolored to the board's magenta/cyan identity, with an
    # eroded (not plain) rule trailing off to the right.
    flourish_l = "|05▄▄▄ |13▄▄|14▄ |15c o m p o s e |13▄▄|14▄ "
    tail_w = w - 46
    lines = [
        "|07",
        "|08  " + flourish_l + "|05" + eroded_rule(tail_w, rng),
        "|05   to   |08%s |15|MT" % cp437(0xFA),
        "|05   from |08%s |15|MF" % cp437(0xFA),
        "|05   subj |08%s |15|MS" % cp437(0xFA),
        "  |05" + eroded_rule(w, rng) + "|07",
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
