#!/usr/bin/env python3
"""Build the login matrix -- the first interactive screen a caller sees, right
after the loginscreen splash: VENDETTA/X over Login / New User / Goodbye. Reuses
the loginscreen's real TDF wordmark (tools/dither.py, art/fonts/CYBRCRME.TDF)
and edge-dither framing instead of the matrix's old standalone logo, so the
two screens read as one identity instead of two different typefaces back to
back. Emits art/matrix.tpl (readable) and compiles it to art/matrix.pp via
tpl2ans, same as the rest of the .pp screens. The board reads the L/N/G
hotkey itself.

    python3 tools/mkmatrix.py
"""
import os
import random
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import (  # noqa: E402
    BCYAN, BMAGENTA, Canvas, cast_shadow, dithered_divider, glints,
    interference_lines, star_field,
)

COLS = 80
FONT_FILE = os.path.join(ROOT, "art", "fonts", "CYBRCRME.TDF")

random.seed(11)


def main():
    rng = random.Random(11)
    c = Canvas(COLS, 2)

    tmp = Canvas(200, 20)
    logo_w, logo_h = tmp.paste_tdf_text(0, 0, FONT_FILE, "Cybercrime", "VENDETTA/X")
    lx = max(0, (COLS - logo_w) // 2)

    # Same environment treatment as the loginscreen/mainmenu: a star field
    # + shadow behind the logo, not just the bare glyphs -- no top/bottom
    # bars here (this screen stays compact; it repaints on every failed
    # login attempt), so no ice= hotspots to add.
    star_field(c, 0, logo_h, rng=rng)
    interference_lines(c, (0, logo_h - 1), rng=rng, runs=2)
    cast_shadow(c, tmp, lx, 0, logo_w, logo_h, rng=rng)

    for y in range(logo_h):
        for x in range(logo_w):
            cell = tmp.get(x, y)
            if cell and cell.cp != 0x20:
                c.set(lx + x, y, cell.cp, cell.fg, cell.bg)

    step = max(1, logo_w // 4)
    glints(c, [(lx + step, 0), (lx + step * 2, 0), (lx + step * 3, 0)])

    div_y = logo_h
    dithered_divider(c, div_y, 4, COLS - 4, BMAGENTA, accent_color=BCYAN, solid=0.6)

    art_rows = c.to_tpl_rows()  # rows 0..div_y: the logo + divider

    out = ["|CL"]
    out += art_rows
    out.append("        |08──── |07|BN |08\xb7|07 this is not a bbs |08────")
    out.append("")
    out.append("          |05arrows + enter, or press |15L |08/ |15N |08/ |15G")
    # the three options as lightbar markers (the bar runs over them).
    # +1 for the |CL line's own trailing-\n row advance, +1 blank gap below
    # the instruction line.
    r = div_y + 6
    out.append("|{%d,18,L,Login}" % r)
    out.append("|{%d,38,N,New User}" % r)
    out.append("|{%d,58,G,Goodbye}" % r)
    out.append("")
    out.append(
        "|[Y%d|[X11|08── |05nodes |15|CN  |08\xb7  |05users |15|TU  |08\xb7  "
        "|05local |15|TI |08──" % (r + 2)
    )

    tpl = os.path.join(ROOT, "art", "matrix.tpl")
    pp = os.path.join(ROOT, "art", "matrix.pp")
    with open(tpl, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out) + "\n")
    preview = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "tpl2ans.py"),
                           tpl, preview, pp])
    os.unlink(preview)
    print("wrote %s -> %s" % (tpl, pp))


if __name__ == "__main__":
    main()
