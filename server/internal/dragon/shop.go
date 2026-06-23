package dragon

// shop.go holds the two town merchants: King Arthur's Weapons and Abdul's
// Armour. Both are the same buy/sell/list loop over a tier table, so they share
// the private shop() helper below.

// weaponShop is King Arthur's Weapons -- buy and sell weapon tiers.
func (g *Game) weaponShop() {
	g.shop("King Arthur's Weapons", Weapons, &g.ch.WeaponN, "weapon")
}

// armorShop is Abdul's Armour -- buy and sell armor tiers.
func (g *Game) armorShop() {
	g.shop("Abdul's Armour", Armors, &g.ch.ArmorN, "armor")
}

// shop runs the merchant loop for one tier table (items), with idx pointing at
// the character's currently equipped index into that table. kind is the noun
// ("weapon"/"armor") used in flavor text.
func (g *Game) shop(title string, items []Item, idx *int, kind string) {
	for {
		cur := clampIdx(*idx, len(items))
		g.cls(title)
		g.status()
		g.nl()

		owned := items[cur]
		g.pf("  \x1b[0;37mYou are wielding the \x1b[1;36m%s\x1b[0;37m (power \x1b[1;33m%d\x1b[0;37m).\x1b[0m\r\n\r\n",
			owned.Name, owned.Power)

		// Preview the next tier up, if any.
		if cur+1 < len(items) {
			nxt := items[cur+1]
			g.pf("  \x1b[1;30mNext: \x1b[1;36m%s\x1b[1;30m  power \x1b[1;33m%d\x1b[1;30m  cost \x1b[1;33m%s gold\x1b[0m\r\n\r\n",
				nxt.Name, nxt.Power, commas(nxt.Price))
		} else {
			g.p("  \x1b[1;33m  You already carry the finest there is.\x1b[0m\r\n\r\n")
		}

		g.p("  \x1b[1;33mB\x1b[0;37m)uy the next tier   \x1b[1;33mL\x1b[0;37m)ist all tiers\r\n")
		g.p("  \x1b[1;33mS\x1b[0;37m)ell current        \x1b[1;33mQ\x1b[0;37m)uit\r\n")

		switch g.prompt("\r\n\x1b[0;37m  Your choice \x1b[1;36m> \x1b[1;37m") {
		case 'b':
			g.shopBuy(items, idx, kind)
		case 'l':
			g.shopList(items, *idx)
		case 's':
			g.shopSell(items, idx, kind)
		case 'q', 0:
			return
		}
	}
}

// shopBuy advances to the next tier if the warrior can afford it.
func (g *Game) shopBuy(items []Item, idx *int, kind string) {
	cur := clampIdx(*idx, len(items))
	g.nl()
	if cur+1 >= len(items) {
		g.pf("\x1b[1;33m  There is no finer %s to be had. You have the best.\x1b[0m\r\n", kind)
		g.pause()
		return
	}
	nxt := items[cur+1]
	g.pf("\x1b[0;37m  The \x1b[1;36m%s\x1b[0;37m (power \x1b[1;33m%d\x1b[0;37m) costs \x1b[1;33m%s\x1b[0;37m gold.\x1b[0m\r\n",
		nxt.Name, nxt.Power, commas(nxt.Price))
	if g.ch.Gold < nxt.Price {
		g.pf("\x1b[1;31m  You can't afford that -- you hold only %s gold.\x1b[0m\r\n", commas(g.ch.Gold))
		g.pause()
		return
	}
	if !g.yesno("  Buy it?") {
		return
	}
	g.ch.Gold -= nxt.Price
	*idx = cur + 1
	g.save()
	g.pf("\r\n\x1b[1;32m  The \x1b[1;36m%s\x1b[1;32m is yours, %s! Wield it well.\x1b[0m\r\n", nxt.Name, g.ch.Name)
	g.pause()
}

// shopSell trades the current tier back for half its price and drops one tier.
func (g *Game) shopSell(items []Item, idx *int, kind string) {
	cur := clampIdx(*idx, len(items))
	g.nl()
	if cur <= 0 {
		g.pf("\x1b[1;33m  Your starting %s is worthless -- no one will buy it.\x1b[0m\r\n", kind)
		g.pause()
		return
	}
	owned := items[cur]
	refund := owned.Price / 2
	g.pf("\x1b[0;37m  Sell your \x1b[1;36m%s\x1b[0;37m for \x1b[1;33m%s\x1b[0;37m gold and drop to the \x1b[1;36m%s\x1b[0;37m?\x1b[0m\r\n",
		owned.Name, commas(refund), items[cur-1].Name)
	if !g.yesno("  Sell it?") {
		return
	}
	g.ch.Gold += refund
	*idx = cur - 1
	g.save()
	g.pf("\r\n\x1b[1;32m  Sold! You pocket %s gold.\x1b[0m\r\n", commas(refund))
	g.pause()
}

// shopList prints the full tier table, marking the owned tier and dimming any
// the warrior can't afford.
func (g *Game) shopList(items []Item, owned int) {
	owned = clampIdx(owned, len(items))
	g.nl()
	g.p("  \x1b[1;30m  # \xb3 Item             \xb3 Power \xb3      Price\x1b[0m\r\n")
	g.p("  \x1b[1;30m" + rule(40) + "\x1b[0m\r\n")
	for i, it := range items {
		mark := " "
		nameColor := "\x1b[0;37m"
		switch {
		case i == owned:
			mark = "\x1b[1;32m*"
			nameColor = "\x1b[1;36m"
		case it.Price > g.ch.Gold:
			// Can't afford -- dim the whole row.
			nameColor = "\x1b[1;30m"
		}
		priceColor := "\x1b[1;33m"
		if i != owned && it.Price > g.ch.Gold {
			priceColor = "\x1b[1;30m"
		}
		g.pf("  \x1b[1;30m%s%2d \xb3 %s%-16s\x1b[1;30m\xb3 \x1b[0;37m%5d \x1b[1;30m\xb3 %s%9s\x1b[0m\r\n",
			mark, i, nameColor, it.Name, it.Power, priceColor, commas(it.Price))
	}
	g.nl()
	g.pf("  \x1b[1;30mYou hold \x1b[1;33m%s\x1b[1;30m gold. \x1b[1;32m*\x1b[1;30m marks your current gear.\x1b[0m\r\n",
		commas(g.ch.Gold))
	g.pause()
}
