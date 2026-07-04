# Vendetta/X BBS Feature Study

A synthesis of mature BBS feature sets (studied from the Iniquity/Mystic/WWIV
lineage, cross-checked against a Renegade/Iniquity-class completeness checklist),
mapped against Vendetta/X's current state, with a prioritized build roadmap.
Vendetta/X is a modern Go BBS served over telnet, ssh, and the web; this study
is the feature reference its roadmap draws on. Much of the "must-have" set below
is already shipping — see `../CHANGELOG.md`.

> **Status note (2026-07):** the gap tables in §2 and the build order in §3
> describe the board as it stood *before* the 0.9.0 Go server; most of Phase 4
> and much of Phase 5 has since shipped. Treat this document as the idea
> backlog / concept reference. The current, code-verified state and the live
> build plan are in **`ROADMAP.md`**.

---

## 1. The Complete BBS Feature Map

What a mature BBS offers, organized by subsystem. Deduplicated across all
findings.

### 1.1 Users & Security

- **User record** — fixed-size, direct-access record: handle/alias, real name,
  location, contact (voice + data phone, address, email), gender, birthday,
  computer type, password.
- **Identity indexes** — names index for O(log n) handle lookup; phone-number
  index for alt/dup detection.
- **Security levels** — `SL` (general/board access, 0-255) and `DSL`
  (download/file access) as independent tiers; each level carries per-call and
  per-day time limits plus message/email/post/file quotas and an abilities mask.
- **Access-right flags** — `AR` (per-board) and `DAR` (per-directory) bitfields;
  a resource lists required bits, access granted if the user holds any. Enables
  group access (Donors, Mods) without per-user lists.
- **Restrictions** — granular toggle bits: no-logon, no-chat, no-post, no-email,
  no-validate, no-automessage, no-anon, no-vote, no-multinode-chat, no-net,
  no-upload. Surgical blocks that don't touch SL/AR/DAR.
- **Exemptions** — override system policy per user: exempt-ratio, exempt-time,
  exempt-post, exempt-all, exempt-auto-delete.
- **Abilities (per SL)** — anonymity posting/reading, limited cosysop (one
  board), full cosysop, network validation.
- **Status/preference flags** — ANSI on/off, color, sounds, pause-on-page,
  expert mode, full-screen reader, conference mode, no-chat, auto-quote,
  24-hour clock, clear-on-logon, etc.
- **Statistics** — logons, on-today, messages posted/read, email/feedback sent,
  posts/email today, uploads/downloads (counts + KB), UL/DL ratio, chains run,
  gfiles read, last-BPS.
- **Time management** — lifetime time-on, time-on-today, sysop extra-time grant,
  time bank (deposit/earn minutes); per-logon and per-day enforcement
  (soft warn / hard disconnect).
- **New-user flow** — profile collection (infoform), ANSI detection, default
  SL/DSL + restrictions, optional auto-validation, feedback-to-sysop.
- **Validation** — auto-validation (ASV) preset profiles (e.g. "Casual",
  "Moderator"); manual sysop approval via user editor; validated-SL floor.
- **Soft delete / inactivity** — deleted + inactive flags (data retained),
  restore, inactivity auto-purge (>30 days, configurable).
- **Mailbox state** — local / network / internet forwarding, mailbox closure,
  email-waiting counter.
- **Per-user customization** — color slots (ANSI + B/W), quick-reply macros,
  menu-set selection, hotkeys, tagline / .PLAN profile text.
- **Read pointers** — newscan timestamp + per-area last-read markers (qscan).
- **Connection tracking** — last IP/address, last BPS; soft IP/CLID ban
  cross-referencing.
- **Guest mode** — no-disk-write session for public/anonymous access.

### 1.2 Messaging

- **Message areas (subboards)** — multiple topic areas, each with name, storage
  file, read/post ACS (SL + AR), anonymity mode, max-message cap, optional
  network binding.
