#!/bin/zsh
# runtelnet.sh -- phase-2 live proof: build the telnet binary, run it in
# DOSBox-X over an emulated NE2000 + mTCP, connect a host telnet client, and
# assert the bytes received match the local io_local render exactly.
#
# macOS dev harness. Requires (all external to this repo):
#   - dosbox-x          (brew install dosbox-x; needs the slirp backend)
#   - Open Watcom       DOS-hosted tree (binw/, h/, lib286/, lib386/)
#   - mTCP source tree  (C++/GPLv3; TCPINC/ TCPLIB/ INCLUDE/)
#   - an NE2000 packet driver .COM (Crynwr) -- the only external binary
# Override the paths below via env vars if yours differ.
set -e
setopt null_glob 2>/dev/null || true   # empty globs vanish instead of erroring
WATCOM_DIR=${WATCOM_DIR:-/Users/dan/dos-toolchain/extracted}
MTCP_DIR=${MTCP_DIR:-/Users/dan/mTCP/MTCP}
CRYNWR_DIR=${CRYNWR_DIR:-/Users/dan/crynwr_mirror/binaries}   # holds ne2000.com
BUILD_DIR=${BUILD_DIR:-/Users/dan/mtcp-build}                 # scratch (objs, cfg)
PROJ=${PROJ:-/Users/dan/bbs}
PORT=${PORT:-12300}                                          # free host port -> guest 23

mkdir -p "$BUILD_DIR"; rm -f "$BUILD_DIR"/*.obj "$PROJ"/VENDX.EXE 2>/dev/null || true
cat > "$BUILD_DIR/mtcp.cfg" <<CFG
PACKETINT 0x60
IPADDR 10.0.2.15
NETMASK 255.255.255.0
GATEWAY 10.0.2.2
NAMESERVER 10.0.2.3
CFG

CONF=""
mk_conf() { CONF=$(mktemp /tmp/pigconf.XXXXXX); print -r -- "$1" > "$CONF"; }
# launch DOSBox as a DIRECT child of this shell so wait/kill work; $2 = seconds
run_dosbox() {
  mk_conf "$1"
  SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy dosbox-x -conf "$CONF" -nopromptfolder >/dev/null 2>&1 &
  DPID=$!
  ( sleep "$2"; kill $DPID 2>/dev/null ) & WDOG=$!
}

echo "[1/3] building VENDX.EXE (engine + io_mtcp + mTCP library)..."
run_dosbox "[dosbox]
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
exit" 240
wait $DPID 2>/dev/null || true; kill $WDOG 2>/dev/null || true; rm -f "$CONF"
[ -f "$PROJ/VENDX.EXE" ] || { echo "BUILD FAILED"; exit 1; }
echo "      VENDX.EXE = $(wc -c < "$PROJ/VENDX.EXE") bytes"

echo "[2/3] running the board in DOSBox-X (NE2000+slirp) and connecting on :$PORT..."
pkill -f dosbox-x 2>/dev/null || true; sleep 1
run_dosbox "[dosbox]
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
g:\\ne2000 0x60 10 0x300 > f:\\ne.log
d:
VENDX.EXE auto > f:\\vx.log
exit" 60
python3 - "$PORT" <<'PY'
import socket,time,sys
port=int(sys.argv[1]); end=time.time()+50; data=b''
while time.time()<end:
    try:
        s=socket.create_connection(('127.0.0.1',port),timeout=2); s.settimeout(10)
        try:
            while True:
                c=s.recv(4096)
                if not c: break
                data+=c
        except socket.timeout: pass
        s.close(); break
    except (ConnectionRefusedError,OSError): time.sleep(1)
open('/tmp/pig_wire.bin','wb').write(data)
print("      received", len(data), "bytes over telnet")
PY
sleep 1; kill $DPID 2>/dev/null || true; kill $WDOG 2>/dev/null || true
pkill -f dosbox-x 2>/dev/null || true; rm -f "$CONF"

echo "[3/3] verifying telnet bytes == local io_local render..."
# strip telnet IAC, then diff against a fresh local render
python3 - <<'PY'
d=open('/tmp/pig_wire.bin','rb').read(); o=bytearray(); i=0; n=len(d); I=255
while i<n:
    b=d[i]
    if b==I:
        if i+1<n and d[i+1]==I: o.append(I); i+=2; continue
        if i+1<n and d[i+1] in (251,252,253,254): i+=3; continue
        if i+1<n and d[i+1]==250:
            j=i+2
            while j+1<n and not(d[j]==I and d[j+1]==240): j+=1
            i=j+2; continue
        i+=2; continue
    o.append(b); i+=1
open('/tmp/pig_clean.bin','wb').write(o)
PY
( cd "$PROJ" && make >/dev/null 2>&1 && ./vendx auto > /tmp/pig_local.bin 2>&1 && rm -f vendx )
if cmp -s /tmp/pig_local.bin /tmp/pig_clean.bin; then
  echo "PASS: board over telnet is byte-identical to local console ($(wc -c < /tmp/pig_clean.bin) bytes)"
else
  echo "FAIL: telnet output differs from local render"; exit 1
fi
