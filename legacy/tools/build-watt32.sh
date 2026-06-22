#!/usr/bin/env bash
#
# build-watt32.sh -- build the Watt-32 TCP/IP stack as a flat / protected-mode
# Open Watcom library (wattcpwf.lib) for the VX/32 telnet build.
#
# Watt-32 ships a DOS build system (configur.bat + util/mkmake, an SLang/djgpp
# TUI program that won't build on a modern Linux CI box). mkmake is really just
# a section selector over makefile.all, so we reimplement that one job in
# tools/mkmake.py and drive wmake directly. The result is the same wattcpwf.lib
# the DOS build would produce, built on the Linux cross-toolchain.
#
# Requires: Open Watcom on PATH (WATCOM set), plus nasm, flex, git, python3, cc.
# Usage:    tools/build-watt32.sh [OUTDIR]   (default OUTDIR=build/watt32)
# Emits:    OUTDIR/lib/wattcpwf.lib  and headers under OUTDIR/inc
#
# Watt-32 is permissively licensed (freely redistributable; some files carry the
# original BSD notice). It is an EXTERNAL dependency -- never vendored into this
# repo -- pinned by commit for reproducibility.
set -euo pipefail

WATT_REPO=${WATT_REPO:-https://github.com/sezero/watt32.git}
WATT_SHA=${WATT_SHA:-30bfdac65010628eb709e6d18ec863e460928b60}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUTDIR=${1:-$ROOT/build/watt32}

: "${WATCOM:?set WATCOM (and put its binl64 on PATH) first}"
export INCLUDE="$WATCOM/h"

echo "==> Watt-32 $WATT_SHA -> $OUTDIR"
rm -rf "$OUTDIR"
git clone --quiet "$WATT_REPO" "$OUTDIR"
git -C "$OUTDIR" checkout --quiet "$WATT_SHA"

cd "$OUTDIR/src"

# 1) select the WATCOM/FLAT makefile out of makefile.all (our mkmake stand-in),
#    rewrite DOS '\' path separators for the Linux tools.
python3 "$ROOT/tools/mkmake.py" WATCOM FLAT | sed -e 's#\\#/#g' > wf.mak

# 2) gzip is disabled in config.h and no zlib is vendored, so blank ZLIB_OBJS
#    (otherwise the lib target depends on objects with no source).
python3 - <<'PY'
import re
t = open('wf.mak').read()
t = re.sub(r'ZLIB_OBJS =.*?(?=\n\n)', 'ZLIB_OBJS =', t, flags=re.S)
open('wf.mak', 'w').write(t)
PY

mkdir -p watcom/flat

# 3) host helper: bin2c (turns the packet-driver stub binary into a C header)
cc -O2 -o ../util/bin2c ../util/bin2c.c

# 4) generated sources the makefile assumes already exist on a DOS build:
#    the real-mode packet stub (nasm -> bin -> C header) and the flex lexer.
nasm -O0 -f bin -o asmpkt.bin asmpkt.nas
../util/bin2c asmpkt.bin > pkt_stub.h
flex -8 -olanguage.c language.l

# 5) build the library.
wmake -f wf.mak

ls -l "$OUTDIR/lib/wattcpwf.lib"
echo "==> wattcpwf.lib ready"
