# VENDETTA/X — The Master Build Plan

*How we make this the best board on the wire — and everything an AI (or human)
builder needs to pick up any item and ship it.*

This document has three parts:

- **Part I — State of the board.** An accurate inventory of what is already
  built and verified, so nobody re-plans shipped work. (The gap tables in
  `BBS-FEATURE-STUDY.md` §2–3 predate the 0.9.0 Go server and are stale; this
  section supersedes them.)
- **Part II — The builder's playbook.** The recipes: how to add a feature
  package, a telnet screen, a web page, a setting, a scheduled action, a piece
  of art — and how to *verify* work the way this repo verifies work.
- **Part III — The roadmap.** Prioritized epics, each written as a
  self-contained build spec: why, design, data model, files to touch,
  verification, size, dependencies.

The product bar (from `DESIGN.md`) still governs everything here: **one dataset,
many renderings · runtime-configurable · scene-authentic · hardened**.

---

## Part I — State of the board (what is DONE)

One Go binary (`server/`), pure-Go SQLite spine, three faces at once:
telnet `:2323`, ssh `:2222`, web `:8080`. Version 0.9.0.

### Session & transport
- Telnet IAC codec, anti-bot "press ESC twice" gate, per-session panic
  recovery, node cap (`-max-nodes`), idle watchdog (`-idle`).
- SSH face (`internal/sshface`): persistent host key, permissive transport auth
  (board runs its own bcrypt login), `TERM`-based charset pick.
- **CP437 ↔ UTF-8 auto-detection** (`internal/term/charset.go`): CPR probe on
  telnet, `TERM` on ssh; output transcoded, input folded back to CP437.
- Cinematic connect: modem-handshake beats → flagship skeleton/logo
  loginscreen with live stat tokens → animated login matrix. All skippable,
  hotkeys select-through.
- Lightbar menu engine with spatial arrow navigation, `.pp` pipe-code renderer
  (`internal/render`), animation primitives (`Reveal`, `WaitKey`).

### Users & access
- bcrypt auth + password policy; per-IP login throttle shared across all
  three faces; reserved handles; console-only first-password enrollment for
  privileged accounts (anti-takeover).
- **ACS** (`internal/acs`): full Iniquity-style evaluator — `s`/`d`/`f`/`u`/
  `a`/`l`, AND/OR/NOT/parens — gating boards, areas, and doors identically on
  every face.
- User record: handle, real name, location, email, tagline, group, SL/DSL,
  flags, call/post counters, UL/DL byte counters.

### Messaging & community
- Message bases with read/post ACS, one-at-a-time reader with prev/next/reply,
  full-screen ANSI editor (`internal/editor`, arrows/home/end, Ctrl-Z save),
  new-scan across readable bases, numbered base picker.
- **Private mail** (`internal/mail`): inbox/outbox/compose/reply/delete,
  unread counts surfaced at logon.
- **QWK offline mail**: real `.QWK` build + `.REP` parse (`internal/qwk`),
  download/upload on the web face, honest summary screen on the terminal.
- **QWK-net** (`internal/qwknet`): the board is a QWK network node — exports
  local posts as `.REP` to a hub over FTP (`internal/ftp`), imports the hub's
  `.QWK`, with loop/dupe protection and a high-water mark. Scheduler-driven.
- Teleconference (`internal/chat`): live multi-node chat with `/w`, `/q`.
- Voting booth, BBS list, g-files library, logon bulletins, oneliner wall,
  user list, last callers, leaderboards (`internal/social`), profile cards.

### Files & transfer
- File areas with ACS, upload/download, new-files scan; file bytes stored in
  SQLite; signed time-limited HTTP download links on the web.
- **Pure-Go ZMODEM** (`internal/zmodem`, proven against lrzsz) wired into the
  terminal for uploads *and* downloads over an 8-bit-clean telnet/ssh bridge.
- **Ratio economy** (`store/ratio.go`): UL/DL byte accounting, configurable
  ratio + free allowance, sysop-exempt, enforced identically on both faces.

