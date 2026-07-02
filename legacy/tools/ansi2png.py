#!/usr/bin/env python3
"""ansi2png.py -- render a CP437 + 16-color ANSI stream to a PNG.

The feedback loop that makes hand-drawn ANSI (and engine-rendered screens)
reviewable without a DOS box: dump a screen, render it, look at it, fix it.
Pair it with tools/render_dump (the real board renderer) so the picture is
exactly what a caller sees:

    ./render_dump art/lbdemo.pp 1 | python3 tools/ansi2png.py - shots/lbdemo.png

Or render a finished .ans straight:

    python3 tools/ansi2png.py art/example.ans out.png

A CP437 ANSI->PNG renderer. Dependency: Pillow. By default it draws
with the bundled IBM VGA 8x16 ROM bitmap (tools/VGA8.F16) for pixel-exact CP437;
pass --font some.ttf to use a TrueType face instead.
"""
import sys
import os
from PIL import Image, ImageDraw, ImageFont

# RGB for the 16 ANSI/SGR colors, indexed by ANSI colour number (0-7) plus 8
# for the bright/bold variants -- this is the order the board emits (ANSI_FG in
# core/render.c: 30=black 31=red 32=green 33=yellow 34=blue 35=magenta 36=cyan
# 37=white), NOT the DOS palette bit order. RGB values are the classic VGA hues.
VGA = [
    (0x00, 0x00, 0x00), (0xAA, 0x00, 0x00), (0x00, 0xAA, 0x00), (0xAA, 0x55, 0x00),
    (0x00, 0x00, 0xAA), (0xAA, 0x00, 0xAA), (0x00, 0xAA, 0xAA), (0xAA, 0xAA, 0xAA),
    (0x55, 0x55, 0x55), (0xFF, 0x55, 0x55), (0x55, 0xFF, 0x55), (0xFF, 0xFF, 0x55),
    (0x55, 0x55, 0xFF), (0xFF, 0x55, 0xFF), (0x55, 0xFF, 0xFF), (0xFF, 0xFF, 0xFF),
]

HERE = os.path.dirname(os.path.abspath(__file__))
VGA_ROM = os.path.join(HERE, "VGA8.F16")     # IBM VGA 8x16 ROM bitmap (default)
TTF_CANDIDATES = [
    "/mnt/skills/examples/canvas-design/canvas-fonts/IBMPlexMono-Regular.ttf",
    "/mnt/skills/examples/canvas-design/canvas-fonts/JetBrainsMono-Regular.ttf",
    "/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
]


class Cell:
    __slots__ = ("cp", "fg", "bg")

    def __init__(self):
        self.cp = 0x20       # raw CP437 code point (0..255)
        self.fg = 7
        self.bg = 0


