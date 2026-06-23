# Changelog

All notable changes to Vendetta/X are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and the project aims to follow
semantic versioning.

## [Unreleased]

### Added

- A cinematic **connect entrance**. Every caller (telnet and ssh) now lands on a
  short, paced modem handshake (`CONNECT 57600/ARQ/V90/LAPM/V42BIS`), then the
  **flagship loginscreen** -- a full 80x30 piece that scrolls as it paints: a
  shaded gradient `VENDETTA` block wordmark with a drop shadow over a framed
  scene panel whose stats are **live** (nodes online, total users, total calls,
  spliced via `|CN`/`|TU`/`|TC` tokens) -- gated on a keypress, and finally the
  login matrix, which **also paints on** instead of snapping into place. Any
  keypress skips ahead, and a hotkey hit mid-paint both skips _and_ selects its
  menu option, so it never slows a regular down.
- The **main menu now paints on** the first time a caller lands on it (redraws
  after backing out of a sub-area stay instant).
- A **new-user welcome ceremony**: the instant an account is created, the caller
  gets a paced "credentials issued / ACCESS GRANTED" sequence, the entry-granted
  banner painted in with their handle/location/first call, and the access they
  were just assigned -- turning a one-line "account created" into an arrival.
- A live **status footer** on the login matrix: nodes online, total users, and
  the board's local time, spliced in fresh on every connection.
- New session-layer animation primitives in `internal/term` -- `Sleep`,
  `WaitKey` (an interruptible timed wait that tells a read deadline from a real
  hangup, pushing the skip key back for the next read), `WaitAnyKey` (a
  splash "press a key" gate), and `Reveal` (a marker-preserving, skippable
  line-by-line screen paint) -- with tests covering pacing, skip, and pushback.

## [0.9.0] - 2026-06-22

The first feature-complete pre-release of the Vendetta/X Go server: three faces
(telnet :2323, ssh :2222, web :8080) over one SQLite spine, with every main-menu
command wired and a full sysop configuration program. Pre-1.0 -- everything is
built, but it hasn't yet had a real-world shakedown.

### Added

- Wired every main-menu command end to end: message bases, file areas (uploads
  plus signed web downloads), email, voting booth, BBS list, G-files, doors,
  QWK offline mail, new-files, oneliners/the wall, teleconference, user list,
  last callers, your stats, system info, and settings.
- Added the full **sysop configuration program** on the web face at `/sysop`:
  CRUD over message bases, file areas, users, G-files, the BBS list, voting
  polls, and the wall, plus global settings — including **per-feature on/off
  toggles** for email, voting, gfiles, bbslist, doors, qwk, newfiles, oneliners,
  and teleconference.
- Added real **QWK offline mail**: packet download and `.REP` reply-upload.
- Added **external/DOSBox door support**, including drop-file generation so
  legacy doors can run against the live board.
- Added **end-to-end board tests** covering the wired menu flows.

### Security & hardening

- TLS for the web face (`-tls-cert`/`-tls-key`, or behind a terminating proxy)
  with `Secure` session cookies; HTTP read/header/write/idle timeouts.
- Per-IP **login throttling** on web and telnet; reserved/validated handles.
- Concurrent-session cap (`-max-nodes`) and an idle-session watchdog (`-idle`)
  over telnet/ssh; a telnet "press ESC twice to connect" gate that drops bots.
- Control-byte **sanitization** of user text so ANSI escapes can't reach other
  callers' screens; a **zip-bomb guard** on QWK reply import.
- Per-session **panic recovery**, a **sysop audit log**, and **graceful
  shutdown** that drains HTTP and flushes the database; SQLite `busy_timeout`.

### Changed

- Door drop files (DOOR.SYS / DORINFO1.DEF) now take their system name, sysop
  name, and DOS path from board settings / per-door config instead of
  hardcoded values.
- Archived the original C/DOS implementation under `legacy/` so the repository
  root is Go-forward; CI now builds and tests the Go server as well.

### Removed

- Removed stray compiled C build artifacts (`*.o` object files) that had been
  left in the repo root, and updated `.gitignore` so they are never recommitted.
