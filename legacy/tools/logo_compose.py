#!/usr/bin/env python3
"""Compose TDF glyphs into a custom logo: per-letter x/y offsets so letters
overlap and stagger like hand-drawn scene logos instead of sitting in a font
row. Later letters paint over earlier ones. (part of the art pipeline.)

usage: python logo_compose.py font.tdf fontindex outfile-utf8.ans spec
  spec: LETTER:dx:dy[,LETTER:dx:dy...]   dx relative to previous letter's
        right edge (negative = overlap), dy absolute row offset
"""
import sys

from render_tdf import FGA, BGA, glyph, parse_fonts


def main():
    fontfile, fontidx, outfile, spec = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]
    font = parse_fonts(fontfile)[fontidx]
    canvas = {}
    x = 0
    for part in spec.split(","):
        ch, dx, dy = part.split(":")
        if ch == "_":  # plain gap
            x += int(dx)
            continue
        g = glyph(font, ch)
        if g is None:
            print("missing glyph %r" % ch)
            return
        w, grid = g
        x += int(dx)
        y0 = int(dy)
        for r, row in enumerate(grid):
            for c, (b, attr) in enumerate(row):
                if b == 0x20 and (attr >> 4) & 0x7 == 0:
                    continue  # transparent cell
                canvas[(y0 + r, x + c)] = (b, attr)
        x += w
    if not canvas:
        return
    rmin = min(r for r, _ in canvas)
    rmax = max(r for r, _ in canvas)
    cmin = min(c for _, c in canvas)
    cmax = max(c for _, c in canvas)
    lines = []
    for r in range(rmin, rmax + 1):
        parts, last = [], None
        for c in range(cmin, cmax + 1):
            b, attr = canvas.get((r, c), (0x20, None))
            if attr != last:
                if attr is None:
                    parts.append("\x1b[0m")
                else:
                    parts.append("\x1b[%d;%dm" % (FGA[attr & 0xF], BGA[(attr >> 4) & 0x7]))
                last = attr
            parts.append(bytes([b]).decode("cp437"))
        parts.append("\x1b[0m")
        lines.append("".join(parts).rstrip())
    with open(outfile, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(lines) + "\n")
    print("%dx%d -> %s" % (cmax - cmin + 1, rmax - rmin + 1, outfile))


if __name__ == "__main__":
    main()
