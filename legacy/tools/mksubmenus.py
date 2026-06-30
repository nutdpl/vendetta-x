#!/usr/bin/env python3
"""Build the message + file submenus as lightbar screens: the board's real
TDF wordmark spelling the section title, eroded gradient bars top and bottom,
an accent blob, over the area list, single column -- the loginscreen's full
treatment. Emits art/<name>.tpl (readable) and compiles to art/<name>.pp via
tpl2ans. Hotkeys mirror the matching .MNU.

    python3 tools/mksubmenus.py
"""
import os
import random
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))  # legacy/tools -> repo root
sys.path.insert(0, HERE)
from dither import bottom_bar_lines, build_chrome  # noqa: E402

FONT_FILE = os.path.join(ROOT, "art", "fonts", "CYBRCRME.TDF")
COLS = 80

# name, TITLE word, subtitle, [(hotkey, label)]
MENUS = [
    ("msgmenu", "MESSAGES", "message bases", [
        ("G", "General Chat"), ("W", "Warez Talk"),
        ("S", "The Scene"), ("Q", "Back to Main")]),
    ("filemenu", "FILES", "file areas", [
        ("W", "Warez Vault"), ("U", "Utilities"),
        ("A", "ANSI & Art"), ("Q", "Back to Main")]),
]


def build(seed, name, title, subtitle, opts):
    random.seed(seed)
    chrome, h = build_chrome(title, FONT_FILE, "Cybercrime", subtitle, cols=COLS)
    ocol = max(1, (COLS - max(len(lbl) for _, lbl in opts)) // 2)

    out = ["|CL"] + chrome
    base = h + 1
    for i, (key, label) in enumerate(opts):
        out.append("|{%d,%d,%s,%s}" % (base + i, ocol, key, label))

    bar_y = base + len(opts) + 1
    bar = bottom_bar_lines(COLS)
    out.append("|[Y%d" % bar_y + bar[0])
    out.append(bar[1])

    tpl = os.path.join(ROOT, "art", name + ".tpl")
    pp = os.path.join(ROOT, "art", name + ".pp")
    with open(tpl, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out) + "\n")
    preview = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "tpl2ans.py"),
                           tpl, preview, pp])
    os.unlink(preview)
    print("wrote %s (logo h=%d, %d options)" % (pp, h, len(opts)))


def main():
    for i, (name, title, subtitle, opts) in enumerate(MENUS):
        build(19 + i, name, title, subtitle, opts)


if __name__ == "__main__":
    main()
