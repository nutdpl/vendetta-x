#!/usr/bin/env python3
"""Build the login matrix -- the first screen a caller sees, before login.
A composed TheDraw logo over the classic [L]ogin / [N]ew / [G]oodbye options
and a "Matrix:" prompt. Emits art/matrix.tpl (readable) and compiles it to
art/matrix.pp via tpl2ans. The board reads the L/N/G hotkey itself.

    python3 tools/mkmatrix.py /path/to/phiber2.tdf
"""
import os
import re
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
SPEC = "V:0:0,E:0:0,N:0:0,D:0:0,E:0:0,T:0:0,T:0:0,A:0:0"


def compose(fontfile):
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
        sys.exit("usage: mkmatrix.py <font.tdf>")
    logo = compose(sys.argv[1])
    indent = " " * max(0, (80 - max((vwidth(l) for l in logo), default=0)) // 2)

    h = len(logo)
    out = ["|CL"]
    for ln in logo:
        out.append(indent + ln.replace("\x1b", "\\e"))
    out.append(indent + "|08──── |07|BN |08·|07 this is not a bbs |08────")
    out.append("")
    out.append(indent + "  |08arrows + enter, or press |15L |08/ |15N |08/ |15G")
    # the three options as lightbar markers (the bar runs over them)
    r = h + 5
    out.append("|{%d,18,L,Login}" % r)
    out.append("|{%d,38,N,New User}" % r)
    out.append("|{%d,58,G,Goodbye}" % r)

    tpl = os.path.join(ROOT, "art", "matrix.tpl")
    pp = os.path.join(ROOT, "art", "matrix.pp")
    with open(tpl, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out) + "\n")
    preview = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "tpl2ans.py"),
                           tpl, preview, pp])
    os.unlink(preview)
    print("wrote %s -> %s" % (tpl, pp))


if __name__ == "__main__":
    main()
