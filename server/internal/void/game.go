package void

import (
	"log"
	"math/rand"
	"strconv"
	"strings"

	"vendetta-x/server/internal/term"
)

// Game runs one play session for one trader over a terminal.
type Game struct {
	s    *term.Session
	st   *Store
	tr   *Trader
	rng  *rand.Rand
	sec  *Sector // current sector (refreshed each turn)
	port *Port   // current sector's port, or nil
}

// NewGame wires a session, store, and the player's (loaded or fresh) trader.
func NewGame(s *term.Session, st *Store, tr *Trader, rng *rand.Rand) *Game {
	return &Game{s: s, st: st, tr: tr, rng: rng}
}

// ---- shared UI helpers (used by every subsystem file) ----------------------

// cls clears the screen and prints the Voidfarer banner with a section title.
func (g *Game) cls(title string) {
	g.s.Print("\x1b[0m\x1b[2J\x1b[H")
	g.s.Print("\x1b[1;36m  V O I D F A R E R \x1b[1;30m\xc4\xc4 \x1b[1;33m" + title + "\x1b[0m\r\n")
	g.s.Print("\x1b[1;30m  " + rule(60) + "\x1b[0m\r\n\r\n")
}

func (g *Game) p(s string)                 { g.s.Print(s) }
func (g *Game) pf(format string, a ...any) { g.s.Printf(format, a...) }
func (g *Game) nl()                        { g.s.Print("\r\n") }
func (g *Game) flush()                     { g.s.Flush() }
func (g *Game) line(max int) string        { return g.s.ReadLine(max) }
func (g *Game) pause()                     { g.s.Pause() }

// key reads one keypress, lowercased; 0 on disconnect.
func (g *Game) key() byte {
	k, ch := g.s.ReadKey()
	switch k {
	case term.KeyEnter:
		return '\r'
	case term.KeyEsc:
		return 27
	case term.KeyEOF:
		return 0
	case term.KeyChar:
		if ch >= 'A' && ch <= 'Z' {
			ch += 32
		}
		return ch
	}
	return 0
}

func (g *Game) prompt(s string) byte {
	g.s.Print(s)
	g.s.Flush()
	return g.key()
}

func (g *Game) yesno(q string) bool {
	return g.prompt(q+" \x1b[1;37m(\x1b[1;32mY\x1b[1;37m/\x1b[1;31mN\x1b[1;37m) \x1b[0m") == 'y'
}

// askInt prompts for a non-negative integer (blank/garbage -> 0).
func (g *Game) askInt(s string) int {
	g.s.Print(s)
	g.s.Flush()
	n, _ := strconv.Atoi(strings.TrimSpace(g.line(12)))
	if n < 0 {
		n = 0
	}
	return n
}

// save persists the trader; a failure is logged, never fatal to play.
func (g *Game) save() {
	if err := g.st.Save(g.tr); err != nil {
		log.Printf("void: save %s: %v", g.tr.Handle, err)
	}
}

// status prints the one-line ship vitals bar (used across subsystems).
func (g *Game) status() {
	t := g.tr
	g.pf("\x1b[1;30m  [\x1b[1;33mCr %s\x1b[1;30m] [\x1b[1;36mTurns %d\x1b[1;30m] [\x1b[0;37mHolds %d/%d\x1b[1;30m] [\x1b[1;32mFig %s\x1b[1;30m] [\x1b[1;34mShd %s\x1b[1;30m]\x1b[0m\r\n",
		commas(t.Credits), t.Turns, t.HoldsUsed(), t.HoldsMax, commas(t.Fighters), commas(t.Shields))
}

// cargoLine prints the current cargo manifest.
func (g *Game) cargoLine() {
	t := g.tr
	g.pf("\x1b[1;30m  Cargo: \x1b[0;37m%s %d  \x1b[0;37m%s %d  \x1b[0;37m%s %d  \x1b[1;30m(%d holds free)\x1b[0m\r\n",
		CommodityShort[Ore], t.Cargo[Ore], CommodityShort[Org], t.Cargo[Org],
		CommodityShort[Equ], t.Cargo[Equ], t.HoldsFree())
}

// ---- the main command loop -------------------------------------------------

// Run applies the daily reset, shows the intro, then loops the sector display
// and command prompt until the captain leaves.
func (g *Game) Run() {
	if g.tr.ResetIfNewDay() {
		g.save()
	}
	g.intro()
	for {
		g.sec, _ = g.st.Sector(g.tr.Sector)
		g.port, _ = g.st.PortAt(g.tr.Sector)
		if g.sec == nil { // safety: lost in space -> back to Sol
			g.tr.Sector = StartSector
			g.save()
			continue
		}
		g.displaySector()

		g.s.Print("\r\n\x1b[0;37m  Command \x1b[1;30m(a sector # to warp, or letter)\x1b[0;37m \x1b[1;36m> \x1b[1;37m")
		g.s.Flush()
		cmd := strings.TrimSpace(g.line(6))
		if cmd == "" {
			continue
		}
		if n, err := strconv.Atoi(cmd); err == nil {
			g.moveTo(n)
			continue
		}
		switch lc(cmd[0]) {
		case 'p':
			if g.port != nil && g.port.Class >= 1 && g.port.Class <= 8 {
				g.portTrade()
			} else {
				g.note("There's no trading port to dock with here.")
			}
		case 'd', 's':
			if g.port.IsStarDock() {
				g.starDock()
			} else {
				g.note("StarDock isn't in this sector.")
			}
		case 'c':
			g.computer()
		case 'r':
			g.rankings()
		case 't', 'a':
			g.attackTrader()
		case 'f':
			g.deployFighters()
		case 'v':
			g.viewShip()
		case 'q':
			g.save()
			g.cls("Disembark")
			g.pf("\x1b[0;37m  Safe travels, Captain \x1b[1;37m%s\x1b[0;37m.\x1b[0m\r\n", g.tr.Name)
			g.pause()
			return
		default:
			g.note("Unknown command.")
		}
	}
}