### Doors & games
- External door layer (`internal/door`): DOOR.SYS / DORINFO1.DEF drop files,
  PTY bridge (dosemu2-ready), graceful failure. Sysop CRUD at `/sysop/doors`.
- **Two full native door games**: Red Dragon (LORD-style town/forest/PvP/
  master-challenge RPG) and Voidfarer (TW2002-style shared-galaxy trader),
  each with its own persistent tables and daily turn resets. Plus two
  mini-games (Guess the Vault, Dice Duel).

### Sysop & operations
- Full web sysop panel (`/sysop`): CRUD over users, boards, areas, g-files,
  bulletins, doors, BBS list, voting, oneliners; global settings; per-feature
  on/off toggles; QWK-net config + run-now; **event scheduler** CRUD
  (daily-at-HH:MM and every-N-minutes actions: message purge, oneliner trim,
  QWK-net exchange). Audit log of every sysop mutation. CSRF guard.
- Hardening: sanitization of all stored user text (`internal/sanitize`),
  parameterized SQL, TLS option, graceful shutdown with WAL checkpoint.

### Art & identity
- The `.tpl → .pp/.ans` pipeline in `legacy/tools/` (see Part II §7): TDF
  wordmark rendering, the `dither.py` ACiD-texture primitives, `ansi2png.py`
  pixel-exact preview, `render_dump.c` render-through-the-real-engine.
  Every screen restyled in the board identity; web face carries the same
  wordmark and palette.

### Known real gaps (verified against the code, 2026-07)
No per-user **read pointers** (new-scan is not resume-where-you-left-off), no
threading/quoting beyond `Re:` prefill, no message search, no **page sysop** /
node-to-node messages, no WFC screen, no **NAWS** (window-size) awareness, no
FILE_ID.DIZ extraction or upload-approval queue, no file search, no IP-ban /
trashcan management UI, no validation/time-limit system, no FidoNet, no web
live-presence (SSE), no art gallery, no guest mode, no automessage, no
feedback-to-sysop channel, no scheduled DB backup. These are the raw material
for Part III.

---

## Part II — The builder's playbook

Everything below is the *house pattern*. Follow it and a feature lands looking
like it was always here.

### 1. Build, test, run

```sh
cd server
CGO_ENABLED=0 go build .        # static binary -> ./server
go vet ./... && go test ./...   # every package must stay green
./server -db /tmp/vx-dev.db     # telnet :2323, ssh :2222, web :8080
```

- E2E: `main_test.go` drives full sessions over `net.Pipe` (signup, login,
  menus, teardown) — extend it when you touch session flow.
- Manual: `telnet localhost 2323` (a real capture beats assumptions — pipe a
  session recording through `legacy/tools/ansi2png.py` to *look* at it).
- Legacy C tree builds/tests separately; you almost never touch it. CI is
  `.github/workflows/build.yml`.

### 2. The feature-package recipe (the most important pattern)

Every optional feature is an isolated package owning its own table over the
shared `*sql.DB`. To add feature `foo`:

1. `server/internal/foo/foo.go` — `type Store struct { db *sql.DB }`,
   `New(db) (*Store, error)` creates its table(s) (idempotent
   `CREATE TABLE IF NOT EXISTS`, `ADD COLUMN` migrations in a loop that
   tolerates duplicate-column errors — copy the idiom from
   `internal/store/store.go`). Small CRUD API. Sanitize all user text with
   `internal/sanitize` at the write boundary. Times are Unix seconds.
2. `server/internal/foo/foo_test.go` — unit tests over an in-memory SQLite
   (`sql.Open("sqlite", ":memory:")`), same as every sibling package.
3. `server/server_foo.go` — the telnet UI: a function on `*board` taking
   `*term.Session`, using `b.screenHeader(s, "title")`, `cp437rule(72)`,
   the established `\x1b[1;36m`-family color idiom, `s.Pause()`.
