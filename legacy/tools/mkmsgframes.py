#!/usr/bin/env python3
"""Build the message reader header (msgread.pp) and composer header
(msgedit.pp): unlike the destination screens, these are frames repainted on
every single message read/compose, so they stay compact -- a giant TDF
wordmark doesn't belong on something a caller sees hundreds of times a
session.

The reader header wears the same treatment as the menus (mksubmenus.py /
mkmainmenu.py): an eroded half-block-bitten gradient bar with iCE hotspots
and glints on top, the dithered circuit-trace divider, a cyan->magenta->red
rail down the field rows in place of the old closed box, and a bitten
half-block rule underneath. Decorative rows are painted with the shared
dither.py primitives on a Canvas and emitted as raw ANSI; the token rows
(|MG/|MC/|MF/... with |XX\\<NN width clamps) stay literal pipe code so live
values keep their alignment.

    python3 tools/mkmsgframes.py
"""
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import (  # noqa: E402
    BCYAN, BMAGENTA, BRED, Canvas, DARKSH, DBL_H, HALF_TOP, LIGHT, MEDSH,
    dithered_divider, glints, hue_magenta_red, textured_bar,
)

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


def canvas_rows(c):
    """Render a Canvas's grid rows as raw-ANSI cp437 strings for a .pp file
    (the renderer passes ESC sequences through, same as the menu screens)."""
    return [r.replace("\\e", "\x1b") for r in c.to_tpl_rows()]


def msgread():
    rng = random.Random(23)

    # Decorative rows on a Canvas, same primitives as the menu chrome: a
    # 2-row gradient bar eroding downward with half-block bite + iCE
    # hotspots and a pair of glints, then (further down, with the token rows
    # between) the circuit-trace divider and a bitten half-block base rule.
    c = Canvas(80, 2)
    textured_bar(c, 0, 2, "bottom", hue_magenta_red, rng=rng,
                 half_block_bite=True, ice_hotspot=True)
    c.grid = c.grid[:2]  # clip the erosion halo: the title row is next
    glints(c, [(14, 0), (63, 0)])
    bar = canvas_rows(c)

    d = Canvas(80, 1)
    dithered_divider(d, 0, 2, 78, BMAGENTA, accent_color=BCYAN, rng=rng)
    divider = canvas_rows(d)[0]

    # The base rule: gradient upper-half blocks with shade bites and a couple
    # of iCE flecks -- the one-row cousin of the menus' bottom bar.
    b = Canvas(80, 1)
    for x in range(2, 78):
        t = (x - 2) / 76.0
        cp = HALF_TOP
        r = rng.random()
        if r < 0.10:
            cp = rng.choice([MEDSH, DARKSH])
        elif r < 0.14:
            cp = LIGHT
        b.set(x, 0, cp, hue_magenta_red(t))
    b.set(rng.randrange(20, 40), 0, DARKSH, BRED, bg=BMAGENTA)  # iCE fleck
    base = canvas_rows(b)[0]

    # Row gradient for the left rail down the field rows (pipe-code colors).
    rail = [PIPE_BCYAN, MAGENTA, PIPE_BMAGENTA, PIPE_BRED]

    lt, md = cp437(LIGHT), cp437(MEDSH)   # ░ ▒
    lh = cp437(0xDD)                       # ▌ the rail glyph
    lines = [
        "|CL",
        bar[0],
        bar[1],
        "  |08%s%s |13|MG\\<50|14|MC\\>16 |08%s%s" % (lt, md, md, lt),
        divider,
        "  |%02d%s |05From    |08: |15|MF\\<58" % (rail[0], lh),
        "  |%02d%s |05To      |08: |15|MT\\<58" % (rail[1], lh),
        "  |%02d%s |05Subject |08: |15|MS\\<58" % (rail[2], lh),
        "  |%02d%s |05Date    |08: |15|MD\\<58" % (rail[3], lh),
        base,
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
