package lord

import (
	"log"
	"math/rand"
	"strconv"

	"vendetta-x/server/internal/term"
)

// Game runs one play session for one warrior over a terminal.
type Game struct {
	s   *term.Session
	st  *Store
	ch  *Character
	rng *rand.Rand
}

// NewGame wires a session, store, and the player's (already loaded or freshly
// created) character together.
func NewGame(s *term.Session, st *Store, ch *Character, rng *rand.Rand) *Game {
	return &Game{s: s, st: st, ch: ch, rng: rng}
}

// ---- shared UI helpers (used by every town service file) -------------------

// cls clears the screen and prints the Red Dragon banner with a section title.
func (g *Game) cls(title string) {
	g.s.Print("\x1b[0m\x1b[2J\x1b[H")
	g.s.Print("\x1b[1;31m  R E D   D R A G O N \x1b[1;30m\xc4\xc4 \x1b[1;33m" + title + "\x1b[0m\r\n")
	g.s.Print("\x1b[1;30m  " + rule(60) + "\x1b[0m\r\n\r\n")
}

func (g *Game) p(s string)                 { g.s.Print(s) }
func (g *Game) pf(format string, a ...any) { g.s.Printf(format, a...) }
func (g *Game) nl()                        { g.s.Print("\r\n") }
func (g *Game) flush()                     { g.s.Flush() }
func (g *Game) line(max int) string        { return g.s.ReadLine(max) }
func (g *Game) pause()                     { g.s.Pause() }

// key reads one keypress, lowercased; returns 0 on disconnect.
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

// prompt prints a prompt and waits for a single key.
func (g *Game) prompt(s string) byte {
	g.s.Print(s)
	g.s.Flush()
	return g.key()
}

// yesno asks a yes/no question (default no).
func (g *Game) yesno(q string) bool {
	return g.prompt(q+" \x1b[1;37m(\x1b[1;32mY\x1b[1;37m/\x1b[1;31mN\x1b[1;37m) \x1b[0m") == 'y'
}

// askInt prompts for a non-negative integer (blank/garbage -> 0).
func (g *Game) askInt(s string) int {
	g.s.Print(s)
	g.s.Flush()
	n, _ := strconv.Atoi(g.line(12))
	if n < 0 {
		n = 0
	}
	return n
}

// save persists the warrior; a failure is logged, never fatal to play.
func (g *Game) save() {
	if err := g.st.Save(g.ch); err != nil {
		log.Printf("lord: save %s: %v", g.ch.Handle, err)
	}
}

// status prints the one-line vitals bar.
func (g *Game) status() {
	c := g.ch
	g.pf("\x1b[1;30m  [\x1b[1;32mHP %d/%d\x1b[1;30m] [\x1b[1;33mGold %s\x1b[1;30m] [\x1b[1;36mLvl %d\x1b[1;30m] [\x1b[1;35mExp %s\x1b[1;30m] [\x1b[0;37mForest %d\x1b[1;30m]\x1b[0m\r\n",
		c.HP, c.MaxHP, commas(c.Gold), c.Level, commas(c.Exp), c.ForestFights)
}

// ---- the town loop ---------------------------------------------------------

