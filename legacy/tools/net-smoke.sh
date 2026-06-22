#!/bin/sh
# net-smoke.sh -- prove the board answers telnet, with no DOS in the loop.
# Runs the sockets board (io_sock) as a server, connects as a caller, and
# asserts the login matrix comes back over the wire. Used by `make net-smoke`
# and CI. Exit 0 = the board answered.
set -e

PORT="${VENDX_PORT:-2323}"
[ -x ./vendx-net ] || { echo "net-smoke: build vendx-net first"; exit 2; }

VENDX_PORT="$PORT" ./vendx-net >/tmp/net-smoke-srv.log 2>&1 &
SRV=$!
trap 'kill "$SRV" 2>/dev/null' EXIT
sleep 1

python3 - "$PORT" <<'PY'
import socket, sys, time
port = int(sys.argv[1])
s = socket.create_connection(("127.0.0.1", port), timeout=5)
s.settimeout(1.5)
buf = b""

def drain():
    global buf
    try:
        while True:
            d = s.recv(4096)
            if not d:
                break
            buf += d
    except socket.timeout:
        pass

drain()                      # telnet negotiation + the login matrix
s.sendall(b"g")              # press G (Goodbye) on the lightbar matrix
time.sleep(0.3); drain()     # goodbye + clean close
s.close()

txt = buf.decode("cp437", "replace").lower()
need = ["this is not a bbs", "login", "new user", "goodbye"]
missing = [k for k in need if k not in txt]
if missing:
    print("net-smoke FAIL: matrix not served, missing %r" % missing)
    sys.exit(1)
print("net-smoke OK: board answered telnet and served the matrix (%d bytes)" % len(buf))
PY