- **Message storage** — header (from/to user+system, date, title, status,
  anonymity, location) + body; random-access layout with allocation table.
- **Storage abstraction** — pluggable message API (open/create, read/write,
  delete, lock, find-user-messages) so backends can vary.
- **Read pointers (qscan)** — per-user last-read per area + scan bitset; resume
  where left off; updated on logoff.
- **New-message scan (NSCAN)** — scan current area, scan-forward-from-here,
  scan-all-conferences; title-only scan; auto-scan at logon.
- **Private email** — separate store and access control (sender/recipient only);
  reply/forward/delete; anonymous option; accept-from-unvalidated check.
- **Threading & quoting** — parent-message reference; `>`-prefixed quoted text;
  auto-quote; reply chains.
- **Anonymity modes** — none / allowed / forced / dear-abby / real-names-only.
- **Message status flags** — deleted, locked, unvalidated, pending-network,
  source-verified.
- **Moderation** — validate / remove / lock posts; limited-cosysop scope.
- **Overflow / expiration** — max-message auto-prune (delete-one/all/none);
  age-based purge with optional archive.
- **Editors** — full-screen editor (word-wrap, abort/save) and/or line editor;
  user editor preference.
- **Feedback** — dedicated user-to-sysop channel, optionally anonymous.
- **Automessage** — system-wide one-line announcement (read/write/lock/delete).
- **Networking — WWIVnet** — typed binary packets (to/from system, main/minor
  type), unicast/multicast/broadcast, store-and-forward routing.
- **Networking — FidoNet/BinkP** — protocol state machine, plain + CRAM-MD5
  auth, CRC file transfer, FTN-address mapping.
- **Sub subscriptions** — add/drop requests, host approve/deny, subs.lst
  distribution.
- **Network ops & visibility** — pending queue, net log, contact tracking
  (last contact, failed calls, bytes), callout scheduling (windows, K-threshold).
- **QWK offline mail** — export `MESSAGES.DAT` + `.NDX` + `CONTROL.DAT`; import
  `.REP` replies; per-user limits, archive/protocol/color options.
- **Conferences** — group subboards/dirs into topic conferences; up/down/jump;
  enable/disable per user.
- **Trashcan** — bad-word filter (auto-delete or moderation queue).
- **Sub-board request** — user proposes new area; sysop approval queue.

### 1.3 Files & Transfer

- **File directories** — multiple areas, each with DSL/DAR or ACS access,
  max-file cap, type masks, FTN area tags.
- **File record** — aligned 8.3 name, description, upload + actual date,
  uploader, size, download count, owner user/system, property bitmask.
- **Extended descriptions** — separate `.ext` store; auto-extract `FILE_ID.DIZ`
  from uploaded archives, sanitized.
- **Directory header** — cached file count + newest-file date for instant
  listings.
- **File masks** — PD/shareware, no-upload, archive, no-ratio, CD-ROM, offline,
  uploadall, guest-visible, extended-desc-present.
- **Listing & search** — browse, filemask search, full-text description search,
  sort, tag/batch-mark, configurable columns (List+).
- **New-file scan** — list files uploaded since user's last scan, across dirs.
- **Upload processing** — validation, `.dir` record creation, DIZ extraction,
  dup checking, new-upload staging directory, allow-list whitelist.
- **File validation queue** — quarantine new uploads for sysop review
  (virus/dup/content) before public.
- **Ratios & points** — UL/DL byte tracking, system minimum ratio, ratio-exempt
  flag, no-ratio files; file-point economy (earn on upload, spend on perks).
- **Batch transfer** — queue multiple UL/DL, time estimation, resume.
- **Transfer protocols** — internal Zmodem/Xmodem/Xmodem-CRC/Ymodem/Ymodem-G
  state machines; external protocol framework (DSZ/sz-rz) via command templates
  + DSZ-log parsing for stats.
- **Archive support** — detect ARC/ZIP/LZH/RAR (magic/extension), list contents,
  view-archive, per-dir format rules.
