#!/usr/bin/env python3
"""Edge-eroded/dithered art primitives, shared by the mk*.py screen generators
that want the board's "textured ACiD" look: a solid shape with a jagged,
shade-character boundary and soft halo, instead of a flat color fill or a
clean gradient band. Measured from a genuine 1996 ACiD Productions piece
(ACID0796.ANS's group logo) -- real dithering is mostly solid fill plus
background, with shade characters concentrated at the transition edge, not a
uniform noise wash.

Also wraps render_tdf's font rendering with the column-headroom fix needed to
avoid the renderer's wrap-on-last-column bug (a row exactly `cols` wide trips
the parser's line-wrap check one character early).
"""
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from render_tdf import parse_fonts, render, serialize  # noqa: E402
from ansi2png import parse as ansi_parse  # noqa: E402

LIGHT, MEDSH, DARKSH, FULL = 0xB0, 0xB1, 0xB2, 0xDB
DBL_H, SGL_H, DIAMOND = 0xCD, 0xC4, 0x04

BLACK, RED, GREEN, BROWN, BLUE, MAGENTA, CYAN, GREY = range(8)
DGREY, BRED, BGREEN, YELLOW, BBLUE, BMAGENTA, BCYAN, WHITE = range(8, 16)


class Cell:
    __slots__ = ("cp", "fg", "bg")

    def __init__(self, cp=0x20, fg=7, bg=0):
        self.cp, self.fg, self.bg = cp, fg, bg


