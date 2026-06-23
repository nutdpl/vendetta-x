# Running doors on Vendetta/X

A *door* is an external program a caller drops into from the board — a classic
DOS game, a util, anything. Vendetta/X runs doors by:

1. writing a standard **drop file** (`DOOR.SYS` or `DORINFO1.DEF`) into the
   door's working directory, describing the caller (handle, security level, time
   left, ANSI on/off, node), and
2. **exec'ing the configured command** and bridging the caller's terminal to the
   door process over a real **pseudo-terminal** (so the door sees a true tty:
   raw keys, `isatty()` true, the controlling terminal an emulator expects).

You configure doors in the web sysop panel at **`/sysop/doors`**:

| Field      | Meaning                                                                 |
| ---------- | ----------------------------------------------------------------------- |
| Command    | the program to run, as plain `argv` split on spaces (no shell, no quotes — one path, simple flags) |
| Work dir   | the directory the drop file is written into and the process runs in     |
| Drop type  | `DOOR.SYS` (Synchronet-style, ~52 lines) or `DORINFO1.DEF` (Fido-style) |
| DOS path   | DOOR.SYS fields 33/34, the door's DOS-side path (e.g. `C:\GAME`); blank if unsure |

> **The Command is split on whitespace, not by a shell.** `dosemu2 -dumb -K
> /opt/doors/game -E START.BAT` is fine; `sh -c '... | ...'` is not. Wrap anything that
> needs a shell, pipes, or quoting in a small launcher script and point Command
> at the script.

## The key fact about DOS doors

A DOS door is a 16-bit `.EXE`; Linux can't exec it directly. You need an
**emulator that exposes the door's console or serial to our bridge**. The choice
of emulator matters:

- **dosemu2** can run a DOS program *in your terminal* and render its text
  screen as ANSI to stdin/stdout. With our PTY bridge that lands straight on the
  caller. **This is the recommended path.**
- **DOSBox / DOSBox-X** is primarily a *GUI* emulator: pointing Command at
  `dosbox …` does **not** bridge the guest program's I/O to us. To use DOSBox-X
  you must redirect the door's COM port to a socket and bridge that (see below) —
  it is not plug-and-play.

## Recommended: dosemu2

[dosemu2](https://github.com/dosemu2/dosemu2) is the modern, maintained DOS
emulator and the path most Linux boards use for DOS doors.

1. Install it (`apt install dosemu2`, or build from source).
2. Put the door's files in a directory, e.g. `/opt/doors/game`.
3. Configure a door with, for example:

   ```
   Command:   dosemu2 -dumb -K /opt/doors/game -E "START.BAT"
   Work dir:  /opt/doors/game
   Drop type: DOOR.SYS
   DOS path:  C:\GAME
   ```

   - `-dumb` runs dosemu2 as a terminal application (no window), rendering the
     DOS screen as ANSI over the tty our bridge provides.
   - `-K <dir>` lazily mounts `<dir>` as the DOS drive; `-E <cmd>` runs the door.
   - Run the door in its **local / sysop mode** so it reads the keyboard and
     writes the screen (the drop file supplies the caller's identity). Doors
     that insist on FOSSIL/COM need the serial path below.

Because the door is handed a real controlling terminal, raw keystrokes and ANSI
flow through cleanly. Test with one door first; tune the `START.BAT` /
`autoexec` per door.

## Native Linux doors

Open-source and ported doors that read stdin / write stdout (or want a tty) need
no emulator at all — point Command straight at the binary:

```
Command:   /opt/doors/mybbsgame --dropfile DOOR.SYS
Work dir:  /opt/doors/mygame
Drop type: DOOR.SYS
```

Vendetta/X also ships two built-in native games (Guess the Vault, Dice Duel)
that need no configuration.

## FOSSIL/serial-only doors (DOSBox-X over a socket)

Some doors only talk to a modem via a FOSSIL driver and never use the local
console. Those need the door's **COM port bridged over a socket** rather than
stdin/stdout. The shape:

1. Launch DOSBox-X headless (`SDL_VIDEODRIVER=dummy`) with a config whose
   `[serial]` maps `serial1` to a `nullmodem` TCP endpoint, and an `autoexec`
   that loads a FOSSIL driver (e.g. `BNU`/`X00`) and runs the door on COM1.
2. Have the board connect to that TCP endpoint and bridge caller ⟷ socket.

Vendetta/X's current bridge is terminal/PTY-based (great for dosemu2 and native
doors); a socket launcher for the DOSBox-X serial path is the planned next step.
If you need FOSSIL-only doors today, prefer dosemu2, which can also attach a DOS
COM port to a pty.

## Safety notes

- Door commands run with the **server's** privileges. Run Vendetta/X as an
  unprivileged user, and ideally sandbox doors (a container, a dedicated user, or
  a chroot). Treat "configure a door" as a full-trust sysop action.
- A missing binary or empty command fails cleanly (`ErrUnavailable` /
  `ErrNotConfigured`) — the caller gets a notice and returns to the menu; the
  board never hangs.
