# Changelog

All notable changes to Vendetta/X are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and the project aims to follow
semantic versioning.

## [Unreleased]

### Added

- **"You have messages waiting" -- a personal scan.** Public posts addressed to
  you by name (a reply to your post, or anything sent to your handle) are now
  surfaced the classic way: the logon greeting counts them and offers to read
  them right there (`read now? [y/N]`), and the message base picker grows a
  **[Y]ours** command that lists every post addressed to you across the bases
  you can read and opens any one in the reader. ACS-scoped like everything else
  -- a gated base never shows up -- and the broadcast "All" recipient never
  counts as personal.

- **A classic "more" pager on the terminal.** Long message posts and g-files no
  longer scroll their own top off an 80x24 screen: the reader now pauses a page
  at a time with the old-school `-- more --` prompt. Space (or N) pages forward,
  Enter reveals one more line, C dumps the rest continuously, Q/Esc stops. Short
  posts that already fit are untouched -- the prompt only appears when there's
  more than a screenful. Wired into the message reader (so it also covers the
  new-scan walk) and the g-files library.

- **The web terminal: the real board in a browser tab.** A new `/terminal` page
  runs the genuine ANSI board -- the modem connect ceremony, the login matrix,
  lightbar menus, the full-screen editor, the doors -- live in the browser, with
  zero install. It's the actual session, not a rendering: a small stdlib RFC 6455
  WebSocket endpoint (`/ws-term`, no third-party server library) hands its
  upgraded socket to the same board entry the ssh face uses, so the node cap,
  bans, and login throttle all apply. The client is a vendored **xterm.js**
  (embedded in the binary -- the board stays a single self-contained
  executable, no CDN), and because it speaks UTF-8 the session transcodes the
  CP437 art on the way out and folds typed UTF-8 back to CP437 on the way in.
  Sysop-toggleable like every feature.

- **Make the board yours: per-user look & feel.** Two preferences on the
  settings screen, editable over telnet/ssh *and* the web, honored on the
  terminal faces. **Expert mode** drops the hand-holding for regulars who know
  the board -- the main menu skips its line-by-line paint-on and the logon
  walk skips the tour and the quick-logon prompt, going straight to the menu.
  **12-hour clock** renders your times as 3:04pm instead of the 24-hour
  default, on the logon greeting and your stats. Both ride on the users table
  and default off, so nothing changes until you flip them.

- **The web face breathes now.** Three things make the browser side feel as
  alive as the wire. **Live presence:** a Server-Sent-Events stream (`/events`,
  stdlib-only, no framework) pushes the who's-online list, and a ~20-line
  vanilla script repaints the masthead node bar as callers come and go -- no
  reload. **A fuller front page:** the home dashboard gains a "fresh uploads"
  feed beside recent messages, so the newest files show the moment they land.
  **Reach for the open web:** a public `/feed.atom` (sysop-toggleable) carries
  recent posts from world-readable bases only -- nothing gated ever leaks --
  with a discovery `<link>` in every page head.

- **The sysop cockpit: stats + a real audit trail.** Two new read-only pages
  in the panel. `/sysop/stats` shows the board at a glance -- total users,
  messages, files, bytes stored -- over a 30-day activity sparkline (posts,
  uploads and signups side by side) and a "busiest bases" ranking, all derived
  from timestamps already on disk. `/sysop/audit` turns the audit log from a
  stdout line into a durable, filterable table: every state-changing action in
  the panel is now recorded (actor, method, path, source IP) to its own table
  and read back newest-first, filterable by actor -- so "who changed this, and
  when?" finally has an answer.

- **Birthdays & board anniversaries.** Set an optional birthday (month-day only
  -- the board never stores a year, so it can't know your age) on the settings
  screen over telnet/ssh or the web, and on the day the logon greeting rings it
  out ("-!- happy birthday -!-"); the web profile shows it with a "today!" star.
  Your **board anniversary** rides along for free, derived from your first-call
  date: "6 years on the board today -- thanks for still calling." Pure flavor,
  disproportionate charm.

- **Achievements.** Callers now earn scene-flavored badges as their counters
  climb -- "100 posts -> CO-CONSPIRATOR", "50 uploads -> COURIER", a GB shared
  -> WAREZ BARON, plus a STAFF mark for the operator. They're pure derivation
  from the numbers already on your record (no new tables, no backfill, and a
  badge lights up the instant you cross its line), and they show on your stats
  screen over telnet/ssh and as a strip of star chips on the web profile. Each
  ladder shows only the top rung you've reached, so a card stays a tidy handful
  of titles rather than a wall.

- **Board-wide search, on every face.** Once a board fills up, "where was that
  post about ratios / that keygen?" is the question no listing answers. Now the
  message base picker grows a **[/]find** and the file listing an **[F]ind**:
  type a term and the board searches subjects, bodies and handles (messages) or
  filenames, descriptions and uploaders (files) across everything at once, then
  drops you straight into the reader on a message hit or a ZMODEM download on a
  file hit. On the web it's a first-class **search** page (one box over both
  corpora, message cards + file rows). Crucially the search only ever looks in
  the bases and areas *you* can already open -- ACS scoping is enforced
  identically on telnet, ssh and the web, so a restricted base can never leak a
  line through search -- and typed wildcards (`%`, `_`) match literally. One
  store, three renderings; sysop-toggleable like every other feature.

