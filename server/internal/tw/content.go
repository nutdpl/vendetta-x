// Package tw is "Trade Wars" -- a native, persistent TradeWars-2002-style BBS
// door. A shared galaxy of sectors, warps, and trading ports lives in SQLite;
// each board account flies one persistent ship through it. It renders over a
// term.Session and plugs into the board like any other native door.
//
// content.go: the commodities, port classes, the price model, and ship-upgrade
// costs -- the pure economic rules the rest of the game builds on.
package tw

// Universe-wide constants.
const (
	TotalSectors = 100  // size of the galaxy
	FedSpaceMax  = 10   // sectors 1..10 are protected Federation space
	StartSector  = 1    // Sol / Terra -- where new traders begin
	DailyTurns   = 1000 // movement/action turns refilled each day
	StartCredits = 1000
	StartHolds   = 20
	MaxStock     = 5000 // a port's max stock of one commodity
)

// Commodities.
const (
	Ore = iota // Fuel Ore
	Org        // Organics
	Equ        // Equipment
	NumCommodities
)

// CommodityName / CommodityShort for display.
var CommodityName = [NumCommodities]string{"Fuel Ore", "Organics", "Equipment"}
var CommodityShort = [NumCommodities]string{"Ore", "Org", "Equ"}

// basePrice is the reference credits-per-unit of each commodity.
var basePrice = [NumCommodities]int{15, 25, 35}

// portClassCode maps a port class (1..8) to its three-letter Buy/Sell code,
// position 0=Ore, 1=Organics, 2=Equipment. 'B' = the port BUYS it from you
// (you sell), 'S' = the port SELLS it to you (you buy). Class 0 = StarDock,
// class 9 = Terra (special, handled elsewhere).
var portClassCode = map[int]string{
	1: "BBS", 2: "BSB", 3: "SBB", 4: "SSB",
	5: "SBS", 6: "BSS", 7: "SSS", 8: "BBB",
}

// ClassCode returns the BBS/SSB/... code for a class, or "" for specials.
func ClassCode(class int) string { return portClassCode[class] }

// PortBuys reports whether a port of this class buys commodity c (you sell to it).
func PortBuys(class, c int) bool {
	code := portClassCode[class]
	return c >= 0 && c < len(code) && code[c] == 'B'
}

// PortSells reports whether a port of this class sells commodity c (you buy).
func PortSells(class, c int) bool {
	code := portClassCode[class]
	return c >= 0 && c < len(code) && code[c] == 'S'
}

// SellUnitPrice is what the port charges YOU per unit of commodity c when it
// sells it (you are buying). It is cheapest when the port is fully stocked and
// dearest when nearly empty -- so you buy where goods are plentiful.
func SellUnitPrice(c, stock int) int {
	r := ratio(stock)
	// 0.80x at full stock .. 1.20x when empty.
	p := float64(basePrice[c]) * (1.20 - 0.40*r)
	return atLeast1(int(p))
}

// BuyUnitPrice is what the port pays YOU per unit of commodity c when it buys
// it (you are selling). It pays best when it holds little of that good and
// least when it is brimming -- so you sell where goods are scarce.
func BuyUnitPrice(c, stock int) int {
	r := ratio(stock)
	// 1.20x when empty .. 0.80x at full stock.
	p := float64(basePrice[c]) * (0.80 + 0.40*(1.0-r))
	return atLeast1(int(p))
}

func ratio(stock int) float64 {
	if stock < 0 {
		stock = 0
	}
	if stock > MaxStock {
		stock = MaxStock
	}
	return float64(stock) / float64(MaxStock)
}

func atLeast1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// ---- StarDock ship-upgrade costs -------------------------------------------

const (
	HoldPrice    = 500  // credits per extra cargo hold
	FighterPrice = 50   // credits per fighter
	ShieldPrice  = 40   // credits per shield point
	MaxHolds     = 1000 // cap on cargo holds
	MaxFighters  = 100000
	MaxShields   = 5000
)