- **Sysop file mgmt** — rename/move/delete/moderate; offline + CD-ROM flags to
  retire files without losing history.
- **Concurrency** — area load/save/lock/unlock, atomic updates, dirty flag.
- **Attachments** — files attached to email/posts, per-type quota.

### 1.4 Doors / External Programs

- **Doors/chains** — configurable external `.COM/.EXE` programs (games, utils),
  launched and returned to the BBS.
- **FOSSIL / socket bridge** — redirect remote I/O to the door over a FOSSIL
  driver (DOS) or socket.
- **Drop-file exchange** — write `DORINFO1.DEF` / `DOOR.SYS` / `CHAIN.TXT` with
  user info (name, SL, time-left, baud, node) pre-launch; re-read user stats on
  return.
- **Access control** — per-door ACS; "free" (no-permission) execution variant
  for demos.
- **Run by name/number** — invoke doors directly from menus.

### 1.5 Sysop & Multinode

- **WFC screen** — waiting-for-caller dashboard: system stats, current node
  user, other instances + locations, sysop availability, mail/feedback waiting,
  periodic refresh, local hotkeys.
- **Multinode** — logical nodes sharing user DB + message files; per-instance
  scratch dirs via format strings (`temp_%n`, `batch_%n`).
- **Instance metadata** — per-node record: user number, online/invisible/
  msg-available flags, location code (50+ states), sublocation, baud, timestamps.
- **Inter-instance messaging** — per-node message queue files; broadcast;
  semaphores (e.g. reload-user signal).
- **Who's online** — current users per node with location/activity.
- **Inter-node chat & page** — node-to-node instant messages; multi-user chat
  room; page-sysop alert with WFC popup; sysop drop-to-chat; read-only chat
  observation; toggle-available / invisible.
- **Sysop online functions** — quick-validate (adjust SL/DSL/AR/DAR/restrict),
  become-user, send instance message, reload user, prstatus snapshot.
- **Config program** — interactive editor for all settings (identity, paths,
  SL definitions, new-user profile, ASV profiles, message/file areas, editors,
  archivers, networks, menus); legacy migration; runs independently of the BBS.
- **Persistent config** — central config file (system identity, paths, limits,
  SL hierarchy, new-user defaults, toggles: closed-system, alias support, etc.).
- **Status file** — real-time global stats DB (callers, calls today, users,
  posts, email, uploads, active minutes, change flags, qscan pointer).
- **Logging** — global sysop log + per-instance log + audit of admin actions
  (validations, deletions, edits); per-call user stats.
- **Event scheduler** — timed events (logoff hook, daily maintenance, network
  calls, chain runs) while users online.
- **Modem/carrier handling** — init strings (AT commands), connect-speed detect,
  DCD carrier-loss auto-hangup. *(Telnet-native Vendetta/X substitutes
  socket/connection handling here.)*
- **Caller ID / ANI** — log/accept/reject by phone number. *(N/A telnet;
  IP-based equivalent.)*
- **Anti-flood / bot hardening** — login-attempt throttling, rate limits,
  bad-actor detection.

### 1.6 Display & UI

- **ANSI rendering** — ESC-sequence generator + parser (cursor move, SGR,
  save/restore, clear screen/line); 16-color IBM/CP437 palette.
- **Color codes** — pipe codes (`|XX` two-char fg/bg, `|#N` WWIV color),
  legacy heart codes (`\x03`+digit); bad-code recovery (literal passthrough).
- **MCI / data codes** — embedded data tokens (`|UH` handle, `|BN` board name,
  `|TL` time-left, file name/size/desc) and `@CODE@` macros with format
  modifiers (pad, byte-format, strftime); empty-string fallback on miss.
- **Theme indirection** — theme slots (`|T1-|T9`) for board-wide recolor via
  config.