// Run applies the daily reset, shows the intro, then loops the town menu until
// the warrior leaves. It is the entry point the door calls.
func (g *Game) Run() {
	if g.ch.ResetIfNewDay() {
		g.save()
	}
	g.intro()
	for {
		g.cls("Town Square")
		g.status()
		g.nl()
		g.p("  \x1b[1;33mF\x1b[0;37m)ight in the Forest      \x1b[1;33mW\x1b[0;37m)eapon Shop\r\n")
		g.p("  \x1b[1;33mH\x1b[0;37m)ealer's Hut             \x1b[1;33mA\x1b[0;37m)rmor Shop\r\n")
		g.p("  \x1b[1;33mB\x1b[0;37m)ank                     \x1b[1;33mI\x1b[0;37m)nn\r\n")
		g.p("  \x1b[1;33mL\x1b[0;37m)ist Warriors            \x1b[1;33mP\x1b[0;37m)layer Attack\r\n")
		g.p("  \x1b[1;33mV\x1b[0;37m)iew your stats          \x1b[1;33mQ\x1b[0;37m)uit to the BBS\r\n")
		if CanChallengeMaster(g.ch) && g.ch.Alive {
			g.pf("\r\n  \x1b[1;31mC\x1b[1;33m)hallenge %s -- you feel ready to advance!\x1b[0m\r\n",
				Masters[g.ch.Level].Name)
		}
		switch g.prompt("\r\n\x1b[0;37m  Your command \x1b[1;36m> \x1b[1;37m") {
		case 'f':
			g.forest()
		case 'w':
			g.weaponShop()
		case 'a':
			g.armorShop()
		case 'h':
			g.healer()
		case 'b':
			g.bank()
		case 'i':
			g.inn()
		case 'l':
			g.scores()
		case 'p':
			g.attackPlayer()
		case 'v':
			g.viewStats()
		case 'c':
			if CanChallengeMaster(g.ch) && g.ch.Alive {
				g.challengeMaster()
			}
		case 'q', 0:
			g.save()
			g.cls("Farewell")
			g.pf("\x1b[0;37m  The town gates close behind you, \x1b[1;37m%s\x1b[0;37m. Until next time.\x1b[0m\r\n", g.ch.Name)
			g.pause()
			return
		}
	}
}

func (g *Game) intro() {
	g.cls("Legend of the Red Dragon")
	g.pf("\x1b[0;37m  Welcome back, \x1b[1;33m%s\x1b[0;37m.\x1b[0m\r\n\r\n", g.ch.Name)
	if !g.ch.Alive {
		g.p("\x1b[1;31m  You are dead. Rest until the new day to fight again.\x1b[0m\r\n\r\n")
	}
	g.p("\x1b[0;37m  The forest waits, dark and hungry. The Red Dragon stirs beyond it.\r\n")
	g.p("  Grow strong, warrior -- only a champion may end its reign.\x1b[0m\r\n")
	g.pause()
}

// ---- the forest + combat (core) --------------------------------------------

func (g *Game) forest() {
	c := g.ch
	if !c.Alive {
		g.cls("The Forest")
		g.p("\x1b[1;31m  You are dead. Come back with the new day.\x1b[0m\r\n")
		g.pause()
		return
	}
	if c.ForestFights <= 0 {
		g.cls("The Forest")
		g.p("\x1b[1;33m  You are too tired to fight any more today.\x1b[0m\r\n")
		g.pause()
		return
	}
	c.ForestFights--
	m := RollMonster(c.Level, g.rng)

	for {
		g.cls("The Forest")
		g.status()
		g.nl()
		g.pf("  \x1b[0;37mYou are face to face with \x1b[1;31m%s\x1b[0;37m.\x1b[0m\r\n", m.Name)
		g.pf("  \x1b[1;30mIts health: \x1b[1;31m%d\x1b[0m\r\n\r\n", m.HP)
		switch g.prompt("\x1b[1;33m  A\x1b[0;37m)ttack  \x1b[1;33mS\x1b[0;37m)tats  \x1b[1;33mR\x1b[0;37m)un   \x1b[1;36m> \x1b[1;37m") {
		case 'a':
			dmg := Damage(c.EffAttack(), 0, g.rng)
			m.HP -= dmg
			g.pf("\r\n\x1b[1;32m  You strike for %d damage!\x1b[0m\r\n", dmg)
			if m.HP <= 0 {
				g.win(m)
				return
			}
			if g.monsterHits(&m) {
				return // died
			}
			g.pause()
		case 's':
			g.viewStats()
		case 'r':
			if g.rng.Intn(100) < 40 {
				g.p("\r\n\x1b[1;36m  You slip away into the trees.\x1b[0m\r\n")
				g.save()
				g.pause()
				return
			}
			g.p("\r\n\x1b[1;31m  You can't escape!\x1b[0m\r\n")
			if g.monsterHits(&m) {
				return
			}
			g.pause()
		case 0:
			return
		}
	}
}

