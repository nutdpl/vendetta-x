#!/bin/zsh
# serve.sh -- run the telnet board in DOSBox-X for real callers.
#
# Connect with a BBS terminal (CP437/ANSI) such as SyncTERM:
#     brew install syncterm
#     syncterm telnet://localhost:2424
# Each caller gets a fresh session; the board re-listens in a loop. Log off
# cleanly with 'g' on the main menu so the node frees for the next caller --
# an abrupt disconnect currently hangs the node until you restart (a known
# carrier-detection gap, phase-5 hardening). Close the DOSBox-X window (or
# Ctrl-C) to stop. Reset state: rm -f data/*.DAT data/*.MSG
#
# Needs (external): dosbox-x w/ slirp, Open Watcom, the mTCP tree, and an
# NE2000 packet driver. Override paths via env vars below.
set -e
setopt null_glob 2>/dev/null || true
PROJ=${PROJ:-/Users/dan/bbs}
WATCOM_DIR=${WATCOM_DIR:-/Users/dan/dos-toolchain/extracted}
MTCP_DIR=${MTCP_DIR:-/Users/dan/mTCP/MTCP}
CRYNWR_DIR=${CRYNWR_DIR:-/Users/dan/crynwr_mirror/binaries}   # holds ne2000.com
BUILD_DIR=${BUILD_DIR:-/Users/dan/mtcp-build}
PORT=${PORT:-2424}                                          # 2323 is taken by 'wraith'
cd "$PROJ"; mkdir -p "$BUILD_DIR"

# mTCP runtime config (static IP on slirp's 10.0.2.0/24)
cat > "$BUILD_DIR/mtcp.cfg" <<CFG
PACKETINT 0x60
IPADDR 10.0.2.15
NETMASK 255.255.255.0
GATEWAY 10.0.2.2
NAMESERVER 10.0.2.3
CFG

# load the packet driver once, then run the board (it serves callers in a loop)
cat > "$BUILD_DIR/serve.bat" <<'BAT'
@echo off
g:\ne2000 0x60 10 0x300
d:
VENDX.EXE
BAT

echo "building the telnet board (engine + io_mtcp + mTCP library)..."
rm -f "$BUILD_DIR"/*.obj "$PROJ"/VENDX.EXE
CONF=$(mktemp /tmp/pigsrvb.XXXXXX)
cat > "$CONF" <<EOF
[dosbox]
memsize=63
[cpu]
cycles=max
[autoexec]
mount c $WATCOM_DIR
mount d $PROJ
mount e $MTCP_DIR
mount f $BUILD_DIR
set WATCOM=C:\\
set PATH=C:\\BINW;%PATH%
set INCLUDE=C:\\H;D:\\CORE;D:\\IO;E:\\TCPINC;E:\\INCLUDE
d:
call mkmtcp.bat
exit
EOF
SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy dosbox-x -conf "$CONF" -nopromptfolder >/dev/null 2>&1 || true
rm -f "$CONF"
[ -f "$PROJ/VENDX.EXE" ] || { echo "build failed -- check mkmtcp.bat / mounts"; exit 1; }

echo ""
echo "  *** Vendetta/X is listening on  localhost:$PORT  ***"
echo "  connect:  syncterm telnet://localhost:$PORT   (or any telnet BBS client)"
echo "  stop:     close the DOSBox-X window, or Ctrl-C here"
echo ""
CONF=$(mktemp /tmp/pigsrv.XXXXXX)
cat > "$CONF" <<EOF
[dosbox]
memsize=63
[cpu]
cycles=max
[ne2000]
ne2000=true
nicbase=0x300
nicirq=10
[ethernet, slirp]
backend=slirp
tcp_port_forwards=${PORT}:23
[autoexec]
mount c $WATCOM_DIR
mount d $PROJ
mount f $BUILD_DIR
mount g $CRYNWR_DIR
set MTCPCFG=f:\\mtcp.cfg
f:\\serve.bat
EOF
dosbox-x -conf "$CONF" -nopromptfolder
rm -f "$CONF"
