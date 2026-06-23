package void

// combat.go: sector fighters + ship-to-ship combat -- the (F)ighter Bay for
// deploying/collecting fighters in a sector, and (A)ttack for engaging another
// trader parked alongside you. Federation space (sectors 1..FedSpaceMax) is a
// demilitarized zone: no deploying, no fighting.

// deployFighters is the "Fighter Bay": move fighters between the ship's holds
// and the current sector. Deployed fighters guard the sector and show up to
// every captain who warps in.
func (g *Game) deployFighters() {
	if g.tr.Sector <= FedSpaceMax {
		g.note("Federation space forbids military deployment.")
		return
	}

	for {
		deployed := g.myDeployedHere()

		g.cls("Fighter Bay")
		g.status()
		g.nl()
		g.pf("  \x1b[1;36mSector \x1b[1;37m%d\x1b[0m\r\n", g.tr.Sector)
		g.pf("  \x1b[0;37mFighters aboard ship : \x1b[1;32m%s\x1b[0m\r\n", commas(g.tr.Fighters))
		g.pf("  \x1b[0;37mYour fighters here   : \x1b[1;32m%s\x1b[0m\r\n", commas(deployed))
		g.nl()
		g.p("  \x1b[1;36mD\x1b[1;30m) \x1b[0;37mDeploy fighters from ship into the sector\x1b[0m\r\n")
		g.p("  \x1b[1;36mC\x1b[1;30m) \x1b[0;37mCollect deployed fighters back to the ship\x1b[0m\r\n")
		g.p("  \x1b[1;36mQ\x1b[1;30m) \x1b[0;37mLeave the bay\x1b[0m\r\n")

		k := g.prompt("\r\n\x1b[0;37m  Choice \x1b[1;36m> \x1b[1;37m")
		switch k {
		case 0, 'q', 27:
			return
		case 'd':
			if g.tr.Fighters <= 0 {
				g.note("No fighters aboard to deploy.")
				continue
			}
			n := g.askInt("\x1b[0;37m  Deploy how many \x1b[1;36m> \x1b[1;37m")
			if n <= 0 {
				continue
			}
			if n > g.tr.Fighters {
				n = g.tr.Fighters
			}
			if err := g.st.SetFighters(g.tr.Sector, g.tr.Handle, deployed+n); err != nil {
				g.note("The launch bay jammed -- deployment failed.")
				continue
			}
			g.tr.Fighters -= n
			g.save()
			g.pf("\r\n\x1b[1;32m  Deployed \x1b[1;37m%s\x1b[1;32m fighters to sector \x1b[1;37m%d\x1b[1;32m.\x1b[0m\r\n",
				commas(n), g.tr.Sector)
			g.pause()
		case 'c':
			if deployed <= 0 {
				g.note("You have no fighters deployed here to collect.")
				continue
			}
			n := g.askInt("\x1b[0;37m  Collect how many \x1b[1;36m> \x1b[1;37m")
			if n <= 0 {
				continue
			}
			if n > deployed {
				n = deployed
			}
			if err := g.st.SetFighters(g.tr.Sector, g.tr.Handle, deployed-n); err != nil {
				g.note("The recovery clamps slipped -- collection failed.")
				continue
			}
			g.tr.Fighters += n
			g.save()
			g.pf("\r\n\x1b[1;32m  Recovered \x1b[1;37m%s\x1b[1;32m fighters to the ship.\x1b[0m\r\n", commas(n))
			g.pause()
		default:
			// ignore stray keys
		}
	}
}

// myDeployedHere sums the fighters this trader has deployed in the current
// sector (case-insensitive owner match).
func (g *Game) myDeployedHere() int {
	figs, err := g.st.FightersAt(g.tr.Sector)
	if err != nil {
		return 0
	}
	total := 0
	for _, f := range figs {
		if equalFoldStr(f.Owner, g.tr.Handle) {
			total += f.Qty
		}
	}
	return total
}