- **Attributes** — bold, blink, reverse-video.
- **Cursor & screen control** — goxy, save/restore position, relative move,
  cls/clreol/clear-line.
- **Virtual screen model** — track cursor + attribute; testable backends;
  delta color compression to save bandwidth.
- **Unified output** — single stream to local console + remote, same colors.
- **Capability detection** — ANSI on/off per user, graceful plain-text
  degradation; CP437 vs UTF-8 encoding awareness for modern clients.
- **Artwork** — `.ans/.asc/.xbin` rendering, SAUCE metadata, render modes
  (instant/line/typewriter), auto-center.
- **Templates** — data-driven `.ans` templates with positioned input tokens and
  list header/line/footer triplets; sysop owns UI data, no recompile.
- **Menu rendering** — generated short (multi-column) vs long (with help) forms;
  custom `.ans` override; per-element colors.
- **Lightbar menus** — arrow-navigable highlighted menus (beyond yes/no widget);
  optional mouse (SGR) support.
- **Frames/dialogs** — bordered pop-up windows (single/double/ASCII, shadow,
  title), region save/restore, confirm/choice dialogs.
- **String table** — every user-visible string externalized; compiled defaults
  + override file + graceful fallback.
- **Text utilities** — ANSI-aware word-wrap with margins + alignment.
- **RIP** — vector/mouse GUI protocol (late-era, niche; low priority).

### 1.7 Misc / Community

- **Menu system** — data-driven menus: hotkey -> command (verb) + ACS;
  menusets, nested/stack navigation, numeric-keypad context (jump to sub/dir),
  per-menu logging, enter/exit actions, bulk-scan verbs. Separation of menu
  structure from command semantics (130+ verbs).
- **Oneliner wall** — short public messages.
- **Last-caller log** — recent callers: name, on/off time, duration, activity.
- **User list** — browse users (filter by SL/activity/location).
- **Bulletins / G-files / news** — logon bulletin (mandatory/optional),
  categorized browseable text library (rules, FAQ, tutorials, art).
- **Voting booths** — sysop polls (yes/no or multi-choice), one vote/user,
  tallies, expiry, archive.
- **Birthday / anniversary** — capture birthday, upcoming-birthday display,
  logon greeting.
- **BBS list** — user-contributed registry of other systems; network-synced
  list.
- **Settings / preferences UI** — in-session toggles (ANSI, expert, colors,
  pause, clock).
- **Performance** — caching, minimal redraws, optimized I/O (small-footprint
  matters on 486/640K).

---

## 2. Vendetta/X Gap Analysis

Status: **Have** / **Partial** / **Missing**.

### Users & Security

| Feature | Status | Note |
|---|---|---|
| User record (handle, location, tagline, group/affil, acs, flags, times_called, first/last call, posts) | Partial | Handle-only, no password ("no nup"); missing real name, contact, gender, birthday, computer type, stats depth |
| Names index | Partial | USER.DAT lookup exists; no formal O(log n) index / phone dedup |
| SL / DSL security levels | Missing | Single `acs` field only; no two-tier levels with quotas |
| AR / DAR access flags | Missing | No bitfield board/dir access |
| Restriction flags (11) | Missing | No granular per-feature blocks |
| Exemptions | Missing | — |
| Abilities (cosysop/anon per SL) | Missing | — |
| Status/preference flags | Partial | Some flags exist; no full preference set / expert mode |
| Statistics (logons, posts, UL/DL, ratio, time) | Partial | Has times_called, posts; missing UL/DL, ratio, time-on |
| Time management / time bank | Missing | No per-call/per-day limits, no bank |
| New-user flow (infoform) | Have | APPLICATION infoform with positioned input, bounded boxes, required-field validation |
| ANSI detection | Partial | Renderer degrades; no explicit detect/store step |
| Validation (ASV + manual) | Missing | No validation flow or feedback |
| Soft delete / inactivity purge | Missing | — |
| Mailbox forwarding/closure | Missing | No private email yet |
| Per-user colors / macros / menu-set | Missing | Theme slots are board-wide, not per-user |
| Read pointers (qscan) | Missing | — |
| IP / connection tracking | Partial | Telnet backend connects; not persisted per user |
| Guest mode | Missing | — |

