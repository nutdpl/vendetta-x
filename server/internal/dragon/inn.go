package dragon

// inn.go is the Red Dragon Inn -- the social heart of town. Seth Able tends the
// bar and trades rumors for gold, Violet flirts (or doesn't), and a warm bed
// upstairs patches you up for a price. Nothing here can be farmed for free:
// drinks and beds cost gold, and Violet's charm is random and one-shot per
// selection, so no keypress loops into unlimited charm.

// rumors are Seth Able's bar talk -- one is served at random with each drink.
var rumors = []string{
	"\"Heard the goblins out east drop fatter purses than the rats. Worth the bruises.\"",
	"\"They say the Red Dragon's hide turns aside steel -- you'll want the sharpest blade gold can buy.\"",
	"\"A wise warrior banks his gold before the forest. The dead carry nothing to the grave.\"",
	"\"Run from a fight if you must -- a live coward outlasts a brave corpse, har!\"",
	"\"The masters guard the road to each new level. Beat one and you'll feel the strength settle in your bones.\"",
	"\"Some nights I swear I hear the dragon stirring beyond the trees. Cold nights. Quiet ones.\"",
	"\"Violet over there has broken more hearts than the dragon has bones. Mind yours, friend.\"",
	"\"Forest's only good for so many fights a day. Push past your luck and you'll feed the worms.\"",
}

// violetWin / violetFail are the flavorful outcomes of a single flirt.
var violetWin = []string{
	"Violet laughs, low and warm, and lets her hand rest a moment on your arm.",
	"Violet leans close enough that you forget your own name, then winks and slips away.",
	"\"Well now,\" Violet purrs, \"aren't YOU full of surprises tonight.\"",
	"Violet tucks a flower behind your ear and calls you 'trouble' with a smile.",
}

var violetFail = []string{
	"Violet rolls her eyes. \"Buy a girl a drink first, hero.\"",
	"Violet pats your cheek -- a touch too firmly. \"Nice try, sweetheart.\"",
	"\"Mmm. No,\" says Violet, already turning to a tableful of knights.",
	"Violet flicks a bar rag at you and laughs. \"Come back when you're a legend.\"",
}

// inn runs the Red Dragon Inn menu loop.
func (g *Game) inn() {
	for {
		g.cls("The Red Dragon Inn")
		g.status()
		g.nl()
		g.p("\x1b[0;37m  Warmth and woodsmoke wrap around you. A fire snaps in the hearth,\r\n")
		g.p("  mugs clatter, and somewhere a lute is being played badly but happily.\x1b[0m\r\n\r\n")

		g.p("  \x1b[1;33mB\x1b[0;37m)uy a drink from Seth Able   \x1b[1;33mR\x1b[0;37m)ent a room for the night\r\n")
		g.p("  \x1b[1;35mV\x1b[0;37m)flirt with Violet            \x1b[1;33mQ\x1b[0;37m)uit to the town square\r\n")

		switch g.prompt("\r\n\x1b[0;37m  Your choice \x1b[1;36m> \x1b[1;37m") {
		case 'b':
			g.innDrink()
		case 'v':
			g.innFlirt()
		case 'r':
			g.innRoom()
		case 'q', 0:
			g.nl()
			g.p("\x1b[0;37m  Seth lifts a mug in farewell as you step back into the night.\x1b[0m\r\n")
			g.pause()
			return
		}
	}
}

// innDrink trades a small fee for a drink and a random rumor. Cheap, repeatable;
// the only thing it ever spends is gold.
func (g *Game) innDrink() {
	c := g.ch
	cost := c.Level * 10
	if cost < 10 {
		cost = 10
	}
	g.nl()
	g.pf("\x1b[0;37m  Seth Able wipes a mug and grins. \"What'll it be? A pint runs you \x1b[1;33m%s\x1b[0;37m gold.\"\x1b[0m\r\n",
		commas(cost))
	if c.Gold < cost {
		g.pf("\x1b[1;31m  You dig through empty pockets -- only %s gold. Seth shrugs.\x1b[0m\r\n", commas(c.Gold))
		g.pause()
		return
	}
	if !g.yesno("  Buy a round?") {
		return
	}
	c.Gold -= cost
	g.save()
	g.pf("\r\n\x1b[1;33m  You slide %s gold across the bar and drink deep.\x1b[0m\r\n\r\n", commas(cost))
	g.pf("\x1b[1;30m  Seth leans in: \x1b[0;37m%s\x1b[0m\r\n", rumors[g.rng.Intn(len(rumors))])
	g.pause()
}

// innFlirt is the classic. Exactly one attempt per selection; success is random
// (~1 in 3) and grants a small charm bump, so there is no way to loop it into
// unlimited charm.
func (g *Game) innFlirt() {
	c := g.ch
	g.nl()
	if c.Married {
		g.p("\x1b[1;35m  Violet catches the ring on your finger and laughs. \"Behave, you.\"\x1b[0m\r\n")
		g.pause()
		return
	}
	g.p("\x1b[1;35m  Violet is holding court by the fire, all dark eyes and trouble.\x1b[0m\r\n\r\n")

	if g.rng.Intn(3) == 0 {
		gain := 1 + g.rng.Intn(3) // 1..3
		c.Charm += gain
		g.save()
		g.pf("\x1b[1;35m  %s\x1b[0m\r\n", violetWin[g.rng.Intn(len(violetWin))])
		g.pf("\r\n\x1b[1;32m  Your charm rises by %d. (Charm: %d)\x1b[0m\r\n", gain, c.Charm)
	} else {
		g.pf("\x1b[1;35m  %s\x1b[0m\r\n", violetFail[g.rng.Intn(len(violetFail))])
	}
	g.pause()
}

// innRoom rents a bed for the night: heals ~30% of MaxHP for gold. A cozy, fixed
// alternative to the healer. Clamps to MaxHP and only charges if affordable.
func (g *Game) innRoom() {
	c := g.ch
	g.nl()
	if !c.Alive {
		g.p("\x1b[1;31m  No bed mends the dead. Rest until the new day, warrior.\x1b[0m\r\n")
		g.pause()
		return
	}
	cost := c.Level * 15
	if cost < 15 {
		cost = 15
	}
	heal := c.MaxHP * 30 / 100
	if heal < 1 {
		heal = 1
	}
	g.pf("\x1b[0;37m  A clean bed upstairs is \x1b[1;33m%s\x1b[0;37m gold, and you'll wake mended some.\x1b[0m\r\n",
		commas(cost))

	if c.HP >= c.MaxHP {
		g.p("\x1b[1;36m  But you're hale as an ox -- no sense paying to sleep off nothing.\x1b[0m\r\n")
		g.pause()
		return
	}
	if c.Gold < cost {
		g.pf("\x1b[1;31m  You can't cover it -- just %s gold to your name.\x1b[0m\r\n", commas(c.Gold))
		g.pause()
		return
	}
	if !g.yesno("  Take the room?") {
		return
	}
	c.Gold -= cost
	c.HP += heal
	if c.HP > c.MaxHP {
		c.HP = c.MaxHP
	}
	g.save()
	g.pf("\r\n\x1b[1;36m  You sleep like the dead and wake to hot bread and birdsong.\x1b[0m\r\n")
	g.pf("\x1b[1;32m  Restored to %d / %d hit points.\x1b[0m\r\n", c.HP, c.MaxHP)
	g.pause()
}
