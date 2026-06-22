#!/usr/bin/env python3
"""Build the full main menu screen: a composed TheDraw logo over the board's
whole command set, laid out in two lightbar columns. Emits art/mainmenu.tpl
(readable source) and compiles it to art/mainmenu.pp (the board screen) via
tpl2ans. The option hotkeys must match the KEY rows in data/MAIN.MNU.

    python3 tools/mkmainmenu.py /path/to/phiber2.tdf
"""
import os
import re
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)

SPEC = "V:0:0,E:0:0,N:0:0,D:0:0,E:0:0,T:0:0,T:0:0,A:0:0"

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


def compose_logo(fontfile):
    tmp = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "logo_compose.py"),
                           fontfile, "0", tmp, SPEC])
    with open(tmp, "r", encoding="utf-8") as f:
        lines = [ln.rstrip("\n") for ln in f]
    os.unlink(tmp)
    return [ln for ln in lines if ln.strip() != ""]


def vwidth(line):
    return len(re.sub(r"\x1b\[[0-9;]*m", "", line))


def main():
    if len(sys.argv) < 2:
        sys.exit("usage: mkmainmenu.py <font.tdf>")
    logo = compose_logo(sys.argv[1])
    h = len(logo)
    indent = " " * max(0, (80 - max((vwidth(l) for l in logo), default=0)) // 2)

    out = ["|CL"]
    for ln in logo:
        out.append(indent + ln.replace("\x1b", "\\e"))
    out.append(indent + "|08─── |07|BN |08─ "
                        "|07main menu |08───")

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
