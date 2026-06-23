package tw

// stardock.go: the StarDock outfitting screen -- "Galactic Outfitters". When a
// captain docks at the special port in Federation space (sector 5), this is the
// shipyard floor where they buy cargo holds, fighters, and shields. Pure
// commerce: no combat, no trading of commodities -- just spending credits to
// grow the ship.

// starDock runs the StarDock outfitting menu loop until the captain leaves (Q)
// or the line drops (0).
func (g *Game) starDock() {
	for {
		g.cls("StarDock")
		g.status()
		g.nl()

		g.p("\x1b[1;35m  StarDock \x1b[1;30m-- \x1b[0;37mGalactic Outfitters\x1b[0m\r\n")
		g.p("\x1b[1;30m  Welders spark and gantry cranes swing overhead; a hundred ships\r\n")
		g.p("  jostle for a berth. Quartermaster's terminal blinks ready.\x1b[0m\r\n\r\n")

		g.pf("\x1b[1;36m  (H) \x1b[1;37mCargo Holds  \x1b[1;30m%6s cr each   \x1b[0;37mmax %s\x1b[0m\r\n",
			commas(HoldPrice), commas(MaxHolds))
		g.pf("\x1b[1;36m  (F) \x1b[1;37mFighters     \x1b[1;30m%6s cr each   \x1b[0;37mmax %s aboard\x1b[0m\r\n",
			commas(FighterPrice), commas(MaxFighters))
		g.pf("\x1b[1;36m  (S) \x1b[1;37mShields      \x1b[1;30m%6s cr each   \x1b[0;37mmax %s\x1b[0m\r\n",
			commas(ShieldPrice), commas(MaxShields))
		g.p("\x1b[1;36m  (Q) \x1b[1;37mLeave dock\x1b[0m\r\n")

		switch g.prompt("\r\n\x1b[0;37m  Outfit \x1b[1;36m> \x1b[1;37m") {
		case 'h':
			g.buyUpgrade("Cargo Holds", HoldPrice, g.tr.HoldsMax, MaxHolds, func(qty int) {
				g.tr.HoldsMax += qty
			})
		case 'f':
			g.buyUpgrade("Fighters", FighterPrice, g.tr.Fighters, MaxFighters, func(qty int) {
				g.tr.Fighters += qty
			})
		case 's':
			g.buyUpgrade("Shields", ShieldPrice, g.tr.Shields, MaxShields, func(qty int) {
				g.tr.Shields += qty
			})
		case 'q':
			g.cls("StarDock")
			g.p("\x1b[0;37m  The gantry retracts and your berth clamps release.\x1b[0m\r\n")
			g.p("\x1b[1;30m  \"Fly safe out there, Captain.\"\x1b[0m\r\n")
			g.pause()
			return
		case 0:
			return
		default:
			g.note("No such item on the manifest.")
		}
	}
}

// buyUpgrade handles one purchase line: prices the item, shows what the captain
// can afford and the headroom under the cap, asks a quantity, clamps it so it
// never overspends or breaches the cap, charges credits, applies it via add,
// saves, and confirms.
func (g *Game) buyUpgrade(name string, unit, have, cap int, add func(qty int)) {
	g.nl()
	g.pf("\x1b[1;35m  %s \x1b[1;30m-- \x1b[1;33m%s cr \x1b[0;37meach\x1b[0m\r\n", name, commas(unit))

	room := cap - have
	if room <= 0 {
		g.note("Your " + name + " are already maxed out.")
		return
	}

	canAfford := g.tr.Credits / unit
	if canAfford <= 0 {
		g.note("You can't afford even one. Come back richer, Captain.")
		return
	}

	// The most they could take this visit, bounded by both wallet and cap.
	maxBuy := canAfford
	if room < maxBuy {
		maxBuy = room
	}

	g.pf("\x1b[1;30m  You hold \x1b[1;33m%s cr\x1b[1;30m -- enough for \x1b[1;32m%s\x1b[1;30m. Room for \x1b[1;36m%s\x1b[1;30m more.\x1b[0m\r\n",
		commas(g.tr.Credits), commas(canAfford), commas(room))

	qty := g.askInt("\x1b[0;37m  How many? \x1b[1;36m> \x1b[1;37m")
	if qty <= 0 {
		g.note("Order cancelled.")
		return
	}
	if qty > maxBuy {
		qty = maxBuy // clamp: never overspend, never exceed the cap
	}

	cost := qty * unit
	g.tr.Credits -= cost
	add(qty)
	g.save()

	g.pf("\r\n\x1b[1;32m  Sold! \x1b[1;37m%s %s \x1b[0;37mfor \x1b[1;33m%s cr\x1b[0;37m.\x1b[0m\r\n",
		commas(qty), name, commas(cost))
	g.pf("\x1b[1;30m  Balance: \x1b[1;33m%s cr\x1b[1;30m.\x1b[0m\r\n", commas(g.tr.Credits))
	g.pause()
}
