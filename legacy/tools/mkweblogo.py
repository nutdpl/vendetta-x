#!/usr/bin/env python3
"""Render the real Cybercrime TDF wordmark (the same one used on telnet/ssh)
as a transparent PNG for the web face, plus a small favicon crop -- so the
brand mark is pixel-identical across all three faces instead of the web
having its own plain-text logo.

    python3 tools/mkweblogo.py
    # -> ../server/internal/web/static/img/logo.png
    # -> ../server/internal/web/static/img/favicon.png
"""
import os
import sys

from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import Canvas  # noqa: E402
from ansi2png import render_rom  # noqa: E402

FONT_FILE = os.path.join(ROOT, "art", "fonts", "CYBRCRME.TDF")
OUT_DIR = os.path.join(ROOT, "server", "internal", "web", "static", "img")
VGA_ROM = os.path.join(HERE, "VGA8.F16")


def render_logo_grid():
    tmp = Canvas(200, 20)
    logo_w, logo_h = tmp.paste_tdf_text(0, 0, FONT_FILE, "Cybercrime", "VENDETTA/X")
    return tmp.grid[:logo_h], logo_w, logo_h


def to_ansi2png_grid(rows, width):
    """Adapt dither.Cell rows to ansi2png's Cell shape (cp/fg/bg attrs)."""
    class C:
        __slots__ = ("cp", "fg", "bg")

        def __init__(self, cp, fg, bg):
            self.cp, self.fg, self.bg = cp, fg, bg

    return [[C(c.cp, c.fg, c.bg) for c in row[:width]] for row in rows]


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    rows, w, h = render_logo_grid()
    grid = to_ansi2png_grid(rows, w)
    rom = open(VGA_ROM, "rb").read()
    img = render_rom(grid, w, rom, scale=3)

    # Chroma-key: the canvas background is solid black: bg=0 everywhere
    # outside the glyphs, so pure black -> transparent is safe here (no
    # glyph pixel is intentionally pure black in this font).
    img = img.convert("RGBA")
    px = img.load()
    for y in range(img.height):
        for x in range(img.width):
            r, g, b, a = px[x, y]
            if r == 0 and g == 0 and b == 0:
                px[x, y] = (0, 0, 0, 0)

    logo_path = os.path.join(OUT_DIR, "logo.png")
    img.save(logo_path)
    print("wrote", logo_path, img.size)

    # Favicon: the full "V" glyph (7 char cells) is tall and narrow, so crop
    # a square from its vertical middle instead of padding a thin sliver --
    # a solid gradient-filled V-cross-section reads better at 32px than a
    # thin diagonal stroke would.
    cell_px = 8 * 3
    glyph_w = cell_px * 7
    crop = img.crop((0, 0, glyph_w, img.height))
    side = min(crop.size)
    top = (crop.height - side) // 2
    square = crop.crop((0, top, side, top + side))
    favicon = square.resize((64, 64), Image.LANCZOS)
    favicon_path = os.path.join(OUT_DIR, "favicon.png")
    favicon.save(favicon_path)
    print("wrote", favicon_path, favicon.size)


if __name__ == "__main__":
    main()