### Messaging

| Feature | Status | Note |
|---|---|---|
| Message areas (subboards) | Have | Per-area `<TAG>.MSG`, MSGAREA command |
| Message storage (header + body) | Have | from/to/subject/body, post + read |
| Message API abstraction | Partial | Direct file access; no pluggable layer |
| Read pointers / NSCAN | Missing | No last-read tracking or new-scan |
| Private email | Missing | — |
| Threading & quoting | Missing | No reply/parent ref or auto-quote |
| Anonymity modes | Missing | — |
| Message status flags / moderation | Missing | No delete/lock/validate |
| Overflow / expiration | Missing | No max-message prune or age purge |
| Full-screen editor | Have | Sysop-custom ANSI header, word-wrap, ^Z/^X; no arrow nav yet |
| Line editor | Missing | — |
| Feedback channel | Missing | — |
| Automessage | Missing | — |
| WWIVnet networking | Missing | — |
| FidoNet/BinkP | Missing | — |
| QWK offline mail | Missing | — |
| Conferences | Missing | — |
| Trashcan / bad-word filter | Missing | — |
| Sub-board request | Missing | — |

### Files & Transfer

| Feature | Status | Note |
|---|---|---|
| File directories | Missing | No file areas at all |
| File record / metadata | Missing | — |
| Extended desc / DIZ extraction | Missing | — |
| File masks / categorization | Missing | — |
| Listing & search (List+) | Missing | — |
| New-file scan | Missing | — |
| Upload processing / validation queue | Missing | — |
| Ratios & points | Missing | — |
| Batch transfer | Missing | — |
| Transfer protocols (Z/X/Ymodem) | Missing | — |
| External protocol framework | Missing | — |
| Archive support | Missing | — |
| Sysop file mgmt | Missing | — |
| Attachments | Missing | — |

### Doors / External

| Feature | Status | Note |
|---|---|---|
| Doors / chains | Missing | — |
| FOSSIL / socket bridge | Missing | — |
| Drop-file exchange | Missing | — |
| Door ACS / run-by-name | Missing | — |

### Sysop & Multinode

| Feature | Status | Note |
|---|---|---|
| WFC screen | Missing | — |
| Multinode + per-instance dirs | Missing | Single-session today |
| Instance metadata | Missing | — |
| Inter-instance messaging | Missing | — |
| Who's online | Missing | — |
| Inter-node chat / page sysop / sysop chat | Missing | — |
| Sysop online functions (quick-validate, become-user) | Missing | — |
| Config program (VXCFG.EXE) | Missing | Config is compiled-in / data files edited by hand |
| Persistent config file | Partial | Data-driven menus + VENDX.STR exist; no central config.dat |
| Status file (real-time stats) | Partial | Last-callers log persisted; no global STATUS.DAT |
| Logging / audit | Partial | Last-callers only; no sysop action audit |
| Event scheduler | Missing | — |
| Connection handling (telnet) | Have | mTCP serve loop, output-buffered, ppio.h io interface |
| Anti-flood / bot hardening | Missing | — |

### Display & UI

