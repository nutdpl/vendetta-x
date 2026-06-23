package tw

import "strconv"

// computer is the Ship's Computer sub-menu: cargo, local/area scans, and a
// nearest-port finder. It loops until the captain quits back to the bridge.
func (g *Game) computer() {
	for {
		g.cls("Ship's Computer")
		g.p("  \x1b[1;36mShip's Computer online.\x1b[0m\r\n\r\n")
		g.p("  \x1b[1;33m(H)\x1b[0;37m Holds report      \xc2\xb7 cargo manifest\x1b[0m\r\n")
		g.p("  \x1b[1;33m(S)\x1b[0;37m Local scan        \xc2\xb7 ports next door\x1b[0m\r\n")
		g.p("  \x1b[1;33m(P)\x1b[0;37m Find nearest port \xc2\xb7 route to trade\x1b[0m\r\n")
		g.p("  \x1b[1;33m(R)\x1b[0;37m Rankings          \xc2\xb7 galactic leaders\x1b[0m\r\n")
		g.p("  \x1b[1;33m(Q)\x1b[0;37m Quit computer\x1b[0m\r\n")

		switch g.prompt("\r\n\x1b[0;37m  Computer \x1b[1;36m> \x1b[1;37m") {
		case 'h':
			g.holdsReport()
		case 's':
			g.localScan()
		case 'p':
			g.findNearestPort()
		case 'r':
			g.rankings()
		case 'q', 27, 0:
			return
		}
	}
}

// holdsReport prints the cargo manifest with holds used/free.
func (g *Game) holdsReport() {
	t := g.tr
	g.cls("Holds Report")
	g.pf("  \x1b[1;36mCargo manifest \x1b[1;30m-- \x1b[0;37mhold capacity \x1b[1;37m%d\x1b[0m\r\n\r\n", t.HoldsMax)
	for c := 0; c < NumCommodities; c++ {
		g.pf("  \x1b[1;36m%-10s \x1b[1;30m(%s)\x1b[1;30m\xb3 \x1b[1;37m%5d\x1b[0;37m units\x1b[0m\r\n",
			CommodityName[c], CommodityShort[c], t.Cargo[c])
	}
	g.p("  \x1b[1;30m" + rule(40) + "\x1b[0m\r\n")
	g.pf("  \x1b[1;36m%-10s          \x1b[1;30m\xb3 \x1b[1;33m%5d\x1b[0;37m used\x1b[0m\r\n", "Holds", t.HoldsUsed())
	g.pf("  \x1b[1;36m%-10s          \x1b[1;30m\xb3 \x1b[1;32m%5d\x1b[0;37m free\x1b[0m\r\n", "", t.HoldsFree())
	g.pause()
}

// localScan lists each adjacent sector and what kind of port (if any) waits there.
func (g *Game) localScan() {
	g.cls("Local Scan")
	sec, _ := g.st.Sector(g.tr.Sector)
	if sec == nil || len(sec.Warps) == 0 {
		g.p("  \x1b[0;37mNo warp lanes detected from this sector.\x1b[0m\r\n")
		g.pause()
		return
	}
	g.pf("  \x1b[1;36mScanning from sector \x1b[1;37m%d\x1b[1;30m...\x1b[0m\r\n\r\n", sec.ID)
	for _, dst := range sec.Warps {
		desc := scanPortDesc(g, dst)
		g.pf("  \x1b[1;36mWarp \xc4\xba \x1b[1;37m%-4d \x1b[1;30m\xb3 %s\x1b[0m\r\n", dst, desc)
	}
	g.pause()
}

// scanPortDesc describes the port (if any) in a sector for the local scan.
func scanPortDesc(g *Game, sector int) string {
	port, _ := g.st.PortAt(sector)
	if port == nil {
		return "\x1b[1;30mno port"
	}
	switch {
	case port.IsStarDock():
		return "\x1b[1;35mStarDock \x1b[1;30m(upgrades)"
	case port.IsTerra():
		return "\x1b[1;32mTerra \x1b[1;30m(homeworld)"
	case port.Class >= 1 && port.Class <= 8:
		return "\x1b[1;33mClass " + strconv.Itoa(port.Class) + " \x1b[1;37m(" + ClassCode(port.Class) + ")"
	default:
		return "\x1b[1;30mno port"
	}
}