def parse(data, cols, rows):
    """Interpret an ANSI/CP437 byte stream into a grid of Cells."""
    grid = [[Cell() for _ in range(cols)] for _ in range(rows)]
    x = y = 0
    fg, bg, bold = 7, 0, False

    def grow(ny):
        while ny >= len(grid):
            grid.append([Cell() for _ in range(cols)])

    i, n = 0, len(data)
    while i < n:
        b = data[i]
        if b == 0x1B and i + 1 < n and data[i + 1] == 0x5B:      # ESC [
            j = i + 2
            while j < n and not (0x40 <= data[j] <= 0x7E):
                j += 1
            if j >= n:
                break
            final = chr(data[j])
            params = data[i + 2:j].decode("latin1")
            nums = [int(p) for p in params.split(";") if p.isdigit()] or []
            if final == "m":
                for p in (nums or [0]):
                    if p == 0:
                        fg, bg, bold = 7, 0, False
                    elif p == 1:
                        bold = True
                    elif p == 22:
                        bold = False
                    elif 30 <= p <= 37:
                        fg = p - 30
                    elif 40 <= p <= 47:
                        bg = p - 40
                    elif 90 <= p <= 97:
                        fg = p - 90 + 8
                    elif 100 <= p <= 107:
                        bg = p - 100 + 8
            elif final in "Hf":
                y = (nums[0] - 1) if len(nums) >= 1 and nums[0] > 0 else 0
                x = (nums[1] - 1) if len(nums) >= 2 and nums[1] > 0 else 0
                grow(y)
            elif final == "A":
                y = max(0, y - (nums[0] if nums else 1))
            elif final == "B":
                y = y + (nums[0] if nums else 1); grow(y)
            elif final == "C":
                x = x + (nums[0] if nums else 1)
            elif final == "D":
                x = max(0, x - (nums[0] if nums else 1))
            elif final == "d":
                y = (nums[0] - 1) if nums and nums[0] > 0 else 0; grow(y)
            elif final == "G":
                x = (nums[0] - 1) if nums and nums[0] > 0 else 0
            elif final == "J":
                if not nums or nums[0] == 2:
                    for row in grid:
                        for c in row:
                            c.cp, c.fg, c.bg = 0x20, 7, 0
                    x = y = 0
            elif final == "K":
                if y < len(grid):
                    for cx in range(x, cols):
                        grid[y][cx] = Cell()
            i = j + 1
            continue
        if b == 0x0D:
            x = 0
        elif b == 0x0A:
            y += 1; x = 0; grow(y)
        elif b == 0x09:
            x = (x // 8 + 1) * 8
        elif b >= 0x20:
            # Deferred (VT-style) wrap, matching real terminals: after the
            # 80th glyph the cursor PARKS at the right edge, and only a
            # further printable wraps. A CR/LF arriving first just starts the
            # next line -- no phantom blank row after a full-width line.
            if x >= cols:
                x = 0; y += 1
            grow(y)
            cell = grid[y][x]
            cell.cp = b
            cell.fg = (fg | 8) if (bold and fg < 8) else fg
            cell.bg = bg
            x += 1
        i += 1
    return grid


def trim(grid):
    """Drop trailing blank rows so the shot is content-height, not a sea of black."""
    last = 0
    for ry, row in enumerate(grid):
        if any(c.cp != 0x20 or c.bg for c in row):
            last = ry
    return grid[:last + 1]


def render_rom(grid, cols, rom, scale):
    """Draw with the IBM VGA 8x16 ROM bitmap: 16 bytes/glyph, MSB = left pixel.
    Rendered at native 1x then NEAREST-scaled, so pixels stay razor crisp."""
    grid = trim(grid)
    rows = len(grid)
    img = Image.new("RGB", (cols * 8, rows * 16), VGA[0])
    px = img.load()
    for ry, row in enumerate(grid):
        for rx in range(cols):
            c = row[rx]
            ox, oy = rx * 8, ry * 16
            if c.bg:
                bgc = VGA[c.bg]
                for yy in range(16):
                    for xx in range(8):
                        px[ox + xx, oy + yy] = bgc
            base = c.cp * 16
            fgc = VGA[c.fg]
            for yy in range(16):
                bits = rom[base + yy]
                if not bits:
                    continue
                for xx in range(8):
                    if bits & (0x80 >> xx):
                        px[ox + xx, oy + yy] = fgc
    if scale != 1:
        img = img.resize((img.width * scale, img.height * scale), Image.NEAREST)
    return img


def render_ttf(grid, cols, font_path, scale):
    cw, ch = 9 * scale, 16 * scale
    font = ImageFont.truetype(font_path, 15 * scale) if font_path else ImageFont.load_default()
    grid = trim(grid)
    rows = len(grid)
    img = Image.new("RGB", (cols * cw, rows * ch), VGA[0])
    d = ImageDraw.Draw(img)
    for ry, row in enumerate(grid):
        for rx in range(cols):
            c = row[rx]
            px, py = rx * cw, ry * ch
            if c.bg:
                d.rectangle([px, py, px + cw - 1, py + ch - 1], fill=VGA[c.bg])
            if c.cp != 0x20:
                d.text((px + scale, py), bytes([c.cp]).decode("cp437", "replace"),
                       fill=VGA[c.fg], font=font)
    return img


def main():
    args = list(sys.argv[1:])
    font_path = None
    scale = 2
    cols = 80
    utf8 = False
    rest = []
    i = 0
    while i < len(args):
        a = args[i]
        if a == "--font":
            font_path = args[i + 1]; i += 2; continue
        if a == "--scale":
            scale = int(args[i + 1]); i += 2; continue
        if a == "--cols":
            cols = int(args[i + 1]); i += 2; continue
        if a == "--utf8":            # input is a UTF-8 preview (e.g. render_tdf output)
            utf8 = True; i += 1; continue
        rest.append(a); i += 1
    if len(rest) < 2:
        sys.exit("usage: ansi2png.py <input.ans|-> <out.png> "
                 "[--scale N] [--cols N] [--font ttf] [--utf8]")
    inp, outp = rest[0], rest[1]

    raw = sys.stdin.buffer.read() if inp == "-" else open(inp, "rb").read()
    # CP437 bytes are the native form; a --utf8 preview is decoded then mapped
    # back to CP437 so the one byte-oriented parser handles both.
    data = raw.decode("utf-8", "replace").encode("cp437", "replace") if utf8 else raw

    grid = parse(data, cols, 25)
    if font_path is None and os.path.exists(VGA_ROM):
        rom = open(VGA_ROM, "rb").read()
        img = render_rom(grid, cols, rom, scale)
        used = "VGA8.F16 (ROM bitmap)"
    else:
        if font_path is None:
            font_path = next((c for c in TTF_CANDIDATES if os.path.exists(c)), None)
        img = render_ttf(grid, cols, font_path, scale)
        used = os.path.basename(font_path or "default")
    img.save(outp)
    print("wrote %s  (%dx%d, font=%s)" % (outp, img.width, img.height, used))


if __name__ == "__main__":
    main()
