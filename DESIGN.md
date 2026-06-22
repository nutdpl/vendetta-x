# Vendetta/X — design document

Vendetta/X is a modern bulletin board system with the soul of a 1996 elite
board and the spine of a 2026 server. This document describes how it's built and
why. It tracks the Go server in `server/` — the board that ships.

## What we're building

A single program that serves one board over three faces at once — **telnet**
(ANSI), **ssh** (ANSI), and a **modern web UI** — all backed by one shared SQLite
database. The terminal faces look and feel like Iniquity/Mystic-era boards (real
CP437 pipe-code art, lightbar menus, ACS access strings, a full-screen ANSI
editor). The web face is a fast, clean rendering of the same live data plus a
full sysop configuration panel. Connect over any face and your activity is
visible on the others in real time.

## Product bar

Commercial-grade, data-driven, scene-authentic:

- **One dataset, many renderings.** No face is a second-class citizen. A message
  posted on ssh is readable on the web and over telnet immediately.
- **Configurable at runtime.** The sysop runs the board from a browser — bases,
  areas, users, content, and feature toggles — with no recompile and no config
  files to hand-edit.
- **Authentic UX.** Numbered Iniquity-style area pickers, ACS access conditions,
  pipe-code art with live token splicing, and the dark AMi/X look throughout.
- **Hardened.** It's meant to face the open internet (see Security below), not
  just a trusted LAN.

## Architecture

One Go binary, one `*sql.DB`, three transports:

```
            telnet :2323 ─┐
            ssh    :2222 ─┼─► board session (term.Session) ─┐
                          │                                  ├─► store (SQLite spine)
            web    :8080 ─┴─► net/http + html/template ──────┘
```

- **`internal/store`** — the spine. One SQLite database (`modernc.org/sqlite`,
  pure Go, no CGO; WAL + `busy_timeout` for concurrent writes) holds users,
  message boards + messages, file areas + files, and oneliners. Every face reads
  and writes through it. This file is the contract the rest of the code targets.
- **`internal/term`** — the transport-agnostic terminal session. It wraps any
  `io.ReadWriteCloser` (a telnet socket or an ssh channel), speaks the telnet IAC
  codec, decodes keys/arrows, renders `.pp` art via the render package, and runs
  lightbar menus. The board flow (`server/main.go` + `server/server_*.go`) is
  written once against `term.Session` and driven identically by both telnet and
  ssh.
- **`internal/render`** — the `.pp` pipe-code engine: `|XX` colors, theme slots,
  `|UH`-style live tokens, list triplets with width/justify, and `|{...}`
  lightbar option markers. The art in `art/` is authored as pipe-code and
  rendered to CP437 at runtime.
- **`internal/web`** — the web face: a small, dependency-free `net/http` +
  `html/template` server. One isolated template set per page over a shared base
  layout; the design system lives in `static/*.css`.
- **`internal/acs`** — Iniquity-style Access Condition Strings (`s100`, flags,
  groups, …) evaluated against a subject derived from the logged-in user. Bases
  and areas carry read/post/access ACS; the same evaluator gates both faces.
- **`internal/auth`** — bcrypt password hashing/verification.
- **`internal/sshface`** — the ssh transport (host key, permissive auth — the
  board runs its own login over the channel).
- **`internal/chat`** — the multi-node teleconference hub.

### The feature-package pattern

Each optional feature owns an isolated package that creates and manages its own
table over the shared `*sql.DB`: `mail`, `voting`, `bbslist`, `gfiles`, `door`.
Each is independently testable and exposes a small store API; the telnet UI
(`server/server_<feature>.go`) and the web UI (`internal/web/web_<feature>.go`)
are thin renderings over it. This keeps features decoupled and lets the board
grow without entangling the core store.

Cross-cutting helpers: `internal/throttle` (per-IP failure limiter) and
`internal/sanitize` (strips terminal control bytes from user text at the write
boundary).

## Access model (ACS)

Access is governed by Iniquity-style ACS strings rather than a fixed role list.
A board might gate reads with `s10` (security level ≥ 10) and the sysop base with
`s100`. A user's subject (SL, DSL, flags, group) is evaluated against the string
on every access, on every face, so telnet and web enforce identical rules. Sysop
access on the web panel is SL ≥ 100 or the `A` flag.

## Security

Vendetta/X assumes hostile callers:

- bcrypt passwords; per-IP login throttling (web + telnet), counting unknown-user
  attempts; reserved/validated handles.
- Optional TLS for the web face (or a terminating proxy) with `Secure`,
  `HttpOnly`, `SameSite` session cookies; HTTP read/header/write/idle timeouts.
- A concurrent-session cap and an idle-session watchdog over telnet/ssh; a telnet
  "press ESC twice" gate that drops bots before the board.
- Control-byte sanitization of all stored user text (no ANSI injection onto other
  callers' screens); parameterized SQL throughout; `html/template` auto-escaping.
- Per-session panic recovery so one bad session can't crash the board; a sysop
  audit log of every admin mutation; graceful shutdown that drains HTTP and
  flushes SQLite.

## Prior art (the soul)

Vendetta/X synthesizes the best of the boards it grew up on:

- **Mystic BBS** — the pipe-code art standard. Vendetta/X's `.pp` renderer and
  token system follow this lineage.
- **Synchronet** — robustness and multi-node sensibilities; the board is built to
  run many simultaneous callers cleanly.
- **Iniquity 1a.25** — the soul. The numbered area pickers, ACS access strings,
  the data-driven menu feel, and the elite aesthetic are modeled on Iniquity's
  UX, studied from the original source and docs.

The synthesis: Iniquity's feel and access model, Mystic's art pipeline, and
Synchronet's robustness — delivered over telnet, ssh, and the web at once.

## Testing

- Each `internal/*` feature package has isolated unit tests over an in-memory
  SQLite database.
- The web package parses every template and exercises handlers via `httptest`.
- `server/main_test.go` drives full board sessions end-to-end over `net.Pipe`
  (new-user signup, login, menu navigation, clean teardown), deadline-guarded so
  tests can't hang.
- CI builds, vets, and tests the Go server, and separately builds/tests the
  legacy C tree (see below).

## Roadmap

Forward-looking ideas, not commitments:

- Richer message networking (QWK is in; a modern federation/sync is the deeper
  goal).
- A real door ecosystem (DOSBox-hosted classics via the drop-file + I/O bridge
  already in place) with shared door score/state.
- More sysop tooling: theming/art management from the panel, scheduled events,
  trashcan/ban management, structured audit views.
- Web polish: live presence over SSE, richer profiles, an art gallery.

## Legacy: the C/DOS board

The project began as a strict-C BBS that also cross-compiles to 16- and 32-bit
DOS (with a real TCP/IP stack via Watt-32). That implementation is preserved
under `legacy/` — built and tested on its own — as the project's roots and a
telnet-on-DOS reference. It is not part of the Go board and not required to build
or run it. See `legacy/README.md`.