func (g *Game) intro() {
	g.cls("Voidfarer")
	g.pf("\x1b[0;37m  Welcome aboard, Captain \x1b[1;33m%s\x1b[0;37m.\x1b[0m\r\n\r\n", g.tr.Name)
	g.p("\x1b[0;37m  The galaxy is open for business. Buy low, sell high, build a fleet,\r\n")
	g.p("  and carve your name into the stars. Type a sector number to warp;\r\n")
	g.p("  letters run the ship's systems. \x1b[1;30m(C)omputer lists every command.\x1b[0m\r\n")
	g.pause()
}

// displaySector renders the current sector: warps, port, fighters, and any
// other traders parked here.
func (g *Game) displaySector() {
	g.cls("Sector " + strconv.Itoa(g.sec.ID))
	g.status()
	g.nl()
	fed := ""
	if g.sec.ID <= FedSpaceMax {
		fed = "  \x1b[1;34m(Federation space)\x1b[0m"
	}
	g.pf("  \x1b[1;36mSector \x1b[1;37m%d \x1b[1;30m- \x1b[0;37m%s\x1b[0m%s\r\n", g.sec.ID, g.sec.Name, fed)

	if g.port != nil {
		switch {
		case g.port.IsStarDock():
			g.p("  \x1b[1;35mPort  : StarDock \x1b[1;30m-- ship upgrades & outfitting  (D to dock)\x1b[0m\r\n")
		case g.port.IsTerra():
			g.p("  \x1b[1;32mPort  : Terra \x1b[1;30m-- the homeworld\x1b[0m\r\n")
		default:
			g.pf("  \x1b[1;33mPort  : Class %d \x1b[1;37m(%s) \x1b[1;30m-- trading  (P to dock)\x1b[0m\r\n",
				g.port.Class, ClassCode(g.port.Class))
		}
	}

	// Warps.
	var w []string
	for _, d := range g.sec.Warps {
		w = append(w, strconv.Itoa(d))
	}
	g.pf("  \x1b[1;36mWarps : \x1b[1;37m%s\x1b[0m\r\n", strings.Join(w, "  "))

	// Deployed fighters here.
	if figs, _ := g.st.FightersAt(g.sec.ID); len(figs) > 0 {
		for _, f := range figs {
			tag := "\x1b[1;31m(hostile)"
			if strings.EqualFold(f.Owner, g.tr.Handle) {
				tag = "\x1b[1;32m(yours)"
			}
			g.pf("  \x1b[1;30mFighters: \x1b[1;37m%s \x1b[0;37mx%d %s\x1b[0m\r\n", f.Owner, f.Qty, tag)
		}
	}

	// Other traders parked here.
	if others, _ := g.st.TradersInSector(g.sec.ID, g.tr.Handle); len(others) > 0 {
		for _, o := range others {
			g.pf("  \x1b[1;31mTrader: \x1b[1;37m%s \x1b[1;30m(fighters %s)\x1b[0m\r\n", o.Name, commas(o.Fighters))
		}
	}
}

// moveTo warps the ship to dest if a warp exists and a turn remains.
func (g *Game) moveTo(dest int) {
	if dest == g.tr.Sector {
		return
	}
	ok, _ := g.st.WarpExists(g.tr.Sector, dest)
	if !ok {
		g.note("No warp lane from here to sector " + strconv.Itoa(dest) + ".")
		return
	}
	if g.tr.Turns <= 0 {
		g.note("Out of turns for today. Return tomorrow, Captain.")
		return
	}
	g.tr.Turns--
	g.tr.Sector = dest
	g.save()
}

// viewShip is the full ship readout.
func (g *Game) viewShip() {
	t := g.tr
	g.cls("Ship's Manifest")
	row := func(label, val string) {
		g.pf("  \x1b[1;36m%-14s\x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", label, val)
	}
	row("Captain", t.Name)
	row("Sector", strconv.Itoa(t.Sector))
	row("Credits", commas(t.Credits))
	row("Turns left", strconv.Itoa(t.Turns))
	row("Cargo holds", strconv.Itoa(t.HoldsUsed())+" / "+strconv.Itoa(t.HoldsMax)+" used")
	row("Fuel Ore", strconv.Itoa(t.Cargo[Ore]))
	row("Organics", strconv.Itoa(t.Cargo[Org]))
	row("Equipment", strconv.Itoa(t.Cargo[Equ]))
	row("Fighters", commas(t.Fighters))
	row("Shields", commas(t.Shields))
	row("Net worth", commas(t.NetWorth()))
	row("Kills", strconv.Itoa(t.Kills))
	g.pause()
}

// note shows a one-line message and waits for a key.
func (g *Game) note(msg string) {
	g.pf("\r\n\x1b[1;33m  %s\x1b[0m\r\n", msg)
	g.pause()
}

// ---- small helpers ---------------------------------------------------------

func rule(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 0xC4
	}
	return string(b)
}

func lc(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

// commas formats an integer with thousands separators.
func commas(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) > 3 {
		var out []byte
		for i, c := range []byte(s) {
			if i > 0 && (len(s)-i)%3 == 0 {
				out = append(out, ',')
			}
			out = append(out, c)
		}
		s = string(out)
	}
	if neg {
		return "-" + s
	}
	return s
}
