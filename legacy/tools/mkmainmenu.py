#!/usr/bin/env python3
"""Build the main menu CHROME: the board's real TDF wordmark, eroded gradient
bars, and an accent blob (the loginscreen's full treatment) over a reserved
two-column block of menu slots. Emits art/mainmenu.tpl (readable source) and
compiles it to art/mainmenu.pp via tpl2ans.

The menu itself -- which command lives in which slot, its label, its hotkey
-- is NOT baked into this art. It's sysop-configurable at runtime
(server/internal/menu + the /sysop/menu panel) and spliced into the
@@MENU_OPTIONS@@ placeholder below by server/server_menu.go on every render,
so a sysop's edit takes effect on a caller's next redraw with no art
rebuild. This script only lays out the reserved slot capacity (LEFT_SLOTS
rows in the left column, RIGHT_SLOTS in the right) and the chrome around it.

The slot row/col positions are mirrored in server/server_menu.go's
mainMenuSlotPos -- change LEFT_SLOTS/RIGHT_SLOTS/LCOL/RCOL here (or the
wordmark, which changes the logo height h and so the base row) and update
that map to match.

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

# Reserved slot capacity per column -- must match len(menu.MainMenuSlots)'s
# L*/R* split in server/internal/menu/menu.go, and mainMenuSlotPos in
# server/server_menu.go.
LEFT_SLOTS, RIGHT_SLOTS = 10, 9
LCOL, RCOL = 8, 44


def main():
    chrome, h = build_chrome("VENDETTA/X", FONT_FILE, "Cybercrime", "main menu",
                              cols=COLS, environment=True, ice=True)

    out = ["|CL"] + chrome

    # The slot block: no options baked in here (see module docstring) -- one
    # placeholder line the Go renderer replaces with the sysop's current
    # bindings on every render, at the exact rows this reserves.
    rows_per = max(LEFT_SLOTS, RIGHT_SLOTS)
    base = h + 1
    out.append("@@MENU_OPTIONS@@")

    # Markers leave the cursor wherever their last label ended, not at a
    # fresh row -- jump to an absolute row before the bottom bar instead of
    # relying on sequential \n drift (bit us once already on matrix.pp). This
    # jump is absolute, so it doesn't matter that only one placeholder line
    # sits between the divider and here instead of one line per option.
    bar_y = base + rows_per + 1
    bar = bottom_bar_lines(COLS, ice=True)
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
    print("wrote %s -> %s  (logo h=%d, %d+%d reserved slots)"
          % (tpl, pp, h, LEFT_SLOTS, RIGHT_SLOTS))


if __name__ == "__main__":
    main()
