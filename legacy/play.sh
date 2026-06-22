#!/bin/zsh
# play.sh -- build the local-console board and open it in a DOSBox-X window.
# Correct CP437 art, fully interactive, no networking needed. Log in, walk the
# menus, post a message. Press 'g' on the main menu to hang up, then close the
# window. Reset state any time with:  rm -f data/*.DAT data/*.MSG
set -e
PROJ=${PROJ:-/Users/dan/bbs}
WATCOM_DIR=${WATCOM_DIR:-/Users/dan/dos-toolchain/extracted}
cd "$PROJ"

echo "building VENDX.EXE (io_local)..."
CONF=$(mktemp /tmp/pigbuild.XXXXXX)
cat > "$CONF" <<EOF
[dosbox]
memsize=16
[cpu]
cycles=max
[autoexec]
mount c $WATCOM_DIR
mount d $PROJ
set WATCOM=C:\\
set PATH=C:\\BINW;%PATH%
set INCLUDE=C:\\H
d:
call mk.bat
exit
EOF
SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy dosbox-x -conf "$CONF" -nopromptfolder >/dev/null 2>&1 || true
rm -f "$CONF"
[ -f VENDX.EXE ] || { echo "build failed -- run 'make' and check for errors"; exit 1; }

echo "opening the board in a DOSBox-X window..."
CONF=$(mktemp /tmp/pigplay.XXXXXX)
cat > "$CONF" <<EOF
[dosbox]
memsize=16
[cpu]
cycles=max
[autoexec]
mount d $PROJ
d:
VENDX.EXE
echo.
echo  (carrier dropped -- close this window)
EOF
dosbox-x -conf "$CONF" -nopromptfolder
rm -f "$CONF" "$PROJ"/VENDX.EXE "$PROJ"/*.obj "$PROJ"/*.map 2>/dev/null || true