// monsterHits applies the foe's counterattack; returns true if the player died.
func (g *Game) monsterHits(m *Monster) bool {
	dmg := Damage(m.Attack, g.ch.EffDefense(), g.rng)
	g.ch.HP -= dmg
	g.pf("\x1b[1;31m  %s hits you for %d!\x1b[0m\r\n", m.Name, dmg)
	if g.ch.HP <= 0 {
		g.die("the forest")
		return true
	}
	return false
}

func (g *Game) win(m Monster) {
	c := g.ch
	c.Exp += m.Exp
	c.Gold += m.Gold
	c.Kills++
	g.pf("\r\n\x1b[1;32m  %s\x1b[0m\r\n", m.DeadMsg)
	g.pf("\x1b[1;33m  You gain %s experience and %s gold.\x1b[0m\r\n", commas(m.Exp), commas(m.Gold))
	if CanChallengeMaster(c) {
		g.pf("\r\n\x1b[1;31m  You feel ready to challenge %s (use C in town).\x1b[0m\r\n", Masters[c.Level].Name)
	}
	g.save()
	g.pause()
}

func (g *Game) die(where string) {
	c := g.ch
	c.Alive = false
	c.Deaths++
	lost := c.Gold
	c.Gold = 0
	c.HP = 0
	g.pf("\r\n\x1b[1;31m  You have been slain in %s!\x1b[0m\r\n", where)
	if lost > 0 {
		g.pf("\x1b[1;33m  You dropped %s gold (your bank savings are safe).\x1b[0m\r\n", commas(lost))
	}
	g.p("\x1b[0;37m  You will rise again with the new day.\x1b[0m\r\n")
	g.save()
	g.pause()
}

// ---- master fights, leveling, the dragon -----------------------------------

func (g *Game) challengeMaster() {
	c := g.ch
	mst := Masters[c.Level]
	g.cls("Master's Challenge")
	g.pf("\x1b[0;37m  %s, master of level %d, rises to meet you.\x1b[0m\r\n", mst.Name, c.Level)
	g.pf("\x1b[1;30m  \"%s, you say? We shall see.\"\x1b[0m\r\n\r\n", c.Name)
	if !g.yesno("  Do you challenge the master?") {
		return
	}
	foe := Monster{Name: mst.Name, HP: mst.HP, Attack: mst.Attack}
	for {
		g.cls("Master's Challenge")
		g.status()
		g.pf("\r\n  \x1b[1;35m%s\x1b[0;37m's health: \x1b[1;31m%d\x1b[0m\r\n\r\n", mst.Name, foe.HP)
		switch g.prompt("\x1b[1;33m  A\x1b[0;37m)ttack  \x1b[1;33mR\x1b[0;37m)etreat   \x1b[1;36m> \x1b[1;37m") {
		case 'a':
			dmg := Damage(c.EffAttack(), mst.Attack/4, g.rng)
			foe.HP -= dmg
			g.pf("\r\n\x1b[1;32m  You strike %s for %d!\x1b[0m\r\n", mst.Name, dmg)
			if foe.HP <= 0 {
				g.levelUp(mst)
				return
			}
			if g.monsterHits(&foe) {
				return
			}
			g.pause()
		case 'r', 0:
			g.p("\r\n\x1b[1;33m  You bow out -- the master smirks.\x1b[0m\r\n")
			g.pause()
			return
		}
	}
}

func (g *Game) levelUp(mst Master) {
	c := g.ch
	c.MaxHP += mst.GainHP
	c.Attack += mst.GainAttack
	c.Defense += mst.GainDefense
	c.HP = c.MaxHP
	g.pf("\r\n\x1b[1;32m  You have defeated %s!\x1b[0m\r\n", mst.Name)
	if c.Level >= MaxLevel {
		g.save()
		g.pause()
		g.dragon()
		return
	}
	c.Level++
	g.pf("\x1b[1;33m  You advance to level %d! (+%d max HP, +%d attack, +%d defense)\x1b[0m\r\n",
		c.Level, mst.GainHP, mst.GainAttack, mst.GainDefense)
	g.save()
	g.pause()
}

