#!/usr/bin/env python3
"""Build the full main menu screen: the board's real TDF wordmark, eroded
gradient bars, and an accent blob (the loginscreen's full treatment) over the
whole command set, laid out in two lightbar columns. Emits art/mainmenu.tpl
(readable source) and compiles it to art/mainmenu.pp via tpl2ans. The option
hotkeys must match the KEY rows in data/MAIN.MNU.

    python3 tools/mkmainmenu.py
"""
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import bottom_bar_lines, build_chrome  # noqa: E402

import subprocess  # noqa: E402
import tempfile  # noqa: E402

FONT_FILE = os.path.join(ROOT, "art", "fonts", "CYBRCRME.TDF")
COLS = 80

random.seed(17)

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
    chrome, h = build_chrome("VENDETTA/X", FONT_FILE, "Cybercrime", "main menu", cols=COLS)

    out = ["|CL"] + chrome

    # Two columns of options, tight under the chrome -- no 25-row VGA clamp
    # here; the board already runs taller screens (loginscreen is 30 rows)
    # and modern telnet/ssh/web clients aren't locked to a CRT page.
    rows_per = max(len(LEFT), len(RIGHT))
    base = h + 1
    for i, (key, label) in enumerate(LEFT):
        out.append("|{%d,%d,%s,%s}" % (base + i, LCOL, key, label))
    for i, (key, label) in enumerate(RIGHT):
        out.append("|{%d,%d,%s,%s}" % (base + i, RCOL, key, label))

    # Markers leave the cursor wherever their last label ended, not at a
    # fresh row -- jump to an absolute row before the bottom bar instead of
    # relying on sequential \n drift (bit us once already on matrix.pp).
    bar_y = base + rows_per + 1
    bar = bottom_bar_lines(COLS)
    out.append("|[Y%d" % bar_y + bar[0])
    out.append(bar[1])

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
