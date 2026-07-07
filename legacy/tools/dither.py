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
MIDDOT_CP = 0xFA  # raw cp437 byte, for Canvas.set() (vs MIDDOT below: decoded str, for text/raw_line)

BLACK, RED, GREEN, BROWN, BLUE, MAGENTA, CYAN, GREY = range(8)
DGREY, BRED, BGREEN, YELLOW, BBLUE, BMAGENTA, BCYAN, WHITE = range(8, 16)

# Emit at most 79 columns per line, no matter how wide the canvas is. A line
# that paints column 80 AND ends in a newline double-spaces on every
# ANSI.SYS-family renderer (TheDraw, DOS ANSI.SYS, SyncTERM's ANSI-BBS/cterm):
# those wrap the cursor to the next row THE MOMENT column 80 is written, so
# the newline that follows opens a second, blank row under every full-width
# line -- stretching the art to double height and shearing everything below
# it (modern terminals defer the wrap until the next glyph, which is why
# xterm-family screens and our PNG previews looked fine while real scene
# terminals garbled). 79 columns + newline renders identically everywhere;
# it's the classic ANSI-scene safe width for exactly this reason.
MAX_EMIT_COLS = 79


class Cell:
    __slots__ = ("cp", "fg", "bg")

    def __init__(self, cp=0x20, fg=7, bg=0):
        self.cp, self.fg, self.bg = cp, fg, bg


class Canvas:
    """A COLS x rows cell grid, serialized straight to CP437 bytes with plain
    \\n line endings (the board's renderer LF->CRLF's everything itself, and
    never double-\\r's a line that already has one -- see render.go emitByte).
    Serialization emits at most MAX_EMIT_COLS (79) columns per row -- see
    that constant for the ANSI.SYS/cterm wrap-doubling story."""

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
                for c in row[:MAX_EMIT_COLS]:
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
            for c in row[:MAX_EMIT_COLS]:
                key = (c.fg, c.bg)
                if key != cur:
                    parts.append("\\e[0;%d;%dm" % (
                        30 + c.fg if c.fg < 8 else 90 + (c.fg - 8),
                        40 + c.bg if c.bg < 8 else 100 + (c.bg - 8)))
                    cur = key
                parts.append(bytes([c.cp]).decode("cp437"))
            rows_out.append("".join(parts).rstrip())
        return rows_out


HALF_TOP, HALF_BOT = 0xDF, 0xDC  # upper/lower half block


def textured_bar(canvas, y0, height, erode_side, hue_fn, rng=None, speckle=0.06,
                  half_block_bite=False, ice_hotspot=False):
    """A solid `height`-row bar across the canvas width, with the named edge
    (top|bottom) eroded into a jagged dithered boundary, plus light interior
    speckle (material grain).

    half_block_bite: the erosion edge steps in half-cell increments (real
    half-block glyphs) instead of only shade-character density -- a finer
    boundary than shade chars alone give.
    ice_hotspot: scatter a few cells with a bright BACKGROUND color instead
    of just foreground shade -- genuine iCE-color technique (SGR 100-107),
    not just bright foreground text."""
    rng = rng or random
    cols = canvas.cols
    for yy in range(height):
        for x in range(cols):
            canvas.set(x, y0 + yy, FULL, hue_fn(x / cols))
    for yy in range(height):
        for x in range(cols):
            if rng.random() < speckle:
                canvas.set(x, y0 + yy, rng.choice([DARKSH, MEDSH]), hue_fn(x / cols))
            if ice_hotspot and rng.random() < speckle * 0.6:
                canvas.set(x, y0 + yy, DARKSH, BRED, bg=BMAGENTA)
    edge_y = y0 if erode_side == "top" else y0 + height - 1
    halo_dir = -1 if erode_side == "top" else 1
    half_glyph = HALF_BOT if erode_side == "top" else HALF_TOP
    for x in range(cols):
        depth = rng.choices([0, 1, 2, 3], weights=[40, 30, 20, 10])[0]
        fg = hue_fn(x / cols)
        if depth == 0:
            continue
        elif depth == 1:
            if half_block_bite and rng.random() < 0.5:
                canvas.set(x, edge_y, half_glyph, fg)
            else:
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


MIDDOT = bytes([0xFA]).decode("cp437")


def hue_magenta_red(t):
    return MAGENTA if t < 0.5 else (BMAGENTA if t < 0.8 else BRED)


