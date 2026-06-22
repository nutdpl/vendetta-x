#!/bin/zsh
# spin.sh -- run the 32-bit PROTECTED-MODE Vendetta/X board in a DOSBox-X window.
#
# Downloads the prebuilt VENDX.EXE from the latest green CI run (the local
# DOSBox+Watcom compiler is unreliable, so we build in the cloud and just RUN
# here), stages the DOS/4GW extender next to it, and opens an interactive
# window. Log in (type a handle, or 'new'), walk the menus; press 'g' on the
# main menu to hang up, then close the window.
#
# Reset board state any time:  rm -f data/*.DAT data/*.MSG
set -e
PROJ=${PROJ:-/Users/dan/bbs}
WATCOM_DIR=${WATCOM_DIR:-/Users/dan/dos-toolchain/extracted}
cd "$PROJ"

echo "fetching the latest protected-mode VENDX.EXE from CI..."
RID=$(gh run list --workflow build --limit 25 --json databaseId,conclusion \
        -q '[.[] | select(.conclusion=="success")][0].databaseId')
[ -n "$RID" ] || { echo "no successful CI run found"; exit 1; }
rm -rf /tmp/vx32dl; mkdir -p /tmp/vx32dl
gh run download "$RID" -n VENDX-dos32 -D /tmp/vx32dl
cp /tmp/vx32dl/VENDX.EXE "$PROJ/VENDX.EXE"
cp "$WATCOM_DIR/binw/dos4gw.exe" "$PROJ/dos4gw.exe"
echo "VENDX.EXE = $(wc -c < VENDX.EXE) bytes (32-bit protected mode, run $RID)"

echo "opening the board in a DOSBox-X window (close it when you're done)..."
CONF=$(mktemp /tmp/vxspin.XXXXXX)
cat > "$CONF" <<EOF
[dosbox]
memsize=32
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
rm -f "$CONF" "$PROJ"/VENDX.EXE "$PROJ"/dos4gw.exe 2>/dev/null || true
