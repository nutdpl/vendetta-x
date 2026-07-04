#!/usr/bin/env python3
"""Build the logoff screen (art/goodbye.pp): the send-off a caller gets when
they [G]oodbye off the board. Same treatment as every other destination
screen -- GOODBYE in the Cybercrime TDF over a star field with the eroded
bars -- plus the caller's handle spliced in live (|UH) and a closing line,
so hanging up feels like leaving a place, not killing a process. The board
prints its own "NO CARRIER" after this art, the way the modem used to.

    python3 tools/mkgoodbye.py
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
MIDDOT = bytes([0xFA]).decode("cp437")


def main():
    random.seed(43)
    chrome, h = build_chrome("GOODBYE", FONT_FILE, "Cybercrime", "logoff",
                              cols=COLS, environment=True, ice=True)

    out = ["|CL"] + chrome

    base = h + 1
    lines = [
        "|[Y%d|[X26|07later, |15|UH|07. the wall remembers." % base,
        "|[Y%d|[X22|08come back before the sysop notices you left." % (base + 1),
        "|[Y%d|[X9|05nodes |08%s |14|CN|[X33|05users |08%s |14|TU|[X59|05calls |08%s |14|TC|07"
        % (base + 3, MIDDOT, MIDDOT, MIDDOT),
    ]
    out += lines

    bar_y = base + 5
    bar = bottom_bar_lines(COLS, ice=True)
    out.append("|[Y%d" % bar_y + bar[0])
    out.append(bar[1])

    tpl = os.path.join(ROOT, "art", "goodbye.tpl")
    pp = os.path.join(ROOT, "art", "goodbye.pp")
    with open(tpl, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out) + "\n")
    preview = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "tpl2ans.py"),
                           tpl, preview, pp])
    os.unlink(preview)
    print("wrote %s -> %s (logo h=%d)" % (tpl, pp, h))


if __name__ == "__main__":
    main()
