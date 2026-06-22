# Vendetta/X

**One board. Three faces. No compromise.**

Vendetta/X is a modern bulletin board system that refuses to pick a lane. The
same board — the same users, messages, files, and live "who's online" — answers
on **telnet**, on **ssh**, and on the **web**, all at once, all from a single Go
binary over one SQLite file. Dial in with a 30-year-old terminal and you land in
full CP437 ANSI with lightbar menus; open the URL and you get a fast, modern
web BBS; and a caller on the wire shows up in the browser's node list in real
time. It's the elite/ACiD-scene aesthetic with a 2026 spine underneath.

Built by **nut** / dpl productions.

---

## Why it's different

Old boards died with the modem. The "retro BBS" revival mostly means a DOS image
behind a WiFi-modem gadget, reachable by exactly one kind of client. Vendetta/X
throws that out:

- **Three faces, one truth.** telnet ANSI, ssh ANSI, and a modern web UI are not
  three apps — they're three renderings of one live dataset. Post a message over
  ssh, read it on the web, get the reply over telnet.
- **One static binary, zero dependencies.** Pure Go with `modernc.org/sqlite`
  (no CGO, no libc). `go build` and you have a single file that runs anywhere;
  the container image is distroless/static.
- **A real sysop, in the browser.** The whole board is configurable at runtime
  from a web admin panel — message bases, file areas, users, content, and
  per-feature on/off switches. No recompiling, no config files to hand-edit.
- **Authentically scene.** Real `.pp` pipe-code art, Iniquity-style numbered
  menus and ACS access strings, a full-screen ANSI message editor, door games,
  QWK offline mail, a teleconference, and a wall — the works.

## The three faces

| Face   | Port   | What you get                                                    |
| ------ | ------ | --------------------------------------------------------------- |
| telnet | `2323` | the old-school terminal board: CP437 art, lightbar menus, ACS gating, bcrypt login, full-screen editor, live teleconference |
| ssh    | `2222` | the exact same board over an encrypted channel (the board runs its own login; ssh is just the pipe) |
| web    | `8080` | a modern HTML rendering of the same board, plus the sysop panel |

All three read and write the one SQLite spine, so presence, posts, files, and
mail are shared instantly across them.

## The full menu

Every main-menu command is wired and live:

- **message bases** — read / post / new-scan, ACS-gated (Iniquity-style);
- **file areas** — browse and list, with uploads and signed-link downloads;
- **email** — private inbox / compose / outbox with unread counts;
- **voting booth** — vote on polls and propose new ones;
- **BBS list** — a directory of other boards, caller-addable;
- **G-files** — the text-file library;
- **doors** — external/DOSBox door games via standard drop files (DOOR.SYS /
  DORINFO1.DEF) with a terminal I/O bridge, plus two built-in native games;
- **QWK offline mail** — download a real `.QWK` packet, upload a `.REP` reply;
- **new-files** — the latest uploads across every readable area;
- **oneliners / the wall** — leave your mark;
- **teleconference** — multi-node live chat;
- plus **user list**, **last callers**, **your stats**, **system info**, and
  **settings** (edit profile, change password).

Most features carry a sysop on/off toggle flipped at runtime; a disabled feature
politely shows a "closed by the sysop" notice and vanishes from the web nav.

## The sysop panel

A complete configuration program lives on the web face at **`/sysop`**
(admin-only). It's full CRUD over the entire board:

- **message bases** and **file areas** — create / edit / delete, with ACS strings;
- **users** — security levels, flags, full profile editor, delete;
- **content** — G-files, the BBS list, voting polls (close / reopen / delete),
  and the wall (moderate one-liners);
- **global settings** — board name, tagline, sysop name, new-user defaults, and
  the per-feature on/off toggles;
- a **dashboard** with live counts and uptime.

Every sysop mutation is written to an audit log with the actor and source IP.

## Hardened for the open internet

Vendetta/X is built to face real callers, not just a LAN:

- bcrypt passwords; per-IP login throttling on every face;
- optional TLS for the web face (or run it behind a terminating proxy) with
  `Secure` session cookies;
- HTTP read/write/idle timeouts (Slowloris-resistant);
- a concurrent-session cap and idle-session reaping over telnet/ssh;
- a telnet "press ESC twice to connect" gate that drops bots and scanners;
- control-byte sanitization so user text can't smuggle ANSI escapes onto other
  callers' screens;
- per-session panic recovery (one bad session can't take down the board) and
  graceful shutdown that flushes the database.

## Build & run

```sh
cd server
CGO_ENABLED=0 go build .   # produces ./server -- one static binary, no libc
./server                   # telnet :2323, ssh :2222, web :8080
```

Useful flags: `-telnet`, `-ssh`, `-http`, `-db`, `-art`, `-hostkey`,
`-tls-cert` / `-tls-key` (HTTPS), `-secure-cookies`, `-max-nodes`, `-idle`.

The board ships with two seed accounts — `nut` (the sysop) and `phantom` (a
regular user). Neither has a password until first login, where you set one.

Or build the container from the **repo root** (the build context is the root so
it can copy both `server/` and `art/`):

```sh
docker build -t vendetta-x .
```

The image exposes **2323 / 2222 / 8080** and keeps the SQLite database and the
ssh host key on a `/data` volume. See `Dockerfile` and `deploy/` for the full
deployment guide.

## Repository layout

```
server/    the Go BBS -- the board that ships (telnet + ssh + web)
art/       the CP437 .pp pipe-code art the board renders
deploy/    deployment notes and helpers
docs/      design and feature documentation
legacy/    the original C/DOS implementation, archived (see legacy/README.md)
```

The `legacy/` tree is the project's roots: a strict-C BBS that also cross-builds
to 16- and 32-bit DOS. It's kept for history and as a telnet-on-DOS reference,
built and tested on its own (`cd legacy && make`); it isn't part of the Go board.

## License

Released under the **MIT License** — use it, fork it, run your own board. See
`LICENSE`. Copyright (c) 2026 nut (dpl productions).
