# tools/ — host-side dev tooling (not part of the DOS board)

These run on the dev box, never on the 486. They exist so the board can be
worked on without a DOS machine in the loop.

## ANSI -> PNG feedback loop

See a screen exactly as a caller would, without booting anything. The board's
real renderer (`core/render.c`) produces the bytes; we just paint them.

- **`render_dump.c`** — renders a screen file (`.pp` template or `.ans` art)
  through the actual board renderer and writes the raw ANSI+CP437 byte stream
  to stdout. One implementation of the pipe codes (the board's), so the picture
  can't drift from reality. Optional second arg highlights a lightbar option,
  the way `lbmenu_run` would.

- **`ansi2png.py`** — turns that byte stream (or any `.ans`) into a PNG, using
  the VGA 16-color palette in the ANSI order the board emits. Dependency:
  Pillow (`pip install Pillow`).

```sh
# build the dumper (host cc, c89)
cc -std=c89 -Icore -Iio tools/render_dump.c core/render.c -o render_dump

# lightbar menu, bar sitting on option #1
./render_dump art/lbdemo.pp 1 | python3 tools/ansi2png.py - shots/lbdemo.png

# a finished .ans, 1:1 pixels
python3 tools/ansi2png.py art/example.ans shots/example.png --scale 2

# or just:  make shots
```

### Font / CP437 fidelity

By default `ansi2png` draws with the bundled IBM VGA 8x16 ROM bitmap
(`tools/VGA8.F16`, 4 KB) for pixel-exact CP437 -- every block/shade/box glyph
correct, rendered at native res and NEAREST-scaled so pixels stay crisp. Pass
`--font some.ttf` to use a TrueType face instead (a clean mono fallback is
auto-picked if the ROM file is absent).

```sh
python3 tools/ansi2png.py art/foo.ans out.png            # authentic VGA (default)
python3 tools/ansi2png.py art/foo.ans out.png --font /path/mono.ttf
```

`VGA8.F16` is from VileR's [vga-text-mode-fonts](https://github.com/viler-int10h/vga-text-mode-fonts)
(plain ROM bitmap, freely includable).

## TheDraw font headers

- **`render_tdf.py`** — render text in TheDraw color fonts (`.TDF`) to an ANSI
  sampler. Point it at a directory of `.TDF` fonts
  and it spells your text in every font that can, so you can pick one for a
  screen header. Fonts aren't vendored here; grab a collection (e.g.
  [tdfiglet](https://github.com/tat3r/tdfiglet)'s `fonts/`).

```sh
# spell VENDETTA in every font, then eyeball them all as one PNG
python3 tools/render_tdf.py "VENDETTA" /path/to/fonts shots/tdf
python3 tools/ansi2png.py shots/tdf-utf8.ans shots/tdf.png --utf8
```

`render_tdf` writes a UTF-8 preview (`*-utf8.ans`) plus a width index
(`*-index.txt`); feed the preview to `ansi2png` with `--utf8`. Once you pick a
font, render just it and paste the ANSI into the screen's `.ans`/`.pp`.

- **`curate_tdf.py`** — score every font for "organic / hand-drawn" ACiD
  character (shade chars, half blocks, height variation, frequent colour
  shifts) and render the top N as contact sheets, so you can hunt a vibe across
  a thousand fonts fast. Tall shaded fonts render best at a wider `--cols`
  (the bundled VGA ROM font already draws their shade glyphs correctly).

```sh
python3 tools/curate_tdf.py "VENDETTA" /path/to/fonts sheets/ 12
python3 tools/ansi2png.py sheets/sheet01-utf8.ans sheets/sheet01.png --utf8
```

- **`logo_compose.py`** — compose individual TDF glyphs with per-letter x/y
  offsets so letters overlap/stagger like a hand-drawn scene logo (and to spell
  text a font lacks a glyph for). `spec` is `LETTER:dx:dy,...` (dx relative to
  the previous letter's right edge, negative = overlap; dy absolute row).

- **`mklbdemo.py`** — build step that compiles a `logo_compose` logo + option
  markers into `art/lbdemo.pp` (the lightbar demo screen). Shows the
  spec-compiles-to-screen pattern; re-run after editing the layout.

- **`tpl2ans.py`** — convert a `\e`-placeholder `.tpl` (readable UTF-8 source)
  into a UTF-8 preview and a real CP437 `.ans` for the board.

## Other

- **`build-watt32.sh`** — fetch + build the Watt-32 TCP stack for the telnet build.
- **`mkmake.py`** — regenerate DOS build lists.