4. `server/internal/web/web_foo.go` + `templates/pages/foo.html` — the web
   rendering; register routes in `web.go`; reuse the `panel` / `sg-note` CSS
   vocabulary.
5. Wire into `main.go`: construct the store, add a main-menu lightbar entry
   (hotkey must match a `|{r,c,K,Label}` marker in `art/mainmenu.pp` — see §7),
   and gate with `store.FeatureEnabled` + a toggle in the Features registry
   (`internal/store/settings.go`) so the sysop can turn it off.
6. Sysop surface if needed: `web_sysop_foo.go` + template + nav entry in
   `sysop-nav`, always writing through the audit log like its siblings.

Study `internal/voting` + `server_voting.go` + `web_voting.go` end-to-end
once; every feature is that same sandwich.

### 3. Adding a setting

`internal/store/settings.go`: typed getters over the `settings` table
(`SettingStr/Bool/Int` with defaults). Expose sysop-editable ones in
`web_sysop_settings.go` + `sysop_settings.html`. Never invent a config file —
runtime settings live in the DB; process-level knobs are flags in `main.go`.

### 4. Adding a scheduled action

1. Add the action to the Catalog in `internal/schedule`.
2. Implement the handler in `server/server_schedule.go`'s dispatch table (the
   one place that can see every store). Handlers log-and-skip on error; the
   loop must never die.
3. It's now creatable at `/sysop/events` (daily HH:MM or every-N-minutes).

### 5. Terminal UX rules

- Write once against `term.Session`; telnet and ssh both get it for free.
- Never raw-sleep: `s.Sleep`/`WaitKey` are carrier-safe and skippable. Any
  reveal/animation must skip on keypress (`Reveal`) — never slow a regular.
- 8-bit output is CP437; the charset layer transcodes for UTF-8 terminals —
  emit CP437 bytes (`\xC4`, `\xFE`, …), never UTF-8 literals.
- All user-entered text passes `sanitize.Line`/`sanitize.Text` before storage.
- ACS-gate everything on *every* face — the evaluator is cheap; call it.

### 6. Web rules

- `html/template`, one isolated template set per page over `base.html`.
  No JS frameworks; the design system is `static/*.css` (see `/styleguide`).
- Unsafe methods require the CSRF same-origin check (comes free via the
  existing handler shell). Sysop routes go through the `s.admin` gate and the
  audit log.

### 7. The art pipeline (screens are generated, not hand-hacked)

```
legacy/tools/mk*.py  ──(dither.py primitives + TDF wordmark)──▶  art/*.tpl
legacy/tools/tpl2ans.py  ──▶  art/*.pp / *.ans      (CP437, what the board loads)
legacy/tools/ansi2png.py ──▶  shots/*.png            (pixel-exact review)
```

- To change a screen, edit its generator (`mkloginscreen.py`, `mkmainmenu.py`,
  `mksubmenus.py`, `mkmsgframes.py`, …) and regenerate — never hand-edit `.pp`.
