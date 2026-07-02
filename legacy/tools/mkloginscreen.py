#!/usr/bin/env python3
"""Build the flagship loginscreen: the board's pride piece, painted in line by
line before the login matrix (see server/server_connect.go's connect()).

The centerpiece is a source-traced 80-col skeleton figure (source/
skeletonguy80.ans), pulled into the board's identity by remapping its palette
into the cyan/magenta/grey family every other screen wears: the fire wings
become magenta plasma, the greens go ice-cyan, the skull stays bone grey.
Below it, VENDETTA/X set in the real Cybercrime TDF font with the full
environment treatment (cast shadow, glints, floor reflection), the live
"front porch" stats (nodes online, total users, total calls -- pipe-code
data tokens |CN |TU |TC spliced by board.loginTokens in server/main.go),
and the eroded magenta bottom bar.

    python3 tools/mkloginscreen.py
    # -> ../art/loginscreen.ans
"""
import os
import random
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from dither import (  # noqa: E402
    BCYAN, BGREEN, BMAGENTA, BRED, BROWN, Canvas, CYAN, DGREY, GREEN, GREY,
    MAGENTA, RED, WHITE, YELLOW, accent_blob, cast_shadow, dithered_divider,
    floor_reflection, glints, textured_bar,
)

COLS = 80
FONT_FILE = os.path.join(HERE, "..", "..", "art", "fonts", "CYBRCRME.TDF")
SOURCE = os.path.join(HERE, "source", "skeletonguy80.ans")
OUT = os.path.join(HERE, "..", "..", "art", "loginscreen.ans")

MIDDOT = bytes([0xFA]).decode("cp437")   # ·  label/value separator
BULLET = bytes([0xF9]).decode("cp437")   # ∙  footer flourish

random.seed(7)  # stable output across regenerations until the layout changes

# The source piece is red/yellow fire and scattered green over blue/cyan/
# magenta. Fold the out-of-family hues into the board palette (same table for
# foreground and background): fire -> magenta plasma, green -> ice cyan.
REMAP = {
    RED: MAGENTA,
    BRED: BMAGENTA,
    BROWN: MAGENTA,
    YELLOW: BMAGENTA,
    GREEN: CYAN,
    BGREEN: BCYAN,
}

SGR = re.compile(rb"\x1b\[([0-9;]*)([A-Za-z])")


def paste_source(c, path, y0):
    """Interpret the source .ans (SGR color runs only) onto canvas c starting
    at row y0, remapping every color through REMAP. Returns rows consumed."""
    data = open(path, "rb").read()
    data = data.replace(b"\r\n", b"\n").replace(b"\r", b"\n")

    fg, bg = 7, 0
    x, y = 0, 0
    i = 0
    rows = 0
    while i < len(data):
        m = SGR.match(data, i)
        if m:
            params, letter = m.group(1), m.group(2)
            if letter == b"m":
                for p in (params or b"0").split(b";"):
                    n = int(p or b"0")
                    if n == 0:
                        fg, bg = 7, 0
                    elif 30 <= n <= 37:
                        fg = n - 30
                    elif 90 <= n <= 97:
                        fg = n - 90 + 8
                    elif 40 <= n <= 47:
                        bg = n - 40
                    elif 100 <= n <= 107:
                        bg = n - 100 + 8
            # J (clear) / H (home) carry no cells; ignore them.
            i = m.end()
            continue
        b = data[i]
        i += 1
        if b == 0x0A:
            x, y = 0, y + 1
            continue
        if x < COLS:
            c.set(x, y0 + y, b,
                  REMAP.get(fg, fg), REMAP.get(bg, bg))
            rows = max(rows, y + 1)
        x += 1
    return rows


def hue_magenta_red(t):
    return MAGENTA if t < 0.5 else (BMAGENTA if t < 0.8 else BRED)


def build():
    rng = random.Random(1994)
    c = Canvas(COLS, 30)

    # The traced skeleton IS the environment -- night sky, moon, the figure.
    art_rows = paste_source(c, SOURCE, 0)

    # Logo block below the piece: measure the TDF wordmark off-canvas first.
    # Spacing is tight on purpose -- the screen scrolls during the reveal, and
    # every saved row leaves one more row of the reaper in the final viewport.
    tmp = Canvas(200, 30)
    logo_w, logo_h = tmp.paste_tdf_text(0, 0, FONT_FILE, "Cybercrime", "VENDETTA/X")
    lx = max(0, (COLS - logo_w) // 2)
    ly = art_rows + 1

    cast_shadow(c, tmp, lx, ly, logo_w, logo_h, color=DGREY, rng=rng)
    for y in range(logo_h):
        for x in range(logo_w):
            cell = tmp.get(x, y)
            if cell and cell.cp != 0x20:
                c.set(lx + x, ly + y, cell.cp, cell.fg, cell.bg)
    glints(c, [(lx + 9, ly), (lx + 33, ly), (lx + 57, ly)])

    bx = min(COLS - 4, lx + logo_w - 6)
    accent_blob(c, bx, ly + 1, 6, BCYAN, rng=rng)

    base_y = ly + logo_h
    floor_reflection(c, tmp, lx, ly, logo_w, logo_h, base_y, rng=rng)

    tagline_y = base_y + 2
    c.text_centered(
        tagline_y,
        '"you have reached a wrong number ... or the best one you ever dialed."',
        GREY,
    )

    div1_y = tagline_y + 2
    dithered_divider(c, div1_y, 4, COLS - 4, BMAGENTA, accent_color=BCYAN, rng=rng)

    stat_y = div1_y + 1
    c.raw_line(
        stat_y,
        "|[X9|05sysop |08%s |15nut|[X33|05group |08%s "
        "|15dpl productions|[X59|05est |08%s |15MMXXVI|07"
        % (MIDDOT, MIDDOT, MIDDOT),
    )
    c.raw_line(
        stat_y + 1,
        "|[X9|05nodes |08%s |14|CN|[X33|05users |08%s "
        "|14|TU|[X59|05calls |08%s |14|TC|07"
        % (MIDDOT, MIDDOT, MIDDOT),
    )

    div2_y = stat_y + 3
    dithered_divider(c, div2_y, 4, COLS - 4, BMAGENTA, accent_color=BCYAN, rng=rng)

    prompt_y = div2_y + 2
    c.text_centered(prompt_y, "[ press any key to enter the board ]", WHITE)

    foot_y = prompt_y + 2
    c.text_centered(
        foot_y, "%s connected at 57600 %s ansi-bbs %s"
        % (BULLET, MIDDOT, BULLET), DGREY,
    )

    bar_y = foot_y + 2
    textured_bar(c, bar_y, 2, "top", hue_magenta_red, rng=rng,
                 half_block_bite=True, ice_hotspot=True)
    return c, bar_y + 2


def main():
    c, total_rows = build()
    c.grid = c.grid[:total_rows]
    with open(OUT, "wb") as f:
        f.write(c.to_bytes())
    print("wrote", OUT, "(%d rows)" % total_rows)


if __name__ == "__main__":
    main()
