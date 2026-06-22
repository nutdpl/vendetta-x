# Deploying Vendetta/X

Vendetta/X is a single Go binary that serves the BBS over three faces at once:

| Face   | Default port | What it is                          |
| ------ | ------------ | ----------------------------------- |
| telnet | `2323`       | ANSI terminal board (real .pp art)  |
| ssh    | `2222`       | same board over an encrypted channel|
| http   | `8080`       | the modern web BBS                  |

The SQLite driver is pure Go (`modernc.org/sqlite`), so the server builds as a
fully static binary (`CGO_ENABLED=0`) with no libc dependency. That makes it
trivial to run on a `distroless`/`scratch` image or to drop onto a host.

**What must persist** (point the flags at durable storage):

- the SQLite database `vendetta.db` and its `-wal` / `-shm` sidecars
- the SSH **host key** (generated on first run; keep it stable so clients don't
  see host-key-changed warnings)

The web templates and CSS are **embedded** in the binary (`go:embed`); the only
external asset is the `art/` directory of ANSI `.pp` screens.

> Note: the legacy C/DOS board archived under `legacy/` is a separate build and
> plays no part in this deployment. Only `server/` and `art/` matter here.

---

## Option A: Docker

The image is `gcr.io/distroless/static:nonroot` (tiny, no shell, runs as the
built-in nonroot user uid **65532**). State is kept on a volume mounted at
`/data`.

### Build

Run from the **repository root** (the build context must include `server/` and
`art/`):

```sh
docker build -t vendetta-x .
```

### Run

```sh
docker run -d --name vendetta-x \
  -p 2323:2323 \
  -p 2222:2222 \
  -p 8080:8080 \
  -v vendetta-data:/data \
  --restart unless-stopped \
  vendetta-x
```

- `-v vendetta-data:/data` is a **named volume**. On first run the server
  creates `/data/vendetta.db` (+ sidecars) and `/data/host_key` there; they
  survive container restarts and re-creation.
- The entrypoint already passes
  `-art=/app/art -db=/data/vendetta.db -hostkey=/data/host_key` plus the default
  listen addresses, so no extra flags are needed.

A named volume inherits the image's `/data` ownership (uid 65532), so it is
writable out of the box. **If you bind-mount a host directory instead**, create
and chown it first, or the server can't write the DB/host key:

```sh
mkdir -p /srv/vendetta-x/data
sudo chown -R 65532:65532 /srv/vendetta-x/data
docker run -d --name vendetta-x \
  -p 2323:2323 -p 2222:2222 -p 8080:8080 \
  -v /srv/vendetta-x/data:/data \
  --restart unless-stopped \
  vendetta-x
```

### Connect

```sh
# Web BBS
open http://localhost:8080      # or just browse to it

# Telnet face
telnet localhost 2323

# SSH face (the board itself handles "login" -- any username works to reach the
# matrix/login screen; -o options silence host-key prompts on a fresh key)
ssh -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null guest@localhost
```

### Logs / lifecycle

```sh
docker logs -f vendetta-x
docker stop vendetta-x
docker start vendetta-x
```

---

## Option B: systemd (host install)

### 1. Build the static binary

```sh
# convenience wrapper (writes <repo>/vendx):
./deploy/build.sh

# ...or by hand:
cd server && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ../vendx .
```

### 2. Install binary + art

```sh
sudo install -d /opt/vendetta-x
sudo install -m 0755 vendx /opt/vendetta-x/vendx
sudo cp -r art /opt/vendetta-x/art
```

### 3. Create the service user

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin vendetta
```

### 4. Install and start the unit

```sh
sudo cp deploy/vendetta-x.service /etc/systemd/system/vendetta-x.service
sudo systemctl daemon-reload
sudo systemctl enable --now vendetta-x
```

`StateDirectory=vendetta-x` makes systemd create and own
`/var/lib/vendetta-x` (mode 0700) for the `vendetta` user. The DB and SSH host
key are created there on first run and persist across restarts and upgrades.

### 5. Check it

```sh
systemctl status vendetta-x
journalctl -u vendetta-x -f
```

You should see a log line like:

```
Vendetta/X: web on :8080, telnet on :2323, ssh on :2222, db /var/lib/vendetta-x/vendetta.db
```

Connect exactly as in the Docker section (point your client at the host instead
of `localhost` if it's remote, and open the relevant ports in your firewall).

### Upgrading

```sh
./deploy/build.sh
sudo install -m 0755 vendx /opt/vendetta-x/vendx
sudo cp -r art /opt/vendetta-x/art          # if art changed
sudo systemctl restart vendetta-x
```

The state in `/var/lib/vendetta-x` (DB + host key) is untouched by an upgrade.

---

## Persistence summary

| What                          | Docker            | systemd                         |
| ----------------------------- | ----------------- | ------------------------------- |
| SQLite DB (`vendetta.db` + WAL/SHM) | `/data` volume | `/var/lib/vendetta-x`           |
| SSH host key                  | `/data/host_key`  | `/var/lib/vendetta-x/host_key`  |
| ANSI art (`art/`)             | baked into image  | `/opt/vendetta-x/art` (on disk) |
| web templates + CSS           | embedded in binary| embedded in binary              |

Keep the DB and host key on durable storage and your board keeps its users,
messages, and stable SSH identity across restarts and redeploys.