class Canvas:
    """A COLS x rows cell grid, serialized straight to CP437 bytes with plain
    \\n line endings (the board's renderer LF->CRLF's everything itself, and
    never double-\\r's a line that already has one -- see render.go emitByte)."""

    def __init__(self, cols, rows):
        self.cols = cols
        self.grid = [[Cell() for _ in range(cols)] for _ in range(rows)]
        self.raw_lines = {}  # y -> literal text, for pipe-code/token lines

    def grow(self, rows):
        while len(self.grid) < rows:
            self.grid.append([Cell() for _ in range(self.cols)])

    def set(self, x, y, cp, fg, bg=0):
        if 0 <= x < self.cols and y >= 0:
            self.grow(y + 1)
            self.grid[y][x] = Cell(cp, fg, bg)

    def get(self, x, y):
        if 0 <= x < self.cols and 0 <= y < len(self.grid):
            return self.grid[y][x]
        return None

    def text(self, x, y, s, fg, bg=0):
        for i, b in enumerate(s.encode("cp437", "replace")):
            self.set(x + i, y, b, fg, bg)

    def text_centered(self, y, s, fg, bg=0):
        self.text((self.cols - len(s)) // 2, y, s, fg, bg)

    def raw_line(self, y, text):
        """A literal line (pipe codes / data tokens) inserted verbatim,
        bypassing the cell grid -- the renderer resolves these at paint time,
        so they can't be represented as plain colored characters here."""
        self.grow(y + 1)
        self.raw_lines[y] = text

    def paste_tdf_text(self, x0, y0, fontfile, fontname, text):
        """Render `text` in a TDF font and paste it into the canvas at
        (x0, y0). Returns (width, height) of the pasted block."""
        fonts = parse_fonts(fontfile)
        font = next(f for f in fonts if f.name.strip() == fontname)
        rows = render(font, text)
        if rows is None:
            raise ValueError("font %r cannot spell %r" % (fontname, text))
        body, width = serialize(rows, center=False)
        data = body.encode("cp437", "replace")
        # +4 headroom: avoid the renderer's wrap-on-last-column bug.
        grid = ansi_parse(data, width + 4, len(rows) + 2)
        for ry, row in enumerate(grid):
            for rx, c in enumerate(row):
                if c.cp != 0x20:
                    self.set(x0 + rx, y0 + ry, c.cp, c.fg, c.bg)
        return width, len(rows)

    def to_bytes(self):
        out = bytearray()
        cur = None
        for y, row in enumerate(self.grid):
            if y in self.raw_lines:
                out += self.raw_lines[y].encode("cp437", "replace")
                cur = None
            else:
                for c in row:
                    key = (c.fg, c.bg)
                    if key != cur:
                        out += ("\x1b[0;%d;%dm" % (
                            30 + c.fg if c.fg < 8 else 90 + (c.fg - 8),
                            40 + c.bg if c.bg < 8 else 100 + (c.bg - 8))).encode("ascii")
                        cur = key
                    out += bytes([c.cp])
            out += b"\n"
            cur = None
        return bytes(out)

    def to_tpl_rows(self):
        """Render each row as a readable .tpl-format string: literal `\\e` for
        ESC and proper cp437-decoded glyphs, ready to feed through tpl2ans.py
        alongside hand-written pipe-code lines. Returns a list of strings (one
        per row, no trailing newline) so a caller can splice in raw_line rows
        that bypass the cell grid (e.g. |{...} lightbar markers)."""
        rows_out = []
        for y, row in enumerate(self.grid):
            if y in self.raw_lines:
                rows_out.append(self.raw_lines[y])
                continue
            cur = None
            parts = []
            for c in row:
                key = (c.fg, c.bg)
                if key != cur:
                    parts.append("\\e[0;%d;%dm" % (
                        30 + c.fg if c.fg < 8 else 90 + (c.fg - 8),
                        40 + c.bg if c.bg < 8 else 100 + (c.bg - 8)))
                    cur = key
                parts.append(bytes([c.cp]).decode("cp437"))
            rows_out.append("".join(parts).rstrip())
        return rows_out


def textured_bar(canvas, y0, height, erode_side, hue_fn, rng=None, speckle=0.06):
    """A solid `height`-row bar across the canvas width, with the named edge
    (top|bottom) eroded into a jagged dithered boundary, plus light interior
    speckle (material grain)."""
    rng = rng or random
    cols = canvas.cols
    for yy in range(height):
        for x in range(cols):
            canvas.set(x, y0 + yy, FULL, hue_fn(x / cols))
    for yy in range(height):
        for x in range(cols):
            if rng.random() < speckle:
                canvas.set(x, y0 + yy, rng.choice([DARKSH, MEDSH]), hue_fn(x / cols))
    edge_y = y0 if erode_side == "top" else y0 + height - 1
    halo_dir = -1 if erode_side == "top" else 1
    for x in range(cols):
        depth = rng.choices([0, 1, 2, 3], weights=[40, 30, 20, 10])[0]
        fg = hue_fn(x / cols)
        if depth == 0:
            continue
        elif depth == 1:
            canvas.set(x, edge_y, DARKSH, fg)
        elif depth == 2:
            canvas.set(x, edge_y, MEDSH, fg)
            if rng.random() < 0.5:
                canvas.set(x, edge_y + halo_dir, LIGHT, fg)
        else:
            canvas.set(x, edge_y, 0x20, fg)
            if rng.random() < 0.6:
                canvas.set(x, edge_y + halo_dir, rng.choice([LIGHT, MEDSH]), fg)
            if rng.random() < 0.25:
                canvas.set(x, edge_y + 2 * halo_dir, LIGHT, fg)


def accent_blob(canvas, cx, cy, n, color, rng=None):
    """Organic random-walk splash with a dithered (eroding) edge -- an
    off-palette accent like the paint-splash detail in real ACiD pieces."""
    rng = rng or random
    core = {(cx, cy)}
    x, y = cx, cy
    for _ in range(n):
        dx, dy = rng.choice([(-1, 0), (1, 0), (0, -1), (0, 1), (1, 1), (-1, -1)])
        x, y = x + dx, y + dy
        core.add((x, y))
    for (x, y) in core:
        canvas.set(x, y, FULL, color)
    halo1, halo2 = set(), set()
    for (x, y) in core:
        for dx, dy in [(-1, 0), (1, 0), (0, -1), (0, 1)]:
            p = (x + dx, y + dy)
            if p not in core:
                halo1.add(p)
    for (x, y) in halo1:
        for dx, dy in [(-1, 0), (1, 0), (0, -1), (0, 1)]:
            p = (x + dx, y + dy)
            if p not in core and p not in halo1:
                halo2.add(p)
    for (x, y) in halo1:
        if rng.random() < 0.75:
            canvas.set(x, y, rng.choice([DARKSH, MEDSH]), color)
    for (x, y) in halo2:
        if rng.random() < 0.35:
            canvas.set(x, y, LIGHT, color)


def dithered_divider(canvas, y, x0, x1, color, accent_color=None, rng=None, solid=0.85):
    """A horizontal rule that occasionally drops to a shade character instead
    of a clean unbroken line -- a 'circuit trace' rather than a ruled box."""
    rng = rng or random
    for x in range(x0, x1):
        if rng.random() < solid:
            canvas.set(x, y, DBL_H, color)
        else:
            canvas.set(x, y, rng.choice([MEDSH, DARKSH]), color)
    if accent_color is not None:
        canvas.set(x0, y, DIAMOND, accent_color)
        canvas.set(x1 - 1, y, DIAMOND, accent_color)
