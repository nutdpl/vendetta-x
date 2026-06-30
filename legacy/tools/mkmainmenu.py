#!/usr/bin/env python3
"""Build the full main menu screen: the board's real TDF wordmark over the
whole command set, laid out in two lightbar columns. Emits art/mainmenu.tpl
(readable source) and compiles it to art/mainmenu.pp via tpl2ans. The option
hotkeys must match the KEY rows in data/MAIN.MNU.

    python3 tools/mkmainmenu.py
"""
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import Canvas  # noqa: E402

import subprocess  # noqa: E402
import tempfile  # noqa: E402

FONT_FILE = os.path.join(ROOT, "art", "fonts", "CYBRCRME.TDF")
COLS = 80
MIDDOT = bytes([0xFA]).decode("cp437")

# (hotkey, label) -- hotkeys mirror data/MAIN.MNU. Two columns, top-to-bottom.
LEFT = [
    ("M", "Message Bases"), ("F", "File Areas"), ("E", "Email"),
    ("O", "Oneliners"), ("W", "Who's Online"), ("C", "Page Sysop"),
    ("D", "Doors"), ("Q", "QWK Mail"), ("N", "New Files"),
]
RIGHT = [
    ("T", "G-Files"), ("B", "BBS List"), ("V", "Voting Booth"),
    ("L", "Last Callers"), ("U", "User List"), ("Z", "Your Stats"),
    ("I", "System Info"), ("X", "Settings"), ("G", "Goodbye"),
]
LCOL, RCOL = 8, 44


def main():
    tmp = Canvas(200, 20)
    logo_w, h = tmp.paste_tdf_text(0, 0, FONT_FILE, "Cybercrime", "VENDETTA/X")
    indent = " " * max(0, (COLS - logo_w) // 2)

    c = Canvas(COLS, h)
    for y in range(h):
        for x in range(logo_w):
            cell = tmp.get(x, y)
            if cell and cell.cp != 0x20:
                c.set(x, y, cell.cp, cell.fg, cell.bg)
    logo_rows = c.to_tpl_rows()

    out = ["|CL"]
    out += [indent + ln if ln else ln for ln in logo_rows]
    out.append(indent + "|05──── |07|BN |05%s |07main menu |05────" % MIDDOT)

    # two columns of options; clamp so the taller column fits the 25-row screen
    rows_per = max(len(LEFT), len(RIGHT))
    base = h + 3
    if base + rows_per - 1 > 25:
        base = 25 - rows_per + 1
    for i, (key, label) in enumerate(LEFT):
        out.append("|{%d,%d,%s,%s}" % (base + i, LCOL, key, label))
    for i, (key, label) in enumerate(RIGHT):
        out.append("|{%d,%d,%s,%s}" % (base + i, RCOL, key, label))

    tpl = os.path.join(ROOT, "art", "mainmenu.tpl")
    pp = os.path.join(ROOT, "art", "mainmenu.pp")
    with open(tpl, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out) + "\n")
    preview = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "tpl2ans.py"),
                           tpl, preview, pp])
    os.unlink(preview)
    print("wrote %s -> %s  (logo h=%d, %d options)"
          % (tpl, pp, h, len(LEFT) + len(RIGHT)))


if __name__ == "__main__":
    main()
