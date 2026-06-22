#!/usr/bin/env python3
"""Build the lightbar demo screen from a *readable* source.

Emits art/lbdemo.tpl -- a human-readable UTF-8 source where the logo blocks
show as real block chars and colours read as \\e[94m hints -- then compiles it
to art/lbdemo.pp (the CP437 file the board loads) via tpl2ans. Edit the .tpl by
hand and recompile with tpl2ans, or change the layout below and re-run me.

    python3 tools/mklbdemo.py /path/to/brothood.tdf
"""
import os
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)

# letter:dx:dy -- natural spacing, no stagger reads cleanest for a block logo
SPEC = "V:0:0,E:0:0,N:0:0,D:0:0,E:0:0,T:0:0,T:0:0,A:0:0"
OPTS = [("F", "File Areas"), ("M", "Message Bases"),
        ("W", "Who's Online"), ("G", "Back to Main")]
OPT_COL = 28


def compose_logo(fontfile):
    tmp = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "logo_compose.py"),
                           fontfile, "0", tmp, SPEC])
    with open(tmp, "r", encoding="utf-8") as f:
        lines = [ln.rstrip("\n") for ln in f]
    os.unlink(tmp)
    return [ln for ln in lines if ln.strip() != ""]


def visible_width(line):
    """Width of a logo line ignoring \\e[..m escapes (rough centring helper)."""
    import re
    return len(re.sub(r"\x1b\[[0-9;]*m", "", line))


def main():
    if len(sys.argv) < 2:
        sys.exit("usage: mklbdemo.py <font.tdf>")
    logo = compose_logo(sys.argv[1])
    h = len(logo)
    width = max((visible_width(ln) for ln in logo), default=0)
    indent = " " * max(0, (80 - width) // 2)

    out = ["|CL"]                                  # clear + home
    for ln in logo:                                # the block logo (rows 1..h)
        out.append(indent + ln.replace("\x1b", "\\e"))   # ESC -> readable \e
    out.append(indent + "|08─── |07|BN |08─ "
                        "|07lightbar menu |08───")          # row h+1

    # options below the logo, consecutive rows, clamped to the 25-row screen
    base = h + 3
    if base + len(OPTS) - 1 > 25:
        base = 25 - len(OPTS) + 1
    for i, (key, label) in enumerate(OPTS):
        out.append("|{%d,%d,%s,%s}" % (base + i, OPT_COL, key, label))

    tpl = os.path.join(ROOT, "art", "lbdemo.tpl")
    pp = os.path.join(ROOT, "art", "lbdemo.pp")
    with open(tpl, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out) + "\n")

    # compile the readable source -> CP437 screen the board loads
    preview = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "tpl2ans.py"),
                           tpl, preview, pp])
    os.unlink(preview)
    print("wrote %s (readable source) -> %s (board screen)" % (tpl, pp))


if __name__ == "__main__":
    main()
