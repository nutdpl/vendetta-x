#!/usr/bin/env python3
"""Build the flagship loginscreen: the board's pride piece, painted in line by
line before the login matrix (see server/server_connect.go's connect()).

VENDETTA/X is set in a real scene TDF font (Cybercrime, art/fonts/CYBRCRME.TDF)
rather than a hand-rolled bitmap face, framed by the board's edge-eroded
dither style (tools/dither.py) instead of flat gradient bars. The live "front
porch" stats (nodes online, total users, total calls) are pipe-code data
tokens (|CN |TU |TC, spliced by board.loginTokens in server/main.go) so the
screen stays alive instead of static.

    python3 tools/mkloginscreen.py
    # -> ../art/loginscreen.ans
"""
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from dither import (  # noqa: E402
    BCYAN, BMAGENTA, BRED, Canvas, DGREY, GREY, MAGENTA, WHITE,
    accent_blob, dithered_divider, textured_bar,
)

COLS = 80
FONT_FILE = os.path.join(HERE, "..", "..", "art", "fonts", "CYBRCRME.TDF")
OUT = os.path.join(HERE, "..", "..", "art", "loginscreen.ans")

MIDDOT = bytes([0xFA]).decode("cp437")   # ·  label/value separator
BULLET = bytes([0xF9]).decode("cp437")   # ∙  footer flourish

random.seed(7)  # stable output across regenerations until the layout changes


def hue_magenta_red(t):
    return MAGENTA if t < 0.5 else (BMAGENTA if t < 0.8 else BRED)


def build():
    c = Canvas(COLS, 30)
    textured_bar(c, 0, 2, "bottom", hue_magenta_red)

    # Render the logo off-canvas to measure its width, then paste it centered.
    tmp = Canvas(200, 30)
    logo_w, logo_h = tmp.paste_tdf_text(0, 0, FONT_FILE, "Cybercrime", "VENDETTA/X")
    lx = max(0, (COLS - logo_w) // 2)
    ly = 3
    for y in range(logo_h):
        for x in range(logo_w):
            cell = tmp.get(x, y)
            if cell and cell.cp != 0x20:
                c.set(lx + x, ly + y, cell.cp, cell.fg, cell.bg)

    accent_blob(c, min(COLS - 4, lx + logo_w - 5), ly, 10, BCYAN)

    tagline_y = ly + logo_h + 1
    c.text_centered(
        tagline_y,
        '"you have reached a wrong number ... or the best one you ever dialed."',
        GREY,
    )

    div1_y = tagline_y + 2
    dithered_divider(c, div1_y, 4, COLS - 4, BMAGENTA, accent_color=BCYAN)

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
    dithered_divider(c, div2_y, 4, COLS - 4, BMAGENTA, accent_color=BCYAN)

    prompt_y = div2_y + 2
    c.text_centered(prompt_y, "[ press any key to enter the board ]", WHITE)

    foot_y = prompt_y + 2
    c.text_centered(
        foot_y, "%s connected at 57600 %s cp437 %s ansi-bbs %s"
        % (BULLET, MIDDOT, MIDDOT, BULLET), DGREY,
    )

    bar_y = foot_y + 3
    textured_bar(c, bar_y, 2, "top", hue_magenta_red)
    return c, bar_y + 2


def main():
    c, total_rows = build()
    c.grid = c.grid[:total_rows]
    with open(OUT, "wb") as f:
        f.write(c.to_bytes())
    print("wrote", OUT, "(%d rows)" % total_rows)


if __name__ == "__main__":
    main()