- Lightbar options live in the art as `|{row,col,hotkey,Label}` markers; the
  Go code only supplies handlers per hotkey. Adding a menu entry = regenerate
  the `.pp` with a new marker + handle the key in `main.go`.
  **Exception: the main menu.** `art/mainmenu.pp` only reserves the slot
  layout (`mkmainmenu.py`'s `LEFT_SLOTS`/`RIGHT_SLOTS`) behind a single
  `@@MENU_OPTIONS@@` placeholder; which action/label/hotkey fills each slot
  is sysop-editable data (`internal/menu`, `/sysop/menu`), spliced in by
  `server_menu.go` on every render — no regeneration needed to rename a
  command or rebind its key. Regeneration is only needed if the *slot
  count* itself changes (a new command needing a 20th slot, say), which
  also means updating `mainMenuSlotPos`/`mainMenuDefaults` in
  `server_menu.go` and `menu.MainMenuSlots` in lockstep.
- Verify visually every time: render through the board (or the `.ans`
  directly) into `ansi2png.py` and *look at the PNG*. For live-token screens,
  capture a real telnet session and render the capture.
- Fonts: `art/fonts/CYBRCRME.TDF` is the wordmark; `render_tdf.py` /
  `curate_tdf.py` help pick faces for new pieces.

### 8. Definition of done (house standard)

1. `go build` + `go vet` + `go test ./...` green, with new unit tests.
2. Feature works on **all applicable faces** (telnet, ssh, web) and respects
   ACS + feature toggles.
3. Exercised end-to-end at least once against a real running server (telnet
   capture or browser), not just tests.
4. Sysop-configurable where it should be; audit-logged where it mutates.
5. `CHANGELOG.md` entry under `[Unreleased]`.

---

## Part III — The roadmap

Epics are grouped in five tiers, ordered by what makes a board *the best*:
**(A)** regulars come back every day, **(B)** the files/doors scene is real,
**(C)** the board is part of a living network, **(D)** the sysop can run it
for years without fear, **(E)** it out-dazzles everything else on the wire.

Sizes: **S** = a day-ish, **M** = a few days, **L** = a week+.

---

### TIER A — The daily-driver loop (highest value, build first)

#### A1. Read pointers (qscan) + real new-scan — **M** — *the single biggest UX win*

**Why.** Regulars live by "show me what's new since I was last here." Today
the new-scan has no memory; a caller re-reads or misses things. Every classic
board had qscan pointers; no serious board lacks them.

**Design.** New table `lastread (user_id, board_id, last_msg_id, PRIMARY KEY
(user_id, board_id))` in `internal/store`. Reading a message advances the
pointer (monotonically — `MAX(last_msg_id, id)`). New-scan iterates readable
bases, jumping straight into the reader at the first message `> last_msg_id`,
with `[N]ext-base / [Q]uit` flow. Show per-base unread counts in the base
picker (`3 new`) and a total on the main menu / logon ("`14 new msgs in 3
bases`"). Web: unread badges on `/boards`, "jump to first unread" per board.

**Touch.** `internal/store` (table + `LastRead`/`SetLastRead`/`UnreadCounts`),
`main.go` (reader advance, new-scan rewrite, picker counts, logon line),
`web_boards.go` + templates. Tests: store unit tests + a `main_test.go`
session that posts, re-logs, and asserts the unread count.

**Verify.** Two-user telnet session: user A posts, user B's next logon shows
the count, new-scan lands on the post, count clears.

#### A2. Threading & auto-quote — **M**

**Why.** Arguments are the lifeblood of a board; they need reply chains and
`>` quoting to be followable.

**Design.** Add `reply_to INTEGER` to `messages` (idempotent `ADD COLUMN`).
Reply flows pass the parent id. Reader gains `[T]hread` — walk
parent/children. Composer pre-fills the editor buffer with the parent quoted
(`nut> the line`, wrapped to 72, capped ~20 lines) above the cursor. Web board
view renders indented threads (one level of indent is enough — this is a BBS,
not a forum tree).

**Touch.** `store` (column + `Thread(id)` query), `server_msgread.go`
(thread nav + quote prefill via `editor.New` initial content — the editor
already accepts initial lines), `web_boards.go` (thread render). QWK export
keeps working (reply-to maps to the packet's reference field in
`internal/qwk`).

#### A3. Node-to-node: page the sysop, node messages, sysop break-in chat — **M**

**Why.** The moment two people are on a board at once, they want to poke each
other. "Page sysop" is *the* iconic BBS interaction, and the main menu already
promises it.

**Design.** Extend the presence registry in `main.go` (it already tracks
per-node handle/activity) with a per-node inbox channel. Features:
`/P`age sysop (reason line; if a sysop is online on any face it beeps/notifies
their terminal between keystrokes and queues a web notice; else it becomes a
feedback mail — see A4); node message (`send <node> <text>`, delivered at the
recipient's next prompt via the same interruptible-read hooks `WaitKey`
uses); sysop break-in chat (privileged user picks a node → two-way split
chat reusing `internal/chat`'s hub with a private channel). Who's-online
shows node numbers + current activity string (set it on menu transitions:
"reading msgs", "in Red Dragon", …).

**Touch.** `main.go` presence struct (activity string + notify channel),
`internal/term` (a `Notify` hook that flushes queued lines before the next
read — the idle-watchdog plumbing shows the shape), `internal/chat` (private
channels), `server_page.go` (new). Feature-toggle `paging`.

#### A4. Feedback channel + automessage — **S**

**Why.** Two classic staples, both nearly free on existing plumbing.

**Design.** *Feedback*: main-menu/matrix option that composes mail to the
sysop account (mail package already does everything; this is a thin flow that
also catches offline pages from A3). *Automessage*: one board-wide
announcement any (ACS-gated) caller can claim — table `automessage(author,
body, set_at)`, shown in the logon walk (`server_logon.go`) after bulletins,
with `[A]utomessage` write access from the main menu. Sysop can lock it via
setting.

#### A5. Live activity everywhere — **M**

**Why.** A board feels *alive* when you can see it move. We have the data;
show it.

**Design.** (a) Terminal: logon walk gains a "since your last call" digest —
new posts count (from A1), new files count, new callers, latest oneliner.
(b) Web: `/` home gets a live activity feed (recent posts/uploads/callers
from existing queries) and **SSE presence** — a `/events` endpoint pushing
who's-online deltas; the header's node count updates without reload. SSE is
stdlib-only (flush-loop handler), no frameworks, honors the no-JS-framework
rule with a ~20-line vanilla script.

**Touch.** `web_home.go` + `web_events.go` (new SSE), presence hook in
`main.go` (subscribe/unsubscribe on connect/disconnect — the chat hub pattern
is the model), `server_logon.go` digest.

---

### TIER B — The scene board (files & doors that earn the name)

#### B1. Upload intake: FILE_ID.DIZ, dupe check, approval queue — **M/L**

**Why.** Warez-board authenticity *and* sysop safety. Right now an upload goes
live instantly with a hand-typed description.

**Design.** On upload (both faces): if the payload is a ZIP (magic check,
`archive/zip` over the in-DB bytes), extract `FILE_ID.DIZ` (case-insensitive,
size-capped, `sanitize.Text`) as the description, preserving hand-typed input
as fallback. Dupe check by (filename) and by SHA-256 content hash (new
`hash` column) — duplicate uploads are refused with the original's area named.
New `approved INTEGER DEFAULT 1` column; a setting `files.moderate` flips new
uploads to `approved=0`: invisible to listings/downloads until the sysop
approves at a new `/sysop/uploads` queue (approve = live + uploader gets a
mail; reject = delete + mail with reason). Uploader ratio credit lands on
approval, not upload (prevents credit-farming garbage).

**Touch.** `internal/store` (columns, `PendingFiles`, approval API),
`server_transfer.go` + `web_files.go` (intake path shared via a new
`internal/store` helper or a small `internal/upload` package),
`web_sysop_uploads.go` (new). Tests: zip-with-DIZ fixture, dupe-hash refusal,
moderation visibility.

#### B2. File find + batch download — **M**

**Why.** Past a few hundred files, browsing dies without search; batch is how
real leeches leech.

**Design.** `[F]ind` in the file menu: substring search over
filename+description across ACS-readable areas (SQL `LIKE`, paged results,
download-by-number from results). Tagging: in any listing, `T n` toggles a
file onto the session's tag list (shown as `√`); `[D]ownload` with tags queued
sends them as one **ZMODEM batch** (the protocol and our `internal/zmodem`
already frame per-file — loop `Send` per file inside one transfer window),
with a summed ratio pre-check. Web equivalent: search box on `/files` +
multi-select → zip-on-the-fly download (`archive/zip` streaming).

**Touch.** `store` (`SearchFiles`), `main.go` file menu, `server_transfer.go`
(batch loop), `web_files.go`. Verify against real `rz` for the batch framing.

#### B3. Door ecosystem, phase 2: FOSSIL/socket bridge + door scores — **L**

**Why.** dosemu2 console doors work today; the classics that *only* speak
FOSSIL/COM (many LORD-era doors) need the serial path `docs/DOORS.md` already
promises. And door scores on the board turn doors into community.

**Design.** (a) New door I/O mode `socket`: the door config grows a
`Bridge` field (`pty` | `socket:<port>`); for socket mode, the launcher
starts the command (DOSBox-X headless with a `nullmodem` serial config),
dials the TCP endpoint with retry/timeout, and bridges caller ⟷ socket using
the same copy-pump as the PTY path (`internal/door/run.go` — the pump is
already transport-shaped). (b) Drop-file *re-read*: after the door exits,
re-read DOOR.SYS-adjacent score files where known. (c) Native-game
leaderboards on the web: Red Dragon and Voidfarer rankings pages
(`/doors/dragon`, `/doors/void`) from their existing tables — cheap, high
delight.

**Touch.** `internal/door` (bridge mode + socket launcher + tests with a fake
TCP echo "door"), `web_sysop_door_edit` template (mode field), new
`web_doors.go` leaderboards. `docs/DOORS.md` update with a worked DOSBox-X
example.

#### B4. File points & the elite economy (optional flavor) — **S/M**

**Why.** Ratio is a stick; points are a carrot. Classic boards ran on both.

**Design.** Setting-gated (`economy.points`, default off). Earn: N points per
approved-upload MiB (B1), M per post (spam-guarded by daily cap). Spend:
download when ratio-blocked (points buy bytes), Red Dragon gold exchange
(flavor!), reserved-handle color in oneliners. All constants sysop-tunable
settings. Points column on `users`, ledger table for audit.

---

### TIER C — The living network

#### C1. FidoNet-style echomail over BinkP — **L** — *the flagship network feature*

**Why.** QWK-net is in; FTN (FidoNet Technology Network) is the deeper scene
network — fsxNet/AgoraNet/ArakNet are alive *today* and boards on them get
daily cross-board traffic. This is the difference between "a BBS" and "a node
in the culture."

**Design.** Two new packages, mirroring the QWK-net shape exactly:
- `internal/ftn`: FTS-0001 packet (`.PKT`) read/write + echomail semantics —
  AREA tag line, origin/tearline, SEEN-BY/PATH (parse, append self, honor for
  dupe/loop control), MSGID/REPLY kludges (map to A2's `reply_to`), zone:net/
  node.point addressing.
- `internal/binkp`: minimal BinkP 1.0 client (TCP :24554) — frame codec,
  plain + CRAM-MD5 auth, send/receive file batches, resume not required for
  v1.
- `server_ftn.go`: the exchange (mirror `server_qwknet.go`): export local
  posts from FTN-mapped boards into `.PKT` bundles, session with the uplink,
  import inbound bundles with dupe control (MSGID table high-water, SEEN-BY).
  Scheduler action `ftn.exchange`; sysop panel page for uplink address/
  password/echo↔board map (mirror `sysop_qwknet.html`).

**Touch.** As above + `store` reuses the `origin` column from QWK-net for
tagging. Tests: golden `.PKT` fixtures round-trip; a fake BinkP server in
tests (the fake-FTP-hub test from QWK-net is the template). Verify against a
real fsxNet hub in a burn-in before shipping default-on… ship default-off.

#### C2. Feeds & read-only reach — **S**

**Why.** Let the open web *see* the board breathe; costs almost nothing.

**Design.** `/feed.atom` (stdlib `encoding/xml`): recent public posts from
world-readable (empty-ACS) bases only, plus new-file announcements. Setting-
gated. Correct `Content-Type`, entry ids stable (`msg-<id>`).

#### C3. Web terminal — the full ANSI board in a browser tab — **L**

**Why.** The web face is a *rendering*; the terminal is the *soul*. One
`<canvas>`/xterm.js page that speaks to the telnet engine gives every drive-by
visitor the real experience — skeleton splash, lightbars, doors — with zero
client install. This is the single best funnel from "curious" to "regular."

**Design.** `/terminal` page embedding a vendored xterm.js (one static JS
file — acceptable exception to the no-framework rule, sysop-toggleable) +
a WebSocket endpoint (`/ws-term`). Server side: accept the WS (a minimal
RFC 6455 codec in `internal/web` — stdlib-only is ~300 lines, or vendor
`golang.org/x/net/websocket`), then hand the conn to the *existing* board
entry as an `io.ReadWriteCloser` — exactly how `sshface` hands over a
channel (`term.NewRW`). Client sets UTF-8; our charset layer already
transcodes. Throttle + node-cap apply as usual (it's just another caller).

**Touch.** `internal/web` (WS endpoint + page), `main.go` (a `runBoardRW`
entry that skips telnet IAC — the ssh path already is this). Verify: full
session in a browser incl. lightbars, editor, and a Red Dragon fight.

---

### TIER D — Run it for years (trust & ops)

#### D1. Ban & trashcan management — **M**

**Why.** A public board *will* be abused. Throttle slows attackers; the sysop
needs durable tools.

**Design.** `internal/guard` package: `bans` table (kind: `ip`/`cidr`/
`handle-pattern`, value, reason, expiry nullable) + `trashcan` word list
(blocked handle words, applied at signup on all faces). Enforcement points:
pre-gate on telnet/ssh accept (before the ESC gate — banned IPs get a curt
`NO CARRIER` and a close), web login/register. Sysop page `/sysop/bans`:
CRUD + "ban this IP" one-click from the audit/last-callers views. Store last
IP per user (column exists conceptually — add `last_ip` on `users`) so
handle→IP bans are one click. All bans audit-logged.

#### D2. Validation levels & time limits — **M**

**Why.** The classic trust ladder: new users get a taste, validated users get
the board. Also the fairness lever every busy board eventually needs.

**Design.** Settings: `newuser.sl` already exists — add `validated.sl`,
`session.minutes` / `daily.minutes` per SL band (a small `limits` table:
`min_sl, session_min, daily_min, daily_calls`). Enforcement in the session
loop: a per-session countdown (the idle watchdog shows where the clock
hooks); soft warning at 5 min, clean goodbye at 0; daily accounting on
`users`. Sysop quick-validate: `/sysop/users` gains a one-click "validate to
SL n" (and A3's page pipeline notifies a caller when they're validated live).
A `feedback`-driven flow: new users are nudged to send feedback (A4);
answering it is where the sysop validates. Guest mode: a `guest` account
(setting-gated) that skips password and gets read-only ACS.

#### D3. Backup, health & the ops story — **S**

**Why.** One SQLite file is the whole board; losing it is losing the
community. Make not-losing-it a built-in.

**Design.** Scheduler action `db.backup`: `VACUUM INTO
'backup-YYYYMMDD.db'` (SQLite-native, safe under WAL) into a settings-defined
dir, keep-last-N rotation. `/healthz` endpoint (200 + node count + db ping)
for uptime monitors, unauthenticated but boring. Document a systemd +
restore runbook in `deploy/README.md`.

#### D4. Structured audit & stats for the sysop — **S**

**Why.** The audit log exists; make it *readable*, and give the sysop the
numbers a board lives by.

**Design.** `/sysop/audit`: filterable table (actor, action, target, when)
over the existing log. `/sysop/stats`: calls/posts/uploads per day (30-day
sparkline tables from existing timestamps), top boards, storage size. Both
read-only, no new writes.

---

### TIER E — Spectacle & polish (the "best board" delta)

#### E1. WFC screen — the sysop's cockpit — **M**

**Why.** The Waiting-For-Caller screen is BBS iconography *and* genuinely
useful ops: a live dashboard where the board runs.

**Design.** `./server -wfc` (or auto on a controlling tty): a full-screen
CP437 dashboard on the *local console* — board name in TDF, nodes + who/
activity (live), today's calls/posts/uploads, mail waiting, last 5 callers,
scheduler next-runs — refreshing via the presence hooks from A5. Local
hotkeys: `[C]hat` (break-in, A3), `[E]dit user`, `[Q]uit server`. Built as
one more `term.Session` over stdin/stdout (`io_local` heritage), art from a
new `mkwfc.py` (there is already a `wfc.ans` seed to grow from).

#### E2. NAWS + big-terminal layouts — **M**

**Why.** Modern terminals are 120×40+; honoring the real size makes every
screen better, while 80×24 stays the guaranteed floor.

**Design.** Telnet: negotiate NAWS (option 31) in the IAC codec, store
rows/cols on `Session` (dynamic updates too); ssh: `pty-req` already carries
dimensions — parse them (window-change requests update). Renderer/UI use it:
pagination lengths (`more` prompts) from real rows; the reader wraps to real
cols; art stays 80-col centered with a soft margin fill. No layout forks —
just stop hardcoding 24/80 in the half-dozen places that do.

#### E3. Art gallery + SAUCE — **M**

**Why.** A scene board must *show art*. We have a renderer, a file store, and
an aesthetic; a gallery is the natural trophy room.

**Design.** New g-files-like package `internal/gallery` or (simpler) a
`gallery` file-area flag: `.ans` files in flagged areas get `[V]iew` in the
terminal (render via `s.RenderScreen` with pacing = the `Reveal` typewriter
mode, honoring SAUCE for width/title/author) and a web viewer that renders
`.ans` → PNG server-side reusing the exact `ansi2png.py` logic **ported to
Go** (small: CP437 font blit — embed `VGA8.F16`) so the web shows pixel-true
art. SAUCE: a tiny parser (128-byte trailer) surfaced as title/author/group
credits in listings.

#### E4. Per-user look & feel — **S/M**

**Why.** Elite boards let regulars make the place theirs.

**Design.** Settings screen gains: theme choice (the renderer's `|T1–|T9`
slots already exist — store a per-user palette name that remaps them at
session start), expert mode (skip menu art, prompt-only — one flag honored in
the menu loop), pause-on-page toggle, 24h clock. All columns on `users`,
all honored on both faces where meaningful.

#### E5. Achievements & board culture — **S**

**Why.** Cheap dopamine that fits the fiction: "100 posts — CO-CONSPIRATOR."

**Design.** `internal/badges`: pure derivation from existing counters (posts,
calls, uploads, dragon master-levels, voidfarer net-worth) — no new state,
just thresholds → titles shown on profile cards (`internal/social` renders
them) and the web profile. One nightly scheduler action stamps "earned"
mail so the caller finds out on logon.

#### E6. Birthdays & anniversaries — **S**

**Design.** Optional birthday on the profile; logon greeting + a who's-online
`*` on the day; "board anniversary" (first-call date) called out on the
profile card. Pure flavor, ~a day, disproportionate charm.

---

## Suggested build order

Dependencies flow left→right; items in the same step are independent (good
for parallel agents):

```
1. A1 qscan        A4 feedback/automsg     D3 backup+health
2. A2 threading    A3 node/page/chat       D1 bans
3. A5 activity+SSE B1 upload intake        D4 audit views
4. B2 find+batch   D2 validation/limits    E4 per-user prefs
5. C1 FTN/BinkP    B3 door bridge          E2 NAWS
6. C3 web terminal E1 WFC                  E3 gallery+SAUCE
7. C2 feeds        B4 points               E5 badges   E6 birthdays
```

Rationale: Tier A first because retention compounds — every later feature is
worth more on a board people already return to daily. D1/D3 ride early
because a public board needs them the day it gets popular, not after. The
big set pieces (C1, C3, E1) land once the daily loop is strong enough to
deserve them.

Each item above is written to be buildable in isolation by an agent that has
read Part II. When one ships: changelog entry, all-faces verification, and —
per house tradition — a screenshot of the new screen rendered through
`ansi2png.py`.

*— dpl productions · est. MMXXVI*
