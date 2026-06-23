package tw

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestGalaxyGenerated(t *testing.T) {
	s := testStore(t)
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tw_sectors`).Scan(&n); err != nil || n != TotalSectors {
		t.Fatalf("sectors = %d (err %v), want %d", n, err, TotalSectors)
	}
	// Specials.
	if p, _ := s.PortAt(StarDockSector); p == nil || !p.IsStarDock() {
		t.Fatalf("no StarDock at sector %d: %+v", StarDockSector, p)
	}
	if p, _ := s.PortAt(TerraSector); p == nil || !p.IsTerra() {
		t.Fatalf("no Terra at sector %d: %+v", TerraSector, p)
	}
	// Some trading ports exist.
	trading := 0
	for i := 1; i <= TotalSectors; i++ {
		if p, _ := s.PortAt(i); p != nil && p.Class >= 1 && p.Class <= 8 {
			trading++
		}
	}
	if trading < 10 {
		t.Fatalf("too few trading ports: %d", trading)
	}
}

func TestGalaxyConnected(t *testing.T) {
	s := testStore(t)
	seen := map[int]bool{1: true}
	queue := []int{1}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		sec, err := s.Sector(cur)
		if err != nil || sec == nil {
			t.Fatalf("Sector(%d): %v", cur, err)
		}
		for _, w := range sec.Warps {
			if !seen[w] {
				seen[w] = true
				queue = append(queue, w)
			}
		}
	}
	if len(seen) != TotalSectors {
		t.Fatalf("galaxy not fully connected: reached %d of %d sectors", len(seen), TotalSectors)
	}
}

func TestWarpsSymmetric(t *testing.T) {
	s := testStore(t)
	sec, _ := s.Sector(1)
	if len(sec.Warps) == 0 {
		t.Fatal("sector 1 has no warps")
	}
	for _, w := range sec.Warps {
		back, _ := s.WarpExists(w, 1)
		if !back {
			t.Fatalf("warp 1->%d not mirrored %d->1", w, w)
		}
	}
}

func TestPricingAndClasses(t *testing.T) {
	// Class codes drive buy/sell.
	if !PortSells(1, Equ) || !PortBuys(1, Ore) { // class 1 = BBS
		t.Fatal("class 1 (BBS) buy/sell wrong")
	}
	if !PortSells(8, Ore) == false { // class 8 = BBB, buys all, sells none
		// PortSells(8, Ore) should be false
	}
	if PortSells(8, Ore) {
		t.Fatal("class 8 (BBB) should sell nothing")
	}
	// Selling price (you buy) is cheaper at full stock than near-empty.
	if SellUnitPrice(Ore, MaxStock) >= SellUnitPrice(Ore, 0) {
		t.Fatal("buying should be cheaper when the port is full")
	}
	// Buy price (port pays you) is higher when the port is empty of it.
	if BuyUnitPrice(Ore, 0) <= BuyUnitPrice(Ore, MaxStock) {
		t.Fatal("a port should pay more for goods it lacks")
	}
}

func TestTraderRoundTrip(t *testing.T) {
	s := testStore(t)
	if _, found, err := s.Load("nobody"); err != nil || found {
		t.Fatalf("Load missing: %v %v", found, err)
	}
	tr := NewTrader("phiber", "Capt Phiber")
	tr.Credits = 50000
	tr.Cargo[Org] = 12
	tr.Sector = 42
	tr.Fighters = 300
	if err := s.Save(tr); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, found, err := s.Load("phiber")
	if err != nil || !found {
		t.Fatalf("Load: %v %v", found, err)
	}
	if got.Name != "Capt Phiber" || got.Credits != 50000 || got.Cargo[Org] != 12 ||
		got.Sector != 42 || got.Fighters != 300 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.HoldsUsed() != 12 || got.HoldsFree() != got.HoldsMax-12 {
		t.Fatalf("holds math wrong: used=%d free=%d", got.HoldsUsed(), got.HoldsFree())
	}
	if got.NetWorth() <= got.Credits {
		t.Fatal("net worth should exceed bare credits with assets aboard")
	}
}

func TestDailyReset(t *testing.T) {
	tr := NewTrader("h", "h")
	tr.Turns = 0
	tr.LastPlayed = "2000-01-01"
	if !tr.ResetIfNewDay() {
		t.Fatal("expected reset")
	}
	if tr.Turns != DailyTurns {
		t.Fatalf("turns = %d, want %d", tr.Turns, DailyTurns)
	}
	if tr.ResetIfNewDay() {
		t.Fatal("second reset same day should be no-op")
	}
}

func TestFightersAndTraders(t *testing.T) {
	s := testStore(t)
	if err := s.SetFighters(42, "alice", 100); err != nil {
		t.Fatalf("SetFighters: %v", err)
	}
	if err := s.SetFighters(42, "bob", 50); err != nil {
		t.Fatal(err)
	}
	figs, _ := s.FightersAt(42)
	if len(figs) != 2 || figs[0].Qty != 100 {
		t.Fatalf("FightersAt: %+v", figs)
	}
	// Clearing removes the row.
	s.SetFighters(42, "bob", 0)
	if figs, _ := s.FightersAt(42); len(figs) != 1 {
		t.Fatalf("clear failed: %+v", figs)
	}

	// Traders in a sector / rankings.
	a := NewTrader("a", "A")
	a.Sector = 7
	a.Credits = 100
	b := NewTrader("b", "B")
	b.Sector = 7
	b.Credits = 999999
	s.Save(a)
	s.Save(b)
	in, _ := s.TradersInSector(7, "a")
	if len(in) != 1 || in[0].Handle != "b" {
		t.Fatalf("TradersInSector: %+v", in)
	}
	rank, _ := s.Rankings(10)
	if len(rank) < 2 || rank[0].Handle != "b" {
		t.Fatalf("Rankings order wrong: %+v", rank)
	}
}
