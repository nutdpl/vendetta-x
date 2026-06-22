# Vendetta/X — legacy C/DOS board

This directory archives the **original C implementation** of Vendetta/X: a
strict-C89 BBS that builds native (`vendx`/`vendx-net`), as a 16-bit real-mode
DOS binary, and as a 32-bit protected-mode DOS binary with a real TCP/IP stack
(Watt-32). Per [`../DESIGN.md`](../DESIGN.md) it is a **co-equal test bench** —
the "real 486 / bare-metal DOS" proving ground — not dead code.

The board that ships today is the Go server in [`../server/`](../server)
(telnet + ssh + web over one SQLite spine). This tree is kept for history and as
the DOS/telnet reference; it is built and tested separately.

## Building

```sh
cd legacy
make          # native host engine (vendx) + the sockets telnet board (vendx-net)
make test     # C unit tests (telnet codec, userbase, acs, ...)
./vendx auto  # scripted walkthrough
```

The DOS cross-compiles (Open Watcom) and the Watt-32 telnet link are wired up in
[`../.github/workflows/build.yml`](../.github/workflows/build.yml). See
[`tools/README.md`](tools/README.md) for the art/render tooling.

## Layout

- `core/` — the engine (render, msgbase, userbase, acs, telnet codec, doors, …)
- `io/` — platform I/O backends (`io_local`, `io_sock`, `io_watt`, `io_mtcp`)
- `src/` — entry points (`main.c`, `vxcfg.c`)
- `tests/` — C unit tests
- `data/` — DOS menu/string data (`*.MNU`, `VENDX.STR`)
- `tools/` — art generators, render-to-PNG, Watt-32 build helper
- `Makefile`, `mk*.bat`, `*.lnk` — host + DOS build rules
- `*.sh` — DOSBox-X run/proof harnesses