def build_chrome(title, font_file, font_name, subtitle, cols=80,
                  top_bar=True, blob=True, rng=None, environment=False, ice=False):
    """Compose the board's standard screen header: an eroded gradient bar,
    the centered TDF wordmark with an accent blob, and a dithered divider
    carrying the board name + subtitle -- the loginscreen's full treatment,
    reusable for any menu screen instead of a bare logo + plain rule.

    environment=True adds the fuller loginscreen-redo treatment: a
    star-noise field + interference static behind the logo, a thinned cast
    shadow, and letter-top glints. ice=True gives the top bar half-block-
    bite erosion + iCE-color background hotspots instead of plain shade
    erosion. Both default off so existing callers (matrix/submenus) render
    exactly as before.

    Returns (lines, rows_consumed) where `lines` is ready to splice into a
    .tpl `out` list right after "|CL", and `rows_consumed` is how many
    terminal rows they occupy counting the |CL line's own row-advance (so a
    caller can place the next thing at row `rows_consumed + 1` for a
    one-row gap, with no further off-by-one math needed).
    """
    rng = rng or random
    c = Canvas(cols, 4)
    y0 = 0
    if top_bar:
        textured_bar(c, 0, 2, "bottom", hue_magenta_red, rng=rng,
                     half_block_bite=ice, ice_hotspot=ice)
        y0 = 3  # 2-row bar + 1 blank gap

    tmp = Canvas(200, 20)
    logo_w, logo_h = tmp.paste_tdf_text(0, 0, font_file, font_name, title)
    lx = max(0, (cols - logo_w) // 2)

    if environment:
        star_field(c, y0, y0 + logo_h, rng=rng)
        interference_lines(c, (y0, y0 + logo_h - 1), rng=rng)
        cast_shadow(c, tmp, lx, y0, logo_w, logo_h, color=DGREY, rng=rng)

    for ry in range(logo_h):
        for rx in range(logo_w):
            cell = tmp.get(rx, ry)
            if cell and cell.cp != 0x20:
                c.set(lx + rx, y0 + ry, cell.cp, cell.fg, cell.bg)
    if blob:
        accent_blob(c, min(cols - 4, lx + logo_w - 5), y0, 10, BCYAN, rng=rng)
    if environment:
        step = max(1, logo_w // 4)
        glints(c, [(lx + step, y0), (lx + step * 2, y0), (lx + step * 3, y0)])

    div_y = y0 + logo_h
    dithered_divider(c, div_y, 4, cols - 4, BMAGENTA, accent_color=BCYAN, solid=0.7, rng=rng)

    lines = c.to_tpl_rows()[:div_y + 1]
    lines.append("|05──── |07|BN |05%s |07%s |05────" % (MIDDOT, subtitle))
    # +1: the |CL line's own \n consumes a row before this content starts.
    return lines, div_y + 2 + 1


def bottom_bar_lines(cols=80, rng=None, ice=False):
    """A 2-row eroded gradient bar (mirrored: erodes its top edge), ready to
    append as the last lines of a screen."""
    rng = rng or random
    c = Canvas(cols, 2)
    textured_bar(c, 0, 2, "top", hue_magenta_red, rng=rng,
                 half_block_bite=ice, ice_hotspot=ice)
    return c.to_tpl_rows()


def star_field(canvas, y0, y1, rng=None, accent_color=MAGENTA, density=0.02):
    """Sparse background noise (dots + occasional light-shade fleck) so a
    logo sits IN a space instead of floating on flat void. Purely additive:
    call before pasting anything opaque on top."""
    rng = rng or random
    for y in range(y0, y1):
        for x in range(canvas.cols):
            r = rng.random()
            if r < density:
                canvas.set(x, y, MIDDOT_CP, DGREY)
            elif r < density * 1.5:
                canvas.set(x, y, LIGHT, DGREY)
            elif r < density * 1.8:
                canvas.set(x, y, MIDDOT_CP, accent_color)


def interference_lines(canvas, rows, rng=None, runs=3, color=DGREY):
    """Short horizontal dash static on the given rows -- a signal-noise
    accent for a background field, not a structural rule."""
    rng = rng or random
    for y in rows:
        for _ in range(runs):
            x0 = rng.randrange(2, max(3, canvas.cols - 10))
            for i in range(rng.randrange(3, 8)):
                canvas.set(x0 + i, y, 0xC4, color)


def cast_shadow(canvas, glyph_canvas, lx, ly, glyph_w, glyph_h, dx=2, dy=1,
                 color=DGREY, density=0.55, rng=None):
    """Drop a shadow of an already-pasted glyph block onto the canvas,
    offset (dx, dy). Thinned to `density` and painted in a neutral color
    (not the glyph's own hue) -- an offset shadow unavoidably lands inside a
    letter's interior gaps too when strokes are only a couple cells apart,
    and a saturated, full-density fill there reads as muddy rather than a
    clean drop shadow. Call BEFORE pasting the glyph itself so the glyph
    paints over its own footprint."""
    rng = rng or random
    for ry in range(glyph_h):
        for rx in range(glyph_w):
            cell = glyph_canvas.get(rx, ry)
            if cell and cell.cp != 0x20 and rng.random() < density:
                canvas.set(lx + rx + dx, ly + ry + dy, LIGHT, color)


def glints(canvas, positions, color=WHITE):
    """Small sparkle marks (a half-block + a dot above it) at each (x, y) --
    a cheap way to fake a light catching a few letter edges."""
    for (x, y) in positions:
        canvas.set(x, y, HALF_TOP, color)
        canvas.set(x + 1, y - 1, MIDDOT_CP, color)


def floor_reflection(canvas, glyph_canvas, lx, ly, glyph_w, glyph_h, base_y,
                      color=MAGENTA, color2=RED, rng=None):
    """A fading reflection under a glyph block's baseline: for each column
    that actually touches the bottom two glyph rows, drop a light shade
    directly below, and sometimes a fainter dot one row further."""
    rng = rng or random
    for rx in range(glyph_w):
        hit = False
        for ry in (glyph_h - 1, glyph_h - 2):
            cell = glyph_canvas.get(rx, ry)
            if cell and cell.cp != 0x20:
                hit = True
                break
        if not hit:
            continue
        if rng.random() < 0.55:
            canvas.set(lx + rx, base_y + 1, LIGHT, color)
        if rng.random() < 0.22:
            canvas.set(lx + rx, base_y + 2, MIDDOT_CP, color2)


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
