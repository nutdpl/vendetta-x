#!/bin/zsh
# runwatt.sh -- VX/32 live-caller proof: boot the PROTECTED-MODE telnet board
# (engine + io_watt + Watt-32) in DOSBox-X over an emulated NE2000 + a Crynwr
# packet driver, connect a host telnet client, and assert the bytes received
# match the local io_local render exactly. This is the out-of-CI test that turns
# "the telnet build links" (what CI proves) into "the board answers telnet".
#
# Unlike the dead-ended runtelnet.sh (real-mode mTCP, never fit the 64k DGROUP),
# this runs the flat 32-bit VENDX.EXE under DOS/4GW. We don't compile in DOSBox
# (the local Watcom path is flaky -- see spin.sh); we download the exact
# VENDX-dos32-telnet artifact CI already built and just RUN it.
#
# macOS dev harness. Requires (all external to this repo):
#   - dosbox-x          (brew install dosbox-x; needs the slirp backend)
#   - gh                (GitHub CLI, authenticated -- to fetch the CI artifact)
#   - Open Watcom tree  (only for dos4gw.exe, the DOS/4GW extender)
#   - an NE2000 packet driver .COM (Crynwr) -- the only external binary
# Override the paths below via env vars if yours differ.
set -e
setopt null_glob 2>/dev/null || true   # empty globs vanish instead of erroring
WATCOM_DIR=${WATCOM_DIR:-/Users/dan/dos-toolchain/extracted}
CRYNWR_DIR=${CRYNWR_DIR:-/Users/dan/crynwr_mirror/binaries}   # holds ne2000.com
PROJ=${PROJ:-/Users/dan/bbs}
PORT=${PORT:-12300}                                          # free host port -> guest 23

cd "$PROJ"

echo "[1/4] fetching the protected-mode telnet VENDX.EXE from the latest green CI..."
RID=$(gh run list --workflow build --limit 25 --json databaseId,conclusion \
        -q '[.[] | select(.conclusion=="success")][0].databaseId')
[ -n "$RID" ] || { echo "no successful CI run found"; exit 1; }
rm -rf /tmp/vxwattdl; mkdir -p /tmp/vxwattdl
gh run download "$RID" -n VENDX-dos32-telnet -D /tmp/vxwattdl
cp /tmp/vxwattdl/VENDX.EXE "$PROJ/VENDX.EXE"
cp "$WATCOM_DIR/binw/dos4gw.exe" "$PROJ/dos4gw.exe"
echo "      VENDX.EXE = $(wc -c < "$PROJ/VENDX.EXE") bytes (32-bit telnet, run $RID)"

# Watt-32 config: static IP on the DOSBox-X slirp net (gateway .2, guest .15).
# Watt-32 auto-detects the packet driver on int 0x60-0x80, so no driver vector
# needs to be named here. WATTCP.CFG (the env var) points at this dir.
cat > "$PROJ/WATTCP.CFG" <<CFG
my_ip   = 10.0.2.15
netmask = 255.255.255.0
gateway = 10.0.2.2
sockdelay = 3
CFG

CONF=""
mk_conf() { CONF=$(mktemp /tmp/vxwattconf.XXXXXX); print -r -- "$1" > "$CONF"; }
run_dosbox() {                                # $1 = conf body, $2 = watchdog seconds
  mk_conf "$1"
  SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy dosbox-x -conf "$CONF" -nopromptfolder >/dev/null 2>&1 &
  DPID=$!
  ( sleep "$2"; kill $DPID 2>/dev/null ) & WDOG=$!
}

echo "[2/4] booting the board in DOSBox-X (NE2000+slirp) and listening on :$PORT..."
pkill -f dosbox-x 2>/dev/null || true; sleep 1
run_dosbox "[dosbox]
memsize=32
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
mount d $PROJ
mount g $CRYNWR_DIR
set WATTCP.CFG=D:\\WATTCP.CFG
g:\\ne2000 0x60 10 0x300 > d:\\ne.log
d:
VENDX.EXE auto > d:\\vx.log
exit" 60

echo "[3/4] connecting a telnet client and capturing the board's bytes..."
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
open('/tmp/vxwatt_wire.bin','wb').write(data)
print("      received", len(data), "bytes over telnet")
PY
sleep 1; kill $DPID 2>/dev/null || true; kill $WDOG 2>/dev/null || true
pkill -f dosbox-x 2>/dev/null || true; rm -f "$CONF"

echo "[4/4] verifying telnet bytes == local io_local render..."
# strip telnet IAC, then diff against a fresh local render
python3 - <<'PY'
d=open('/tmp/vxwatt_wire.bin','rb').read(); o=bytearray(); i=0; n=len(d); I=255
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
open('/tmp/vxwatt_clean.bin','wb').write(o)
PY
( cd "$PROJ" && make >/dev/null 2>&1 && ./vendx auto > /tmp/vxwatt_local.bin 2>&1 && rm -f vendx )
rm -f "$PROJ"/VENDX.EXE "$PROJ"/dos4gw.exe "$PROJ"/WATTCP.CFG "$PROJ"/ne.log "$PROJ"/vx.log 2>/dev/null || true
if cmp -s /tmp/vxwatt_local.bin /tmp/vxwatt_clean.bin; then
  echo "PASS: protected-mode board answers telnet, byte-identical to local console ($(wc -c < /tmp/vxwatt_clean.bin) bytes)"
else
  echo "FAIL: telnet output differs from local render (see /tmp/vxwatt_clean.bin vs /tmp/vxwatt_local.bin)"; exit 1
fi
