package lord

// This file holds the player-vs-player corner of Red Dragon: the "Warriors of
// the Realm" leaderboard and the duel where you hunt down another stored
// warrior. Both are reached from the town menu in game.go.

// aliveMark returns a short coloured (alive)/(dead) badge for a warrior.
func aliveMark(alive bool) string {
	if alive {
		return "\x1b[1;32malive\x1b[0m"
	}
	return "\x1b[1;31mdead\x1b[0m"
}

// scores is "Warriors of the Realm": the leaderboard of other stored warriors,
// strongest first.
func (g *Game) scores() {
	g.cls("Warriors of the Realm")

	others, err := g.st.Others(g.ch.Handle, 15)
	if err != nil || len(others) == 0 {
		g.p("\x1b[0;37m  The realm has no other warriors yet. You stand alone.\x1b[0m\r\n")
		g.pause()
		return
	}

	g.p("\x1b[1;30m  ##  Warrior              Lvl  Experience      Dragons  \x1b[0m\r\n")
	g.p("\x1b[1;30m  " + rule(54) + "\x1b[0m\r\n")
	for i, o := range others {
		g.pf("  \x1b[1;36m%2d\x1b[0;37m  \x1b[1;37m%-18s\x1b[1;36m%4d\x1b[1;35m%14s\x1b[1;31m%8d\x1b[1;30m   %s\x1b[0m\r\n",
			i+1, trunc(o.Name, 18), o.Level, commas(o.Exp), o.DragonKills, aliveMark(o.Alive))
	}
	g.nl()
	g.pause()
}

// attackPlayer hunts down another warrior for a duel.
func (g *Game) attackPlayer() {
	g.cls("Slay Another Warrior")
	g.status()
	g.nl()

	c := g.ch
	if c.PlayerFights <= 0 {
		g.p("\x1b[1;33m  You have no player fights left today.\x1b[0m\r\n")
		g.pause()
		return
	}
	if !c.Alive {
		g.p("\x1b[1;31m  You are dead. Come back with the new day.\x1b[0m\r\n")
		g.pause()
		return
	}

	others, err := g.st.Others(c.Handle, 12)
	if err != nil || len(others) == 0 {
		g.p("\x1b[0;37m  There are no other warriors to challenge yet.\x1b[0m\r\n")
		g.pause()
		return
	}

	g.p("\x1b[0;37m  Whom do you hunt?\x1b[0m\r\n\r\n")
	for i, o := range others {
		g.pf("  \x1b[1;33m%2d\x1b[0;37m) \x1b[1;37m%-18s\x1b[1;36m Lvl %-3d  \x1b[1;30m%s\x1b[0m\r\n",
			i+1, trunc(o.Name, 18), o.Level, aliveMark(o.Alive))
	}

	choice := g.askInt("\r\n\x1b[0;37m  Number to attack (\x1b[1;33mQ\x1b[0;37m to leave) \x1b[1;36m> \x1b[1;37m")
	if choice < 1 || choice > len(others) {
		return
	}
	target := others[choice-1]

	// Re-load the chosen warrior fresh -- their state may have changed since the
	// list was built, and you cannot strike the dead.
	foe, found, err := g.st.Load(target.Handle)
	if err != nil || !found || foe == nil {
		return
	}
	if !foe.Alive {
		g.nl()
		g.pf("\x1b[1;31m  %s already lies dead. Choose a living foe.\x1b[0m\r\n", foe.Name)
		g.pause()
		return
	}

	// The fight begins -- the turn is spent whatever the outcome.
	c.PlayerFights--
	g.save()

	for {
		g.cls("Slay Another Warrior")
		g.pf("\x1b[1;30m  You: \x1b[1;32mHP %d/%d\x1b[1;30m   vs   \x1b[1;37m%s: \x1b[1;31mHP %d/%d\x1b[0m\r\n\r\n",
			c.HP, c.MaxHP, foe.Name, foe.HP, foe.MaxHP)

		switch g.prompt("\x1b[1;33m  A\x1b[0;37m)ttack  \x1b[1;33mR\x1b[0;37m)un   \x1b[1;36m> \x1b[1;37m") {
		case 'a':
			dmg := Damage(c.EffAttack(), foe.EffDefense(), g.rng)
			foe.HP -= dmg
			g.pf("\r\n\x1b[1;32m  You strike %s for %d damage!\x1b[0m\r\n", foe.Name, dmg)
			if foe.HP <= 0 {
				g.pvpWin(foe)
				return
			}
			back := Damage(foe.EffAttack(), c.EffDefense(), g.rng)
			c.HP -= back
			g.pf("\x1b[1;31m  %s hits you for %d!\x1b[0m\r\n", foe.Name, back)
			if c.HP <= 0 {
				g.pvpLose(foe)
				return
			}
			g.pause()
		case 'r':
			g.p("\r\n\x1b[1;36m  You break off the hunt and slip away.\x1b[0m\r\n")
			g.save()
			g.pause()
			return
		case 0:
			g.save()
			return
		}
	}
}

// pvpWin resolves a victory: loot the foe's on-hand gold, lay them low, and
// persist both warriors.
func (g *Game) pvpWin(foe *Character) {
	c := g.ch
	loot := foe.Gold
	c.Gold += loot
	c.Kills++

	foe.Gold = 0
	foe.Alive = false
	foe.HP = 0
	foe.Deaths++
	_ = g.st.Save(foe)

	g.save()

	g.pf("\r\n\x1b[1;31m  You have slain %s!\x1b[0m\r\n", foe.Name)
	if loot > 0 {
		g.pf("\x1b[1;33m  You loot \x1b[1;36m%s\x1b[1;33m gold from the body (their bank is untouchable).\x1b[0m\r\n", commas(loot))
	} else {
		g.p("\x1b[0;37m  They carried no gold on hand to steal.\x1b[0m\r\n")
	}
	g.pause()
}

// pvpLose resolves a defeat: your on-hand gold is looted and you fall. The foe
// is not modified.
func (g *Game) pvpLose(foe *Character) {
	c := g.ch
	lost := c.Gold
	c.Gold = 0
	c.Alive = false
	c.HP = 0
	c.Deaths++
	g.save()

	g.pf("\r\n\x1b[1;31m  %s has slain you!\x1b[0m\r\n", foe.Name)
	if lost > 0 {
		g.pf("\x1b[1;33m  They looted \x1b[1;36m%s\x1b[1;33m gold from your corpse (your bank is safe).\x1b[0m\r\n", commas(lost))
	}
	g.p("\x1b[0;37m  You will rise again with the new day.\x1b[0m\r\n")
	g.pause()
}

// trunc clamps a name to max runes so the leaderboard columns stay aligned.
func trunc(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	return string(r[:max])
}
