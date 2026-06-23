package tw

// portTrade runs the docked-at-port trading screen for a class 1..8 port.
// The captain buys commodities the port SELLS and sells commodities the port
// BUYS, with prices sliding against the port's stock so the economy responds to
// every deal. Loops until the captain quits or the line drops.
func (g *Game) portTrade() {
	for {
		g.cls("Trading Port")
		g.status()
		g.cargoLine()
		g.nl()

		g.pf("  \x1b[1;33mClass %d \x1b[1;37m(%s)\x1b[1;30m -- the port's standing orders:\x1b[0m\r\n\r\n",
			g.port.Class, ClassCode(g.port.Class))

		// One line per commodity: whether the port buys or sells it and at what
		// per-unit price right now.
		for c := 0; c < NumCommodities; c++ {
			switch {
			case g.port.Class >= 1 && g.port.Class <= 8 && PortSells(g.port.Class, c):
				price := SellUnitPrice(c, g.port.Stock[c])
				g.pf("  \x1b[1;36m%c\x1b[1;30m) \x1b[0;37m%-10s \x1b[1;32mSELLS \x1b[0;37mto you \x1b[1;33m%s\x1b[0;37m cr/unit \x1b[1;30m(stock %s)\x1b[0m\r\n",
					'1'+c, CommodityName[c], commas(price), commas(g.port.Stock[c]))
			case PortBuys(g.port.Class, c):
				price := BuyUnitPrice(c, g.port.Stock[c])
				g.pf("  \x1b[1;36m%c\x1b[1;30m) \x1b[0;37m%-10s \x1b[1;31mBUYS \x1b[0;37mfrom you \x1b[1;33m%s\x1b[0;37m cr/unit \x1b[1;30m(holding %s)\x1b[0m\r\n",
					'1'+c, CommodityName[c], commas(price), commas(g.port.Stock[c]))
			default:
				g.pf("  \x1b[1;30m%c) %-10s -- not traded here\x1b[0m\r\n", '1'+c, CommodityName[c])
			}
		}

		k := g.prompt("\r\n\x1b[0;37m  Pick a commodity \x1b[1;30m(1-3)\x1b[0;37m, or \x1b[1;36mQ\x1b[0;37muit \x1b[1;36m> \x1b[1;37m")
		if k == 0 || k == 'q' {
			g.save()
			return
		}
		if k < '1' || k > '0'+NumCommodities {
			continue
		}
		c := int(k - '1')

		switch {
		case PortSells(g.port.Class, c):
			g.doBuy(c)
		case PortBuys(g.port.Class, c):
			g.doSell(c)
		default:
			g.note("This port doesn't trade " + CommodityName[c] + ".")
		}
	}
}

// doBuy lets the captain buy commodity c from the port. Amount is clamped to
// holds free, credits on hand, and the port's stock so nothing can overflow.
func (g *Game) doBuy(c int) {
	price := SellUnitPrice(c, g.port.Stock[c])
	maxAmt := g.tr.HoldsFree()
	if byCredits := g.tr.Credits / price; byCredits < maxAmt {
		maxAmt = byCredits
	}
	if g.port.Stock[c] < maxAmt {
		maxAmt = g.port.Stock[c]
	}
	if maxAmt < 0 {
		maxAmt = 0
	}
	if maxAmt == 0 {
		g.note("You can't buy any " + CommodityName[c] + " right now (holds, credits, or stock).")
		return
	}

	g.pf("\r\n\x1b[0;37m  Buying \x1b[1;33m%s\x1b[0;37m at \x1b[1;33m%s\x1b[0;37m cr/unit. You can take up to \x1b[1;32m%s\x1b[0;37m.\x1b[0m\r\n",
		CommodityName[c], commas(price), commas(maxAmt))
	amt := g.askInt("\x1b[0;37m  How many units \x1b[1;36m> \x1b[1;37m")
	if amt <= 0 {
		return
	}
	if amt > maxAmt {
		amt = maxAmt
	}

	cost := amt * price
	g.tr.Credits -= cost
	g.tr.Cargo[c] += amt
	g.port.Stock[c] -= amt
	if g.port.Stock[c] < 0 {
		g.port.Stock[c] = 0
	}
	g.persist()

	g.pf("\r\n\x1b[1;32m  Bought \x1b[1;37m%s\x1b[1;32m units of \x1b[1;37m%s\x1b[1;32m for \x1b[1;31m%s\x1b[1;32m credits.\x1b[0m\r\n",
		commas(amt), CommodityName[c], commas(cost))
	g.pause()
}

// doSell lets the captain sell commodity c to the port. Amount is clamped to
// cargo on hand and the room left in the port (MaxStock - current holding).
func (g *Game) doSell(c int) {
	price := BuyUnitPrice(c, g.port.Stock[c])
	maxAmt := g.tr.Cargo[c]
	if room := MaxStock - g.port.Stock[c]; room < maxAmt {
		maxAmt = room
	}
	if maxAmt < 0 {
		maxAmt = 0
	}
	if maxAmt == 0 {
		g.note("You have nothing to sell here, or the port is full of " + CommodityName[c] + ".")
		return
	}

	g.pf("\r\n\x1b[0;37m  Selling \x1b[1;33m%s\x1b[0;37m at \x1b[1;33m%s\x1b[0;37m cr/unit. You can offload up to \x1b[1;32m%s\x1b[0;37m.\x1b[0m\r\n",
		CommodityName[c], commas(price), commas(maxAmt))
	amt := g.askInt("\x1b[0;37m  How many units \x1b[1;36m> \x1b[1;37m")
	if amt <= 0 {
		return
	}
	if amt > maxAmt {
		amt = maxAmt
	}

	gain := amt * price
	g.tr.Credits += gain
	g.tr.Cargo[c] -= amt
	if g.tr.Cargo[c] < 0 {
		g.tr.Cargo[c] = 0
	}
	g.port.Stock[c] += amt
	if g.port.Stock[c] > MaxStock {
		g.port.Stock[c] = MaxStock
	}
	g.persist()

	g.pf("\r\n\x1b[1;32m  Sold \x1b[1;37m%s\x1b[1;32m units of \x1b[1;37m%s\x1b[1;32m for \x1b[1;33m%s\x1b[1;32m credits.\x1b[0m\r\n",
		commas(amt), CommodityName[c], commas(gain))
	g.pause()
}

// persist writes the mutated port and trader back to the store. Failures are
// non-fatal: g.save logs its own errors and a port write is best-effort.
func (g *Game) persist() {
	_ = g.st.SavePort(g.port)
	g.save()
}
