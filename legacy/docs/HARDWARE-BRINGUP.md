# Vendetta/X — 486 hardware bring-up (Phase 2: "it answers")

The board is built; the one unproven thing is a real caller connecting over
telnet. This is the checklist to close that, on the real 486 (or DOSBox-X with
an emulated NE2000). **Single caller first** — multinode/hardening come later.

## The target stack (decided)

```
packet driver for the NIC      <- the real unknown; verify FIRST
DOS/4GW (or DOS/32A) extender   <- DOS/4GW for now (CI parity); try DOS/32A later
VX/32 engine                    <- 32-bit protected mode, flat model (mk32.bat)
Watt-32 telnet backend          <- io/io_watt.c, links against Watt-32
single caller                   <- prove one session end-to-end before scaling
```

Why 32-bit: flat memory, no 64k DGROUP wall (the 16-bit build is one `-zt256`
trick away from the ceiling), and Watt-32 already handles the ugly real-mode
packet-driver-callback / DPMI-transition / buffer-ownership problems so we
don't have to. `core/telnet.c` (IAC negotiation) is transport-independent —
the stack is an `io` backend swap, never a rewrite.

## 0. Know the card (do this before any backend work)

The packet driver and Watt-32 config depend entirely on the actual NIC. Fill in:

- [ ] **NIC make/model:** ____  (NE2000 clone? 3Com 3C509? Intel EtherExpress?)
- [ ] **Packet driver + software interrupt:** ____  (e.g. `NE2000.COM 0x60`)
- [ ] **I/O base / IRQ:** ____
- [ ] DHCP available on the LAN, or static IP? ____

NE2000 clones are the easy path (well-supported packet drivers, Watt-32 sees
them cleanly). If it's a 3Com/Intel, get its specific packet driver first.

## 1. Driver + connectivity smoke (before the board)

Prove the network works with known-good DOS tools, so a failure later is the
*board*, not the card:

- [ ] packet driver loads without error; note the vector it installs on
- [ ] `WATTCP.CFG` (or DHCP) gives the box an IP — confirm with a Watt-32
      sample tool (`tcpinfo`, `ping`)
- [ ] `ping <gateway>` succeeds
- [ ] `ping <another host>` succeeds (DNS/route sanity)

If any of these fail, stop — fix the driver/IP layer before touching VENDX.

## 2. The board answers (single caller)

- [ ] get a 32-bit telnet `VENDX.EXE`:
      - CI artifact `VENDX-dos32-telnet`, **or**
      - `runwatt.sh` (downloads the CI EXE + boots it in DOSBox-X), **or**
      - build locally: `tools/build-watt32.sh` then link `io/io_watt.c`
- [ ] board boots, binds, and **listens on port 23** (or 2323 if 23 is taken)
- [ ] connect from **SyncTERM** (CP437 + ANSI client) → the login **matrix**
      paints (Phiber2 logo, lightbar)
- [ ] arrow-key the lightbar; `L` → handle prompt; walk MAIN → Messages/Files
- [ ] log off with `G` — the line drops cleanly and the node frees

## 3. The robustness cases (where telnet boards actually break)

- [ ] **idle disconnect** — client sits idle past the inactivity timeout → node
      reclaimed (no stuck "ghost" node)
- [ ] **abrupt disconnect** — yank the connection mid-session → carrier loss
      detected, session torn down, node freed
- [ ] **repeated callers** — connect / use / drop, three times in a row →
      stores reopen cleanly each time, no warm-state weirdness
- [ ] **reboot / re-run** — quit VENDX and relaunch → listens again, prior
      session state doesn't bleed through

## 4. Notes for later (not blockers)

- **DOS/32A:** smaller + faster startup than DOS/4GW. Keep DOS/4GW for CI
  parity; try DOS/32A on the metal once the above is green.
- **Doors are the real boss fight,** not telnet: third-party doors expect a
  serial/FOSSIL port, the engine pauses while a door runs, and TCP still needs
  servicing. That needs a FOSSIL/socket bridge or a resident-servicing story
  (see DESIGN.md "Doors, FOSSIL, and multitasking"). Do **not** let doors block
  getting the board itself solid over telnet.

When section 2 is green, **Phase 2 is done** and everything else stops being
theoretical.