| Feature | Status | Note |
|---|---|---|
| ANSI render + parser | Have | template/.ans renderer |
| Pipe codes (`|XX`) | Have | Mystic-style, two-char data tokens |
| Heart codes | Missing | Optional legacy compat only |
| MCI / data tokens | Have | Two-char data tokens, width/justify modifiers |
| `@CODE@` macros w/ modifiers | Partial | Have token modifiers; not full `@`-code set |
| Theme slots | Have | `|T` theme slots (board-wide) |
| Attributes (bold/blink/reverse) | Partial | Via ANSI; not all surfaced |
| Cursor/screen control | Have | Used by infoform positioning + editor |
| Virtual screen model / delta compression | Partial | Output-buffered; no formal VScreen/delta |
| Unified local+remote output | Have | One io interface (ppio.h) behind console + telnet |
| Capability detect / encoding | Partial | Degrades for non-ANSI; CP437 native; UTF-8 path unclear |
| Artwork + SAUCE | Partial | `.ans` rendered; no SAUCE parse / auto-center |
| Data-driven templates (header/line/footer) | Have | List triplets, width/justify modifiers |
| Menu rendering (short/long) | Partial | Data-driven menus exist; no generated short/long forms |
| Lightbar menus | Partial | Yes/no LIGHTBAR widget only; no full lightbar menus |
| Frames / dialogs | Partial | Bounded boxes in infoform; no general frame system |
| String table (defaults + override + fallback) | Have | VENDX.STR compiled-in defaults + fallback |
| Word-wrap (ANSI-aware) | Have | In editor |
| RIP | Missing | Niche, low priority |

### Misc / Community

