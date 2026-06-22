#!/usr/bin/env python3
"""Build the message + file submenus as lightbar screens: a TheDraw section-
title logo over the area list, single column. Emits art/<name>.tpl (readable)
and compiles to art/<name>.pp via tpl2ans. Hotkeys mirror the matching .MNU.

    python3 tools/mksubmenus.py /path/to/phiber2.tdf
"""
import os
import re
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)

# name, TITLE word, subtitle, [(hotkey, label)]
MENUS = [
    ("msgmenu", "MESSAGES", "message bases", [
        ("G", "General Chat"), ("W", "Warez Talk"),
        ("S", "The Scene"), ("Q", "Back to Main")]),
    ("filemenu", "FILES", "file areas", [
        ("W", "Warez Vault"), ("U", "Utilities"),
        ("A", "ANSI & Art"), ("Q", "Back to Main")]),
]


def spec_for(title):
    parts = []
    for ch in title:
        parts.append("_:8:0" if ch == " " else "%s:0:0" % ch)
    return ",".join(parts)


def compose(fontfile, title):
    tmp = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "logo_compose.py"),
                           fontfile, "0", tmp, spec_for(title)])
    with open(tmp, "r", encoding="utf-8") as f:
        lines = [ln.rstrip("\n") for ln in f]
    os.unlink(tmp)
    return [ln for ln in lines if ln.strip() != ""]


def vwidth(line):
    return len(re.sub(r"\x1b\[[0-9;]*m", "", line))


def build(fontfile, name, title, subtitle, opts):
    logo = compose(fontfile, title)
    h = len(logo)
    indent = " " * max(0, (80 - max((vwidth(l) for l in logo), default=0)) // 2)
    ocol = max(1, (80 - max(len(lbl) for _, lbl in opts)) // 2)

    out = ["|CL"]
    for ln in logo:
        out.append(indent + ln.replace("\x1b", "\\e"))
    out.append(indent + "|08─── |07|BN |08─ |07%s |08───" % subtitle)
    base = h + 3
    if base + len(opts) - 1 > 25:
        base = 25 - len(opts) + 1
    for i, (key, label) in enumerate(opts):
        out.append("|{%d,%d,%s,%s}" % (base + i, ocol, key, label))

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
    if len(sys.argv) < 2:
        sys.exit("usage: mksubmenus.py <font.tdf>")
    for name, title, subtitle, opts in MENUS:
        build(sys.argv[1], name, title, subtitle, opts)


if __name__ == "__main__":
    main()
