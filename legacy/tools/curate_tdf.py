#!/usr/bin/env python3
"""Score all TDF color fonts for 'organic / hand-drawn' character and render the
top candidates as contact sheets for visual review. (part of the art pipeline.)

Heuristic: organic ACiD-style fonts use lots of shade chars and half blocks, are
tall, vary glyph heights, and shift colors often. Geometric fonts are mostly
solid blocks at uniform height.

usage: python curate_tdf.py "VENDETTA" fontdir [fontdir2 ...] outdir topN
"""
import os
import statistics
import sys

from render_tdf import glyph, parse_fonts, render, serialize

SHADE = set("░▒▓")
HALF = set("▌▐▀▄")


def score(font):
    cells = sh = hf = 0
    heights = []
    attrs = set()
    for c in "WILDPG":
        g = glyph(font, c)
        if g is None:
            return None
        _, grid = g
        heights.append(len(grid))
        for row in grid:
            for ch, attr in row:
                cells += 1
                u = bytes([ch]).decode("cp437")
                if u in SHADE:
                    sh += 1
                elif u in HALF:
                    hf += 1
                attrs.add(attr)
    if cells < 60 or max(heights) < 6:
        return None
    return (
        2.0 * sh / cells
        + 1.0 * hf / cells
        + 0.04 * min(len(attrs), 12)
        + 0.15 * statistics.pstdev(heights)
        + 0.05 * min(max(heights), 12)
    )


def shape_key(font):
    """Same letterforms recolored (Cyan/Red/... variants) -> same key."""
    parts = []
    for c in "WILDPG":
        g = glyph(font, c)
        _, grid = g
        parts.append(tuple(tuple(ch for ch, _ in row) for row in grid))
    return hash(tuple(parts))


def render_block(font, text):
    rows = render(font, text)
    if rows is not None:
        body, width = serialize(rows)
        if width <= 78:
            return body
    out = []
    for part in text.split():
        r = render(font, part)
        if r is None:
            return None
        out.append(serialize(r)[0])
    return "\n".join(out)


def main():
    text = sys.argv[1]
    fontdirs = sys.argv[2:-2]
    outdir, topn = sys.argv[-2], int(sys.argv[-1])
    os.makedirs(outdir, exist_ok=True)
    best = {}
    for d in fontdirs:
        for fn in sorted(os.listdir(d)):
            if not fn.lower().endswith(".tdf"):
                continue
            try:
                fonts = parse_fonts(os.path.join(d, fn))
            except Exception:
                continue
            for f in fonts:
                try:
                    s = score(f)
                    if s is None:
                        continue
                    k = shape_key(f)
                    if k not in best or s > best[k][0]:
                        best[k] = (s, f, fn)
                except Exception:
                    continue
    ranked = sorted(best.values(), key=lambda t: -t[0])[:topn]
    per_sheet = 8
    index = []
    for i in range(0, len(ranked), per_sheet):
        sheet = []
        for rank, (s, f, fn) in enumerate(ranked[i : i + per_sheet], start=i + 1):
            body = render_block(f, text)
            if body is None:
                continue
            label = "#%02d %.2f %s [%s]" % (rank, s, f.name.strip(), fn)
            sheet.append("\x1b[0;36m%s\x1b[0m\n%s\n" % (label, body))
            index.append(label)
        n = i // per_sheet + 1
        with open(
            os.path.join(outdir, "sheet%02d-utf8.ans" % n), "w",
            encoding="utf-8", newline="\n",
        ) as fp:
            fp.write("\n".join(sheet))
    with open(os.path.join(outdir, "index.txt"), "w", encoding="utf-8") as fp:
        fp.write("\n".join(index))
    print("%d unique shapes scored, top %d -> %d sheets" % (
        len(best), len(ranked), (len(ranked) + per_sheet - 1) // per_sheet))


if __name__ == "__main__":
    main()
