#!/usr/bin/env python3
"""Render text in TheDraw color fonts (.TDF) -> ANSI sampler.

Part of the art pipeline. Format per tdfiglet.c: magic
\x13'TheDraw FONTS file'\x1a, then font records at pos: marker[4] namelen[1]
name[12] ... type@+21 spacing@+22 blocksize<H>@+23 charlist 94*<H>@+25, glyph
data @+213, next record at +213+blocksize. Glyph: width, height, then
(char,attr) pairs, 0x0D=EOL, 0x00=end. attr: low nibble fg, high nibble bg
(TheDraw palette order).

    python3 render_tdf.py "VENDETTA" fonts/ out
    # -> out-utf8.ans (sampler of every font that can spell it) + out-index.txt
    # pipe the .ans through ansi2png.py to eyeball them all at once.
"""
import os
import struct
import sys

MAGIC = b"\x13TheDraw FONTS file\x1a"
FGA = [30, 34, 32, 36, 31, 35, 33, 37, 90, 94, 92, 96, 91, 95, 93, 97]
BGA = [40, 44, 42, 46, 41, 45, 43, 47]
COLOR_FNT = 2
SPACE_GAP = 4
MAX_WIDTH = 78


class TdfFont:
    def __init__(self, name, spacing, charlist, data, src):
        self.name = name
        self.spacing = spacing
        self.charlist = charlist
        self.data = data
        self.src = src


def parse_fonts(path):
    with open(path, "rb") as f:
        b = f.read()
    if not b.startswith(MAGIC):
        return []
    fonts = []
    pos = len(MAGIC)
    while pos + 213 <= len(b):
        namelen = b[pos + 4]
        name = b[pos + 5 : pos + 5 + min(namelen, 12)].decode("cp437", "replace")
        fonttype = b[pos + 21]
        spacing = b[pos + 22]
        blocksize = struct.unpack_from("<H", b, pos + 23)[0]
        charlist = struct.unpack_from("<94H", b, pos + 25)
        data = b[pos + 213 : pos + 213 + blocksize]
        if fonttype == COLOR_FNT and data:
            fonts.append(TdfFont(name, spacing, charlist, data, os.path.basename(path)))
        if blocksize == 0:
            break
        pos += 213 + blocksize
    return fonts


def glyph(font, c):
    i = ord(c) - 33
    if not 0 <= i < 94 or font.charlist[i] == 0xFFFF:
        return None
    d, p = font.data, font.charlist[i]
    if p + 2 > len(d):
        return None
    w = d[p]
    p += 2
    grid, row = [], []
    while p < len(d) and d[p] != 0:
        ch = d[p]
        p += 1
        if ch == 0x0D:
            grid.append(row)
            row = []
        else:
            if p >= len(d):
                break
            attr = d[p]
            p += 1
            row.append((0x20 if ch < 0x20 else ch, attr))
    if row:
        grid.append(row)
    if not grid:
        return None
    w = max([w] + [len(r) for r in grid])
    return w, grid


def render(font, text):
    pieces = []
    for c in text:
        if c == " ":
            pieces.append(None)
            continue
        g = glyph(font, c) or glyph(font, c.swapcase())
        if g is None:
            return None  # font can't spell the text; skip it
        pieces.append(g)
    if not any(pieces):
        return None
    height = max(len(grid) for p in pieces if p for w, grid in [p])
    rows = []
    for r in range(height):
        line = []
        for p in pieces:
            if p is None:
                line += [(0x20, None)] * SPACE_GAP
                continue
            w, grid = p
            grow = grid[r] if r < len(grid) else []
            line += grow + [(0x20, None)] * (w - len(grow) + font.spacing)
        while line and line[-1][0] == 0x20 and line[-1][1] in (None, 0):
            line.pop()
        rows.append(line)
    return rows


def serialize(rows, center=True):
    width = max((len(r) for r in rows), default=0)
    pad = " " * max(0, (MAX_WIDTH - width) // 2) if center else ""
    out = []
    for line in rows:
        parts, last = [pad], "init"
        for ch, attr in line:
            if attr != last:
                if attr is None:
                    parts.append("\x1b[0m")
                else:
                    parts.append("\x1b[%d;%dm" % (FGA[attr & 0xF], BGA[(attr >> 4) & 0x7]))
                last = attr
            parts.append(bytes([ch]).decode("cp437"))
        parts.append("\x1b[0m")
        out.append("".join(parts))
    return "\n".join(out), width


def main():
    text, fontdir, outbase = sys.argv[1], sys.argv[2], sys.argv[3]
    chunks, index, seen = [], [], set()
    for fn in sorted(os.listdir(fontdir)):
        if not fn.lower().endswith(".tdf"):
            continue
        try:
            fonts = parse_fonts(os.path.join(fontdir, fn))
        except Exception:
            continue
        for font in fonts:
            key = (font.name, font.src)
            if key in seen:
                continue
            seen.add(key)
            try:
                rows = render(font, text)
                if rows is None:
                    parts = text.split()
                    stacked = [render(font, p) for p in parts]
                    if any(s is None for s in stacked):
                        continue
                    body, width = [], 0
                    for s in stacked:
                        t, w = serialize(s)
                        body.append(t)
                        width = max(width, w)
                    body = "\n".join(body)
                else:
                    body, width = serialize(rows)
                    if width > MAX_WIDTH:
                        parts = text.split()
                        stacked = [render(font, p) for p in parts]
                        body, width = [], 0
                        for s in stacked:
                            t, w = serialize(s)
                            body.append(t)
                            width = max(width, w)
                        body = "\n".join(body)
            except Exception:
                continue
            label = "%s  [%s]" % (font.name.strip() or "(unnamed)", font.src)
            chunks.append("\x1b[0;36m%s\x1b[1;30m %s\x1b[0m\n\n%s\n" % ("-" * 4, label, body))
            index.append("%-14s %3d wide  %s" % (font.src, width, font.name.strip()))
    sampler = "\n".join(chunks)
    with open(outbase + "-utf8.ans", "w", encoding="utf-8", newline="\n") as f:
        f.write(sampler)
    with open(outbase + "-index.txt", "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(index))
    print("rendered %d fonts -> %s-utf8.ans" % (len(index), outbase))


if __name__ == "__main__":
    main()
