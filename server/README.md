# Vendetta/X server (Go)

The board that ships: one Go binary, one shared SQLite dataset, three faces at
once.

- **telnet** (ANSI) on `:2323` — the old-school terminal board: real `.pp` art,
  lightbar menus, ACS access control, bcrypt login, a full-screen message
  editor, and a live multi-node teleconference.
- **ssh** (ANSI) on `:2222` — the exact same board over an encrypted channel.
- **web** (HTML) on `:8080` — the modern rendering of the same board, plus the
  full sysop configuration panel at `/sysop`.

All three read and write one SQLite spine (`modernc.org/sqlite`, pure Go, no
CGO), so users, posts, files, mail, and "nodes online" are shared live across
every face.

## Run

```sh
cd server
CGO_ENABLED=0 go build .   # -> ./server (static, no libc)
./server                   # telnet :2323, ssh :2222, web :8080
```

Flags: `-telnet`, `-ssh`, `-http`, `-db`, `-art`, `-hostkey`, `-tls-cert` /
`-tls-key`, `-secure-cookies`, `-max-nodes`, `-idle`.

Seed accounts: `sysop` (SL 255, the board administrator) plus `nut` and
`phantom` (SL 10 demo users). None ships with a password; you set one on first
login. Because `sysop` is privileged, its first password can only be set from
the **console** — a loopback connection (run the BBS and log in locally, or
tunnel in over SSH). A remote caller is refused, so the admin account can't be
claimed by whoever connects first. Ordinary accounts can set their first
password from anywhere.

The **Sysop** message base is gated by the ACS string `s100`, so `phantom` is
denied and `sysop` gets in — the canonical access-control demo.

## Layout

```
main.go              session flow, presence, node cap, idle reaping, signals
server_*.go          telnet UI per feature (mail, voting, gfiles, bbslist, doors, qwk, ...)
internal/store       SQLite spine: users, boards, messages, files, oneliners, settings
internal/render      .pp pipe-code renderer (colors / tokens / lightbars)
internal/term        telnet/ssh session: IAC codec, keys, lightbar, idle watchdog
internal/acs         Iniquity-style Access Condition Strings
internal/auth         bcrypt password hashing
internal/editor       full-screen ANSI message editor
internal/chat         multi-node teleconference hub
internal/social       leaderboards, last-callers, profile cards
internal/sshface      the ssh transport (host key, channel bridge)
internal/web          the HTML face + the /sysop configuration panel
internal/mail         private mail        \
internal/voting       voting booth         \  isolated feature packages, each
internal/bbslist      BBS directory        /  owning its own table + tests
internal/gfiles       text-file library   /
internal/door         external/DOSBox doors + drop files
internal/throttle     per-IP login limiter
internal/sanitize     control-byte stripping for user text
internal/qwk          QWK packet build + .REP parse
```

## Testing

```sh
go test ./...          # all packages (in-memory SQLite, template parse, e2e session)
go vet ./...
```

`main_test.go` drives full board sessions over `net.Pipe` end to end; every
feature package has its own isolated tests. See `../DESIGN.md` for the
architecture and `../README.md` for the project overview.
