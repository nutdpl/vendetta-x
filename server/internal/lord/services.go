package lord

// This file holds two town services: the Healer's Hut (restores hit points for
// gold, scaled by level) and Ye Olde Bank (move gold between hand and the safe
// vault). Both are reached from the town menu in game.go.

// healer is the Healer's Hut: pay gold to restore hit points.
func (g *Game) healer() {
	g.cls("Healer's Hut")
	g.status()
	g.nl()

	c := g.ch
	if !c.Alive {
		g.p("\x1b[1;31m  The healer shakes her head sadly.\x1b[0m\r\n")
		g.p("\x1b[0;37m  \"I cannot raise the dead, warrior. Rest until the new day.\"\x1b[0m\r\n")
		g.pause()
		return
	}

	per := HealCost(c.Level)
	g.pf("\x1b[0;37m  Your wounds: \x1b[1;32m%d\x1b[0;37m / \x1b[1;32m%d\x1b[0;37m hit points.\x1b[0m\r\n", c.HP, c.MaxHP)
	g.pf("\x1b[0;37m  The healer charges \x1b[1;33m%s\x1b[0;37m gold per hit point.\x1b[0m\r\n\r\n", commas(per))

	if c.HP >= c.MaxHP {
		g.p("\x1b[1;36m  You are already in perfect health.\x1b[0m\r\n")
		g.pause()
		return
	}

	g.p("  \x1b[1;33mF\x1b[0;37m)ully heal              \x1b[1;33mS\x1b[0;37m)ome healing\r\n")
	g.p("  \x1b[1;33mQ\x1b[0;37m)uit\r\n")

	var points int
	switch g.prompt("\r\n\x1b[0;37m  Your choice \x1b[1;36m> \x1b[1;37m") {
	case 'f':
		// Heal as much of the missing HP as gold allows.
		points = c.MaxHP - c.HP
		if per > 0 {
			afford := c.Gold / per
			if afford < points {
				points = afford
			}
		}
	case 's':
		want := g.askInt("\r\n\x1b[0;37m  How many points \x1b[1;36m> \x1b[1;37m")
		points = want
		if missing := c.MaxHP - c.HP; points > missing {
			points = missing
		}
		if per > 0 {
			if afford := c.Gold / per; points > afford {
				points = afford
			}
		}
	default:
		return
	}

	if points <= 0 {
		g.nl()
		g.p("\x1b[1;31m  You haven't the gold for that.\x1b[0m\r\n")
		g.pause()
		return
	}

	cost := points * per
	c.Gold -= cost
	c.HP += points
	if c.HP > c.MaxHP {
		c.HP = c.MaxHP
	}
	g.save()

	g.nl()
	g.pf("\x1b[1;32m  The healer mends %d hit point(s) for \x1b[1;33m%s\x1b[1;32m gold.\x1b[0m\r\n", points, commas(cost))
	g.pf("\x1b[0;37m  You now have \x1b[1;32m%d\x1b[0;37m / \x1b[1;32m%d\x1b[0;37m HP and \x1b[1;33m%s\x1b[0;37m gold.\x1b[0m\r\n", c.HP, c.MaxHP, commas(c.Gold))
	g.pause()
}

// bank is Ye Olde Bank: deposit and withdraw gold. Banked gold is safe from
// death and player-vs-player theft.
func (g *Game) bank() {
	c := g.ch
	for {
		g.cls("Ye Olde Bank")
		g.status()
		g.nl()
		g.pf("\x1b[0;37m  Gold on hand: \x1b[1;33m%s\x1b[0m\r\n", commas(c.Gold))
		g.pf("\x1b[0;37m  Gold in bank: \x1b[1;33m%s\x1b[0m\r\n", commas(c.Bank))
		g.p("\x1b[1;30m  Banked gold is safe from death and from other warriors.\x1b[0m\r\n\r\n")

		g.p("  \x1b[1;33mD\x1b[0;37m)eposit                 \x1b[1;33mW\x1b[0;37m)ithdraw\r\n")
		g.p("  \x1b[1;33mQ\x1b[0;37m)uit\r\n")

		switch g.prompt("\r\n\x1b[0;37m  Your choice \x1b[1;36m> \x1b[1;37m") {
		case 'd':
			x := g.askInt("\r\n\x1b[0;37m  Deposit how much \x1b[1;36m> \x1b[1;37m")
			if x > c.Gold {
				x = c.Gold
			}
			if x <= 0 {
				continue
			}
			c.Gold -= x
			c.Bank += x
			g.save()
			g.pf("\r\n\x1b[1;32m  Deposited \x1b[1;33m%s\x1b[1;32m gold.\x1b[0m\r\n", commas(x))
			g.pf("\x1b[0;37m  Hand: \x1b[1;33m%s\x1b[0;37m   Bank: \x1b[1;33m%s\x1b[0m\r\n", commas(c.Gold), commas(c.Bank))
			g.pause()
		case 'w':
			x := g.askInt("\r\n\x1b[0;37m  Withdraw how much \x1b[1;36m> \x1b[1;37m")
			if x > c.Bank {
				x = c.Bank
			}
			if x <= 0 {
				continue
			}
			c.Bank -= x
			c.Gold += x
			g.save()
			g.pf("\r\n\x1b[1;32m  Withdrew \x1b[1;33m%s\x1b[1;32m gold.\x1b[0m\r\n", commas(x))
			g.pf("\x1b[0;37m  Hand: \x1b[1;33m%s\x1b[0;37m   Bank: \x1b[1;33m%s\x1b[0m\r\n", commas(c.Gold), commas(c.Bank))
			g.pause()
		case 'q', 0:
			return
		}
	}
}