- **Sysop-configurable main menu.** Which command lives in each of the 19
  telnet/ssh main-menu slots, its label, and the key that picks it are no
  longer baked into the art -- they're editable at `/sysop/menu`, take
  effect on a caller's next redraw, and survive across restarts. A slot can
  be rebound to a different command, relabeled, given a new hotkey, or
  hidden entirely; feature-gated commands (email, doors, voting, ...) still
  honor their own on/off switch under settings no matter which slot they
  land in. `art/mainmenu.pp` now reserves the slot layout (10 left, 9
  right) behind a placeholder the board splices its live bindings into on
  every render, instead of hardcoding 19 fixed labels/hotkeys.
- **FTN message networking: the board joins the big networks.** A from-
  scratch FidoNet-technology stack -- FTS-0001 type 2+ packets with full
  echomail dressing (`internal/ftn`) over a BinkP 1.0 mailer with CRAM-MD5
  (`internal/binkp`) -- covering fsxNet, FidoNet, AgoraNet, ArakNet and every
  other FTN network with one implementation (DOVE-Net was already covered by
  QWK-net). The new sysop / networks page joins any number of uplinks at
  once, each with its own address, password, and echo-to-board map; the
  `ftn.exchange` scheduler action polls them unattended. Locally-posted
  messages flow out with proper MSGID/tearline/origin; inbound echomail is
  MSGID-deduped (our own posts can never echo back in), origin-tagged (so it
  can never be re-exported), and REPLY-kludge threaded onto local messages.

- **Nightly backups, out of the box.** A fresh board ships with a scheduled
  `db.backup` event (04:00): each run writes a consistent `VACUUM INTO`
  snapshot of the whole board into the backup directory (sysop-configurable,
  default `backups/`, keep-last-7) -- safe with callers online, with a
  catch-up run at startup if the board slept through one. Restore runbook
  in `deploy/README.md`, plus a `/healthz` endpoint for uptime monitors.
- **Bans and the trashcan.** The new sysop / bans page lays down durable
  door policy: single-IP and CIDR-range bans (connection dropped before the
  board answers, on every face) and handle patterns no new account may
  contain -- each with a reason and optional expiry. The loopback console
  can never be banned, so a sysop can't lock themselves out.

- **A real goodbye.** Logging off now paints a proper send-off piece --
  GOODBYE in the board's TDF wordmark over the star field, "later,
  <handle>. the wall remembers.", live node/user/call counts -- with the
  modem's own `NO CARRIER` as the last word, instead of a bare one-liner.

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

### Security

- **Same-origin gate on the web-terminal WebSocket.** The `/ws-term` upgrade is
  a GET, which the CSRF guard (only checking unsafe methods) never saw, so any
  cross-origin page could open a readable+writable board session in a visitor's
  browser (cross-site WebSocket hijacking) -- and from a browser co-located with
  the board, drive the loopback enrollment path to claim a still-passwordless
  privileged account. The upgrade now rejects a foreign `Origin` (browsers always
  send it on a WebSocket handshake) with a 403, while still allowing header-less
  non-browser clients. Caught by this branch's own security review.

### Fixed

- **Every art screen double-spaced and smeared on real scene terminals**
  (SyncTERM, TheDraw, anything ANSI.SYS-shaped). Two stacked causes, both
  found from one SyncTERM photo. The big one: the generated art painted all
  **80 columns and then sent a newline** -- ANSI.SYS-family renderers wrap
  the cursor the instant column 80 is written, so that newline opened a
  blank row under every full-width line, stretching the wordmarks to double
  height, shearing the screens ("spaces between each line"), and smearing
  absolute-positioned menu labels into the shifted art. Modern terminals
  defer the wrap until the next glyph, which is why xterm-family screens
  and the PNG previews always looked fine. All generated art now stops at
  **79 columns** (the classic ANSI-scene safe width, capped centrally in
  the art pipeline's serializers), which renders identically on every
  terminal -- verified by replaying captured sessions through both an
  immediate-wrap ANSI.SYS-style emulator and a deferred-wrap vt emulator at
  80x25. Second, the interactive screens were too tall for the terminal
  they actually get: SyncTERM's default 80x25-with-status-line leaves the
  session only **24 rows**, and any screen painting past its bottom row
  scrolls the whole thing up a line -- shearing the already-painted
  lightbar labels one row off the absolute positions the lightbar then
  repaints at, so every menu item showed doubled, one row apart. All the
  menu screens (main, messages, files) plus goodbye now fit within 24
  rows: the `|CL` clear rides the first art row instead of burning its
  own, and the taller screens drop the top eroded bar. Two CI tests keep
  both properties from regressing: one audits every art line's width, the
  other replays each served screen (main menu composed with its shipped
  bindings) through an ANSI.SYS-style 80x24 emulator and fails on any
  scroll or cursor jump past row 24.
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
