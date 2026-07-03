# Changelog

All notable changes to Vendetta/X are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and the project aims to follow
semantic versioning.

## [Unreleased]

### Added

- **Real upload intake on both faces.** A ZIP that carries a `FILE_ID.DIZ`
  now describes itself in the listing (the scene standard; the typed line is
  the fallback), and exact duplicate uploads are refused board-wide by
  content hash before they cost anyone anything.
- **An upload review queue.** Flip "hold new uploads for review" in settings
  and caller uploads wait -- invisible to listings, scans, and downloads on
  every face -- in the new `/sysop/uploads` queue. Approve: the file goes
  live, the uploader's ratio credit lands, and they're mailed the good news.
  Reject: the file is deleted and the uploader is mailed your reason.
  Sysops bypass the queue; credit lands on approval, so junk can't farm
  ratio.

- **The automessage** -- one board-wide shout, claimable by any caller (WWIV
  heritage). It sits above the wall on the oneliners screen; start your wall
  line with `!` to claim it, and whoever claims it last owns it until the
  next caller does. The sysop can clear it from the wall-moderation page.
- **A fuller "since your last call" digest at logon.** Alongside new
  messages and unread mail, the greeting now counts **new files** in areas
  you can see and **new callers** who joined since you were last on.

- **Page Sysop** -- the classic doorbell, now real. `P` from the main menu:
  state your business, the board runs the paging beat, and the page lands in
  the operator's mailbox (subject `PAGE: ...`), with the caller told whether
  an operator is on the board right now. Sysop-toggleable like every other
  feature.
- **Who's-online now shows what everyone is doing.** The node list grew the
  classic Node / Caller / Doing columns -- "in the message bases", "in
  teleconference", "paging the sysop", "in the doors" -- updated live as
  callers move around the board.

- **Threaded replies with classic auto-quote.** Replying now opens the editor
  on the original, `Handle>`-quoted (wrapped, capped, cursor underneath), and
  the post remembers what it answers. The reader footer grows **[T]hread** on
  any reply -- one key walks up to the original. Private mail replies quote
  the same way. On the web, boards render as real threads (replies hang
  indented under their post with a "re:" credit), every message has a
  **reply** link that prefills the quoted form, and a forged/stale reply id
  safely posts as a fresh thread root.

- **Read pointers (the classic qscan).** The board now remembers, per caller
  per base, the last message read. The reader resumes at the oldest unread
  message and advances the pointer as you go; **[N]ew scan** walks every base
  you can read and steps through only what arrived since your last visit
  (skip a base, quit anytime, reply in place). The base picker grew a **New**
  column, the logon greeting counts what's waiting ("3 new in 2 bases", plus
  unread private mail), and on the web the board index shows **"n new"**
  badges with viewing a board catching you up. One pointer store drives all
  three faces.

### Changed

- **The message reader header now wears the menu treatment.** The old
  gradient double-line box is gone; in its place, the same visual language
  as every menu screen: a two-row eroded gradient bar with half-block bite,
  iCE-color hotspots and glints up top, the dithered circuit-trace divider,
  a cyan-to-red gradient rail down the From/To/Subject/Date rows, and a
  bitten half-block base rule under the header. Same row count, same token
  alignment -- message bodies line up exactly as before.

### Fixed

- The main menu's `C` slot was labeled **"Page Sysop" but opened the
  teleconference** -- and the teleconference itself was listed nowhere.
  `C` is now labeled Teleconference (what it always ran), and the new `P`
  Page Sysop entry drives the real paging feature.
- The message and file submenus' lightbars carried hardcoded seeded area
  names instead of the command set the board acts on, leaving Read / Post /
  New Scan (and List & Download / New Files) unreachable from the menu.
  Regenerated both screens with the real commands plus a live
  "current · base" line so callers can see what they're acting on.

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
- A **default `sysop` account** that ships without a password: the operator sets
  it on first login. Because the account is privileged, that first password can
  only be set from the **console** (a loopback connection) -- a remote caller is
  refused, so a passwordless admin can't be claimed by whoever connects first.
  The guard is enforced identically on the telnet/ssh and web faces. (Replaces
  the personal `nut` seed admin, so a fresh install has a generic, usable sysop
  login out of the box.)
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