// findNearestPort runs a breadth-first search outward from the player's sector
// to the closest trading port (class 1..8), and reports the hop-by-hop route.
func (g *Game) findNearestPort() {
	g.cls("Find Nearest Port")
	start := g.tr.Sector

	visited := map[int]bool{start: true}
	prev := map[int]int{}
	queue := []int{start}
	examined := 0
	target := -1

	for len(queue) > 0 && examined < TotalSectors {
		cur := queue[0]
		queue = queue[1:]
		examined++

		// Don't accept the starting sector itself as the answer.
		if cur != start {
			if port, _ := g.st.PortAt(cur); port != nil && port.Class >= 1 && port.Class <= 8 {
				target = cur
				break
			}
		}

		sec, _ := g.st.Sector(cur)
		if sec == nil {
			continue
		}
		for _, dst := range sec.Warps {
			if !visited[dst] {
				visited[dst] = true
				prev[dst] = cur
				queue = append(queue, dst)
			}
		}
	}

	if target < 0 {
		g.p("  \x1b[0;37mNo trading port found within reach of this sector.\x1b[0m\r\n")
		g.pause()
		return
	}

	// Rebuild the route start -> target.
	var route []int
	for at := target; ; at = prev[at] {
		route = append([]int{at}, route...)
		if at == start {
			break
		}
	}

	if port, _ := g.st.PortAt(target); port != nil {
		g.pf("  \x1b[1;36mNearest trading port: sector \x1b[1;37m%d \x1b[1;33m(Class %d %s)\x1b[0m\r\n",
			target, port.Class, ClassCode(port.Class))
	} else {
		g.pf("  \x1b[1;36mNearest trading port: sector \x1b[1;37m%d\x1b[0m\r\n", target)
	}
	g.pf("  \x1b[1;30m%d hop(s) away.\x1b[0m\r\n\r\n", len(route)-1)

	var parts []string
	for _, s := range route {
		parts = append(parts, strconv.Itoa(s))
	}
	g.p("  \x1b[1;36mRoute: \x1b[1;37m")
	for i, s := range parts {
		if i > 0 {
			g.p(" \x1b[1;30m\xc4>\x1b[1;37m ")
		}
		g.p(s)
	}
	g.p("\x1b[0m\r\n")
	g.pause()
}

// rankings is the Galactic Rankings board: the top traders by net worth.
func (g *Game) rankings() {
	g.cls("Galactic Rankings")
	top, _ := g.st.Rankings(15)
	if len(top) == 0 {
		g.p("  \x1b[0;37mThe galaxy has no traders yet. Be the first to make your mark.\x1b[0m\r\n")
		g.pause()
		return
	}

	g.pf("  \x1b[1;30m%-3s %-18s %14s %8s %9s\x1b[0m\r\n", "#", "Captain", "Net Worth", "Sector", "Fighters")
	g.p("  \x1b[1;30m" + rule(56) + "\x1b[0m\r\n")
	for i, t := range top {
		mark := " "
		nameColor := "\x1b[1;37m"
		if t.Handle == g.tr.Handle {
			mark = "\x1b[1;32m>"
			nameColor = "\x1b[1;32m"
		}
		g.pf("  %s\x1b[1;33m%-2d %s%-18s \x1b[0;37m%14s \x1b[1;36m%8d \x1b[1;32m%9s\x1b[0m\r\n",
			mark, i+1, nameColor, truncName(t.Name, 18), commas(t.NetWorth()), t.Sector, commas(t.Fighters))
	}
	g.pause()
}

// truncName clips a captain name to fit the rankings column.
func truncName(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