| Feature | Status | Note |
|---|---|---|
| Data-driven menu system | Have | data/*.MNU hotkey -> command + ACS |
| Command verbs | Partial | GOTO/DISPLAY/LOGOFF/LASTCALL/USERLIST/ONELINER/MSGAREA; far from 130+ |
| Menusets / nested nav | Missing | Single menu layer |
| Oneliner wall | Have | ONELINER command + wall |
| Last-caller log | Have | Persisted |
| User list | Have | USERLIST command |
| Bulletins / G-files / news | Missing | — |
| Voting | Missing | — |
| Birthday / anniversary | Missing | — |
| BBS list | Missing | — |
| Settings / preferences UI | Missing | — |

---

## 3. Prioritized Roadmap (next ~12-18 features)

Vendetta/X is targeting **Phase 4 (commercial-grade)** and **Phase 5 (research)**.
Order below is the recommended build sequence; size is rough effort
(S = days, M = a week-ish, L = multi-week).

### Phase 4 — Commercial-grade (build a BBS a sysop can actually run)

The theme: a real account/security model, files, doors, and the sysop tooling
to operate a board. These are table-stakes for "mature BBS."

1. **Persistent config file + VXCFG.EXE (config program)** — *L* —
   Everything below needs central, sysop-editable settings (identity, paths,
   SL definitions, new-user defaults, area lists). Build the config.dat-style
   format and a curses/ANSI editor. Foundational; pull early.
   *(PULL EARLY — gates security levels, areas, validation.)*

2. **SL/DSL + AR/DAR + restrictions + ACS evaluation** — *L* —
   The security spine. Promote the single `acs` field into real
   levels + access bitfields + restriction flags, and add an ACS expression
   evaluator that menus/areas/doors all consult. Unlocks fine-grained access
   everywhere. *(PULL EARLY — every later feature gates on it.)*

3. **Validation flow (ASV profiles + manual sysop approval + feedback)** — *M* —
   New users currently walk straight in. Add default-restricted new users,
   auto-validation presets, a feedback-to-sysop message, and sysop approval.
   Depends on #1/#2.

4. **Time management (per-call + per-day limits, time bank, extra-time)** — *M* —
   Core fairness/abuse control tied to SL quotas. Soft warn + hard disconnect.

5. **Read pointers (qscan) + new-message scan + threading/quoting** — *M* —
   Makes the existing message bases usable at scale: resume-where-left-off,
   "what's new," reply chains, `>`-quoting. High user-visible value, builds on
   what's already there.

6. **Private email** — *M* — Separate store + sender/recipient access + reply/
   forward/delete + email-waiting indicator. Pairs naturally with #5's editor
   and read-pointer work.

7. **File areas + file records + listing/search + new-file scan** — *L* —
   The other half of a BBS. Multi-directory areas with DSL/DAR access, metadata
   (owner, size, dl-count), browse/search, new-file scan. (Transfer protocols
   split out below.)

8. **Transfer protocols (Zmodem + Xmodem/Ymodem) + batch + ratios** — *L* —
   Internal Z/X/Ymodem state machines (or external-protocol framework first),
   batch queue with time estimate, UL/DL byte tracking + ratio enforcement.
   Depends on #7. Largest single chunk; consider external-protocol framework as
   an MVP before internal Zmodem.

9. **Doors (FOSSIL/socket bridge + drop-file exchange)** — *L* —
   `DORINFO1.DEF`/`DOOR.SYS` generation, remote-I/O redirect, return-and-update.
   Unlocks the entire third-party door-game ecosystem; major draw.

10. **Lightbar menus + generated short/long menu forms + more command verbs** —
    *M* — Upgrade the data-driven menu layer: full arrow-nav lightbar menus
    (you only have the yes/no widget), generated menu rendering, and expand the
    verb set toward parity. Improves UX across everything above.

11. **Bulletins / G-files / news + logon bulletin** — *S* —
    Cheap, high-value: categorized text library + mandatory/optional logon
    bulletin. Mostly reuses the renderer.

12. **Sysop tooling: WFC screen + who's-online + page-sysop/sysop-chat +
    online user-edit** — *M* — A real waiting-for-caller dashboard, live
    user/quick-validate, and paging. Needs the status-file plumbing; even
    single-node it's valuable.

13. **Sysop log / audit + status file (real-time stats)** — *S* —
    Global STATUS.DAT-style stats + an audit trail of admin actions. Underpins
    the WFC screen (#12) and accountability.

### Phase 5 — Research (multinode, networking, offline, niche)

14. **Multinode (shared user DB, per-instance dirs, instance metadata,
    inter-instance messaging, inter-node chat)** — *L* —
    The big architectural leap from single-session to a real multi-node board.
    Format-string scratch dirs, INSTANCE.DAT-style metadata, message queues.

15. **QWK offline mail (export packet + import .REP)** — *M* —
    Offline reader support; high nostalgia/value, self-contained once messaging
    (#5/#6) is mature.

16. **WWIVnet / FidoNet networking (packets, routing, BinkP, callout
    scheduling, sub subscriptions)** — *L* —
    Federated messaging. The deepest networking work; a from-scratch
    implementation of the packet formats and BinkP. Research-grade.

17. **Event scheduler + inactivity purge + anti-flood/bot hardening** — *M* —
    Timed events (maintenance, net calls), dormant-account purge, login-attempt
    throttling. Operational maturity; some (anti-flood) worth pulling earlier if
    the telnet board gets exposed publicly.
    *(Consider pulling anti-flood earlier if Vendetta/X goes public before Phase 5.)*

18. **Community extras: voting booths, birthday/anniversary, BBS list,
    per-user preferences UI, conferences** — *S-M each* —
    Engagement/polish features. Conferences in particular become worthwhile once
    area counts (messages #5, files #7) grow large.

**Pull-early callouts:** #1 (config + VXCFG) and #2 (SL/DSL/AR/DAR/ACS) are
prerequisites for almost everything in Phase 4 — schedule them first. Anti-flood
hardening (part of #17) should be pulled forward from Phase 5 if the telnet
server is exposed to the public internet before then.

---

## 4. On compatibility

This study is **concepts-only**: it documents *what* mature BBS subsystems do and
*why*, distilled from the behavior and feature sets of the boards Vendetta/X grew
up on. Vendetta/X implements these subsystems from scratch with its own data
model (a single SQLite spine). Where it names concrete artifacts — `DORINFO1.DEF`
and `DOOR.SYS` drop files, QWK `MESSAGES.DAT` / `.REP` packets, `FILE_ID.DIZ`,
pipe codes — those are well-known community formats, matched only at the
interoperability boundary so the board can talk to existing doors and offline
readers. Everything else is Vendetta/X's own design.
