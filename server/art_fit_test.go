package main

import (
	"bytes"
	"os"
	"regexp"
	"strconv"
	"testing"

	"vendetta-x/server/internal/menu"
	"vendetta-x/server/internal/render"
)

// TestArtFits80x24 renders every full-screen art file the board serves and
// replays the resulting ANSI through a tiny ANSI.SYS-style terminal at
// 80x24, failing if the screen ever scrolls or a cursor jump asks for a row
// past 24. 24 rows -- not 25 -- because SyncTERM's default 80x25 screen
// mode keeps line 25 for its own status bar, leaving the session 24; and
// ANSI.SYS-family terminals wrap the moment column 80 is painted. A screen
// that paints its bottom row and then emits one more newline scrolls the
// whole thing up a line, shearing already-painted lightbar labels one row
// off the absolute positions Lightbar then repaints at (every label shows
// doubled, one row apart -- exactly the bug this test pins down).
//
// loginscreen.ans is exempt: it's the cinematic splash, taller than any
// screen on purpose -- it scrolls as it reveals and has no lightbar.
func TestArtFits80x24(t *testing.T) {
	tokens := map[string]string{
		// Representative-length values for every data token the screens use;
		// a missing token would render literally and distort line widths.
		"BN": "Vendetta/X", "CN": "12", "TU": "1234", "TC": "56789",
		"TI": "23:59", "UH": "somelonghandle", "UL": "Some Location",
		"UC": "123", "MB": "General Discussion", "FA": "Uploads Area",
		"MG": "General Discussion", "MC": "12 of 345", "MF": "somelonghandle",
		"MT": "anotherhandle", "MS": "Re: a fairly long subject line here",
		"MD": "Tue 2026-07-07 03:59",
	}

	screens := map[string][]byte{}
	for _, name := range []string{
		"matrix.pp", "msgmenu.pp", "filemenu.pp", "goodbye.pp",
		"welcome.pp", "newuser.pp", "msgread.pp", "msgedit.pp",
	} {
		b, err := os.ReadFile("../art/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		screens[name] = b
	}
	// The main menu is composed at runtime: splice the shipped default
	// bindings into the placeholder exactly like mainMenuTemplate does.
	raw, err := os.ReadFile("../art/mainmenu.pp")
	if err != nil {
		t.Fatalf("read mainmenu.pp: %v", err)
	}
	items := map[string]menu.Item{}
	for _, d := range mainMenuDefaults {
		items[d.Slot] = d
	}
	optionsText, _ := mainMenuOptions(items)
	screens["mainmenu.pp (composed)"] = bytes.Replace(
		raw, []byte("@@MENU_OPTIONS@@"), []byte(optionsText), 1)

	for name, src := range screens {
		var out bytes.Buffer
		if err := render.Render(&out, src, &render.Ctx{Tokens: tokens}); err != nil {
			t.Fatalf("%s: render: %v", name, err)
		}
		scrolls, maxRow := replay80x24(out.Bytes())
		if scrolls > 0 {
			t.Errorf("%s scrolls an 80x24 terminal %d time(s); every screen "+
				"must fit within 24 rows (SyncTERM leaves 24 with its status line on)",
				name, scrolls)
		}
		if maxRow > 24 {
			t.Errorf("%s jumps the cursor to row %d; must stay within 24", name, maxRow)
		}
	}
}

var csiRe = regexp.MustCompile(`^\x1b\[([0-9;?]*)([A-Za-z])`)

// replay80x24 walks an ANSI byte stream over an 80x24 ANSI.SYS-style screen
// (IMMEDIATE wrap when column 80 is painted), returning how many times the
// screen scrolled and the highest row any cursor-positioning sequence asked
// for (requests past the height clamp on a real terminal, hiding the
// overflow -- so they're reported, not clamped silently).
func replay80x24(data []byte) (scrolls, maxRow int) {
	const w, h = 80, 24
	x, y := 1, 1
	down := func() {
		y++
		if y > h {
			y = h
			scrolls++
		}
	}
	for i := 0; i < len(data); {
		if data[i] == 0x1b {
			if m := csiRe.FindSubmatch(data[i:]); m != nil {
				var n []int
				for _, p := range bytes.Split(m[1], []byte(";")) {
					if v, err := strconv.Atoi(string(bytes.TrimPrefix(p, []byte("?")))); err == nil {
						n = append(n, v)
					}
				}
				arg := func(k, def int) int {
					if k < len(n) && n[k] > 0 {
						return n[k]
					}
					return def
				}
				switch m[2][0] {
				case 'H', 'f':
					r, c := arg(0, 1), arg(1, 1)
					if r > maxRow {
						maxRow = r
					}
					y, x = min(r, h), min(c, w)
				case 'G':
					x = min(arg(0, 1), w)
				case 'd':
					r := arg(0, 1)
					if r > maxRow {
						maxRow = r
					}
					y = min(r, h)
				case 'A':
					y = max(1, y-arg(0, 1))
				case 'B':
					y = min(h, y+arg(0, 1))
				case 'C':
					x = min(w, x+arg(0, 1))
				case 'D':
					x = max(1, x-arg(0, 1))
				case 'J':
					if len(n) > 0 && n[0] == 2 {
						x, y = 1, 1
					}
				}
				i += len(m[0])
				continue
			}
			i++
			continue
		}
		switch data[i] {
		case '\r':
			x = 1
		case '\n':
			down()
		case 0x00, 0x07:
		default:
			x++
			if x > w { // ANSI.SYS: wrap the moment column 80 is painted
				x = 1
				down()
			}
		}
		i++
	}
	if y > maxRow {
		maxRow = y
	}
	return scrolls, maxRow
}