func (g *Game) dragon() {
	c := g.ch
	g.cls("The Red Dragon")
	g.p("\x1b[1;31m  Beyond the master lies the lair of the Red Dragon itself.\x1b[0m\r\n")
	if !g.yesno("  Do you enter, and face the beast?") {
		return
	}
	foe := Monster{Name: "the Red Dragon", HP: 120000, Attack: 3000}
	for {
		g.cls("The Red Dragon")
		g.status()
		g.pf("\r\n  \x1b[1;31mThe Red Dragon\x1b[0;37m's health: \x1b[1;31m%s\x1b[0m\r\n\r\n", commas(foe.HP))
		switch g.prompt("\x1b[1;33m  A\x1b[0;37m)ttack  \x1b[1;33mR\x1b[0;37m)un   \x1b[1;36m> \x1b[1;37m") {
		case 'a':
			dmg := Damage(c.EffAttack(), 400, g.rng)
			foe.HP -= dmg
			g.pf("\r\n\x1b[1;32m  You wound the dragon for %d!\x1b[0m\r\n", dmg)
			if foe.HP <= 0 {
				g.slayDragon()
				return
			}
			if g.monsterHits(&foe) {
				return
			}
			g.pause()
		case 'r', 0:
			g.p("\r\n\x1b[1;33m  You flee the lair, heart pounding.\x1b[0m\r\n")
			g.pause()
			return
		}
	}
}

func (g *Game) slayDragon() {
	c := g.ch
	c.DragonKills++
	c.Gold += 5000000
	g.cls("Champion of the Realm")
	g.pf("\x1b[1;33m  The Red Dragon falls! %s, you are a LEGEND.\x1b[0m\r\n", c.Name)
	g.pf("\x1b[0;37m  Dragon kills: \x1b[1;31m%d\x1b[0;37m. The bards will sing of this.\x1b[0m\r\n\r\n", c.DragonKills)
	g.p("\x1b[0;37m  Reborn anew, you set out to do it all again...\x1b[0m\r\n")
	// Reborn: fresh level-1 warrior, but the legend (dragon kills) endures.
	dk := c.DragonKills
	name := c.Name
	*c = *NewCharacter(c.Handle, name)
	c.DragonKills = dk
	c.LastPlayed = "" // a fresh day's allotment awaits
	g.save()
	g.pause()
}

// viewStats shows the full character sheet.
func (g *Game) viewStats() {
	c := g.ch
	g.cls("Your Stats")
	row := func(label, val string) {
		g.pf("  \x1b[1;36m%-14s\x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", label, val)
	}
	row("Name", c.Name)
	row("Level", strconv.Itoa(c.Level))
	row("Experience", commas(c.Exp))
	row("Hit Points", strconv.Itoa(c.HP)+" / "+strconv.Itoa(c.MaxHP))
	row("Attack", strconv.Itoa(c.EffAttack())+"  ("+Weapons[clampIdx(c.WeaponN, len(Weapons))].Name+")")
	row("Defense", strconv.Itoa(c.EffDefense())+"  ("+Armors[clampIdx(c.ArmorN, len(Armors))].Name+")")
	row("Gold on hand", commas(c.Gold))
	row("Gold in bank", commas(c.Bank))
	row("Forest fights", strconv.Itoa(c.ForestFights))
	row("Player fights", strconv.Itoa(c.PlayerFights))
	row("Charm", strconv.Itoa(c.Charm))
	row("Kills / Deaths", strconv.Itoa(c.Kills)+" / "+strconv.Itoa(c.Deaths))
	row("Dragon kills", strconv.Itoa(c.DragonKills))
	g.pause()
}

// ---- small formatting helpers ----------------------------------------------

func rule(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 0xC4
	}
	return string(b)
}

func clampIdx(n, length int) int {
	if n < 0 {
		return 0
	}
	if n >= length {
		return length - 1
	}
	return n
}

// commas formats an integer with thousands separators.
func commas(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
