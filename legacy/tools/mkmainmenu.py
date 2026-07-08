#!/usr/bin/env python3
"""Build the main menu CHROME: the board's real TDF wordmark and an accent
blob (the loginscreen's treatment, minus the top/bottom eroded bars -- see
below) over a reserved two-column block of menu slots. Emits
art/mainmenu.tpl (readable source) and compiles it to art/mainmenu.pp via
tpl2ans.

The menu itself -- which command lives in which slot, its label, its hotkey
-- is NOT baked into this art. It's sysop-configurable at runtime
(server/internal/menu + the /sysop/menu panel) and spliced into the
@@MENU_OPTIONS@@ placeholder below by server/server_menu.go on every render,
so a sysop's edit takes effect on a caller's next redraw with no art
rebuild. This script only lays out the reserved slot capacity (LEFT_SLOTS
rows in the left column, RIGHT_SLOTS in the right) and the chrome around it.

Unlike every other build_chrome screen (msgmenu/filemenu/goodbye), this one
passes top_bar=False and skips bottom_bar_lines() entirely: with 10 reserved
left-column rows (double a typical submenu's option count) on top of the
usual ~17-row wordmark+bars chrome, the full-bars version needs ~30 terminal
rows -- past what a standard 80x25 terminal (SyncTERM's out-of-the-box
default, among others) can show, so the reserved slots' absolute-position
markers landed past the visible screen and got clobbered by scroll-induced
misalignment (menu items overlapping each other, confirmed by replaying a
captured session through a fixed-height VT100 emulator). Dropping both bars
brings the whole screen back under 25 rows -- the ceiling every other
screen in this project already respects -- at the cost of the decorative
framing on the board's single most-viewed screen.

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
from dither import build_chrome  # noqa: E402

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
                              cols=COLS, environment=True, ice=True, top_bar=False)

    out = list(chrome)  # chrome's first line carries the |CL itself

    # The slot block: no options baked in here (see module docstring) -- one
    # placeholder line the Go renderer replaces with the sysop's current
    # bindings on every render, at the exact rows this reserves. No bottom
    # bar follows (see module docstring): the reserved rows are the last
    # thing on screen, keeping the whole layout under 25 rows.
    base = h + 1
    out.append("@@MENU_OPTIONS@@")

    tpl = os.path.join(ROOT, "art", "mainmenu.tpl")
    pp = os.path.join(ROOT, "art", "mainmenu.pp")
    with open(tpl, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out) + "\n")
    preview = tempfile.NamedTemporaryFile(suffix="-utf8.ans", delete=False).name
    subprocess.check_call([sys.executable, os.path.join(HERE, "tpl2ans.py"),
                           tpl, preview, pp])
    os.unlink(preview)
    last_row = base + max(LEFT_SLOTS, RIGHT_SLOTS) - 1
    print("wrote %s -> %s  (logo h=%d, base row %d, last row %d, %d+%d reserved slots)"
          % (tpl, pp, h, base, last_row, LEFT_SLOTS, RIGHT_SLOTS))


if __name__ == "__main__":
    main()