// attackTrader is "Engage!": pick another trader parked in this sector and
// fight them ship-to-ship until one side's fighters are gone.
func (g *Game) attackTrader() {
	if g.tr.Sector <= FedSpaceMax {
		g.note("Federation patrols would vaporize you -- no combat in Fed space.")
		return
	}

	others, err := g.st.TradersInSector(g.tr.Sector, g.tr.Handle)
	if err != nil || len(others) == 0 {
		g.note("Sector scan is clear -- no other traders to engage.")
		return
	}

	g.cls("Engage!")
	g.status()
	g.nl()
	g.p("  \x1b[1;31mTargets in this sector:\x1b[0m\r\n\r\n")
	for i, o := range others {
		g.pf("  \x1b[1;36m%d\x1b[1;30m) \x1b[1;37m%-20s \x1b[1;30m(fighters \x1b[1;32m%s\x1b[1;30m, shields \x1b[1;34m%s\x1b[1;30m)\x1b[0m\r\n",
			i+1, o.Name, commas(o.Fighters), commas(o.Shields))
	}

	pick := g.askInt("\r\n\x1b[0;37m  Engage which target \x1b[1;30m(0 to abort)\x1b[1;36m > \x1b[1;37m")
	if pick < 1 || pick > len(others) {
		return
	}
	target := others[pick-1]

	if !g.yesno("\r\n\x1b[1;31m  Open fire on " + target.Name + "?\x1b[0m") {
		return
	}

	// Re-load the target fresh so we fight the latest figures.
	foe, ok, lerr := g.st.Load(target.Handle)
	if lerr != nil || !ok || foe == nil {
		g.note("Target slipped away before you could lock weapons.")
		return
	}

	if g.tr.Fighters <= 0 {
		g.note("You have no fighters aboard -- you can't attack.")
		return
	}

	g.cls("Engage!")
	g.pf("\x1b[1;31m  COMBAT \x1b[1;37m-- \x1b[1;33m%s \x1b[0;37mvs \x1b[1;33m%s\x1b[0m\r\n\r\n", g.tr.Name, foe.Name)

	myFig := g.tr.Fighters
	foeFig := foe.Fighters
	myShd := g.tr.Shields
	foeShd := foe.Shields

	const maxRounds = 50
	rounds := 0
	for myFig > 0 && foeFig > 0 && rounds < maxRounds {
		rounds++

		// Each side's damage scales with its own fighter count and a random
		// factor; the defender's shields absorb a fraction of incoming losses.
		myHit := combatDamage(myFig, g.rng.Float64())
		foeHit := combatDamage(foeFig, g.rng.Float64())

		myHit = shieldSoak(myHit, foeShd)
		foeHit = shieldSoak(foeHit, myShd)

		foeShd = drainShield(foeShd)
		myShd = drainShield(myShd)

		foeFig -= myHit
		myFig -= foeHit
		if foeFig < 0 {
			foeFig = 0
		}
		if myFig < 0 {
			myFig = 0
		}

		g.pf("  \x1b[1;30mRound %2d: \x1b[1;32myou hit \x1b[1;37m%s\x1b[1;32m, \x1b[1;31mthey hit \x1b[1;37m%s  \x1b[1;30m| you \x1b[1;32m%s\x1b[1;30m  foe \x1b[1;31m%s\x1b[0m\r\n",
			rounds, commas(myHit), commas(foeHit), commas(myFig), commas(foeFig))
	}

	myLost := g.tr.Fighters - myFig
	if myLost < 0 {
		myLost = 0
	}
	foeLost := foe.Fighters - foeFig
	if foeLost < 0 {
		foeLost = 0
	}

	g.nl()
	youWin := foeFig <= 0 && myFig > 0

	if youWin {
		// Loot 25-50% of the foe's credits.
		pct := 25 + g.rng.Intn(26)
		loot := foe.Credits * pct / 100
		if loot < 0 {
			loot = 0
		}
		if loot > foe.Credits {
			loot = foe.Credits
		}

		g.tr.Fighters = myFig
		g.tr.Credits += loot
		g.tr.Kills++

		foe.Credits -= loot
		foe.Fighters = 0
		foe.Shields = 0
		foe.Sector = 1
		_ = g.st.Save(foe)

		g.pf("\x1b[1;32m  VICTORY after %d rounds!\x1b[0m\r\n", rounds)
		g.pf("  \x1b[0;37mFighters lost: \x1b[1;31m%s\x1b[0;37m   Enemy destroyed: \x1b[1;33m%s\x1b[0m\r\n",
			commas(myLost), commas(foeLost))
		g.pf("  \x1b[1;33m%s \x1b[0;37mis crippled and limps back to Sol. You loot \x1b[1;33m%s\x1b[0;37m credits (%d%%).\x1b[0m\r\n",
			foe.Name, commas(loot), pct)
	} else {
		// Defeat: your fighters are gone, shields stripped, credits skimmed,
		// and you're bounced to Sol. The foe keeps whatever fighters remain.
		pct := 25 + g.rng.Intn(26)
		loss := g.tr.Credits * pct / 100
		if loss < 0 {
			loss = 0
		}
		if loss > g.tr.Credits {
			loss = g.tr.Credits
		}

		g.tr.Fighters = 0
		g.tr.Shields = 0
		g.tr.Credits -= loss
		g.tr.Sector = 1

		foe.Fighters = foeFig
		if foe.Fighters < 0 {
			foe.Fighters = 0
		}
		_ = g.st.Save(foe)

		g.pf("\x1b[1;31m  DEFEAT after %d rounds.\x1b[0m\r\n", rounds)
		g.pf("  \x1b[0;37mFighters lost: \x1b[1;31m%s\x1b[0;37m   You dealt: \x1b[1;33m%s\x1b[0m\r\n",
			commas(myLost), commas(foeLost))
		g.pf("  \x1b[0;37mYour hull is breached. You eject to Sol and lose \x1b[1;31m%s\x1b[0;37m credits (%d%%).\x1b[0m\r\n",
			commas(loss), pct)
	}

	if g.tr.Turns > 0 {
		g.tr.Turns--
	}
	g.save()
	g.pause()
}

// combatDamage is the fighters one side destroys this round: a fraction of its
// own fighter strength, jittered by a random factor in roughly [0.05, 0.25].
func combatDamage(fighters int, r float64) int {
	if fighters <= 0 {
		return 0
	}
	frac := 0.05 + 0.20*r
	dmg := int(float64(fighters) * frac)
	if dmg < 1 {
		dmg = 1
	}
	return dmg
}

// shieldSoak removes a fraction of incoming fighter losses proportional to the
// defender's shields (capped so shields never make a side invulnerable).
func shieldSoak(hit, shields int) int {
	if hit <= 0 {
		return 0
	}
	if shields <= 0 {
		return hit
	}
	// Up to 40% absorbed, scaling with shield strength toward a cap.
	soak := shields
	if soak > 1000 {
		soak = 1000
	}
	absorbed := hit * soak / 2500 // <= 40% at the cap
	out := hit - absorbed
	if out < 1 {
		out = 1
	}
	return out
}

// drainShield erodes a shield bank a little each round as it absorbs fire.
func drainShield(shields int) int {
	if shields <= 0 {
		return 0
	}
	shields -= shields/10 + 1
	if shields < 0 {
		shields = 0
	}
	return shields
}

// equalFoldStr is a tiny case-insensitive ASCII string compare, kept local so
// combat.go needs no extra imports.
func equalFoldStr(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lc(a[i]) != lc(b[i]) {
			return false
		}
	}
	return true
}
