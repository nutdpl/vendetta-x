package dragon

import (
	"database/sql"
	"math/rand"
	"testing"
	"time"

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

func TestCharacterRoundTrip(t *testing.T) {
	s := testStore(t)

	if _, found, err := s.Load("nobody"); err != nil || found {
		t.Fatalf("Load missing: found=%v err=%v", found, err)
	}

	c := NewCharacter("phiber", "Phiber Optik")
	c.Level = 4
	c.Exp = 12345
	c.Gold = 9999
	c.Bank = 500
	c.WeaponN = 3
	c.ArmorN = 2
	c.Kills = 7
	if err := s.Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, found, err := s.Load("phiber")
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.Name != "Phiber Optik" || got.Level != 4 || got.Exp != 12345 ||
		got.Gold != 9999 || got.Bank != 500 || got.WeaponN != 3 || got.ArmorN != 2 || got.Kills != 7 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Save again (upsert) and confirm one row, updated.
	got.Gold = 1
	if err := s.Save(got); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	again, _, _ := s.Load("phiber")
	if again.Gold != 1 {
		t.Fatalf("upsert didn't update: gold=%d", again.Gold)
	}
}

func TestOthersOrderAndExclude(t *testing.T) {
	s := testStore(t)
	mk := func(h string, lvl int) {
		c := NewCharacter(h, h)
		c.Level = lvl
		if err := s.Save(c); err != nil {
			t.Fatal(err)
		}
	}
	mk("me", 5)
	mk("weak", 1)
	mk("strong", 9)

	others, err := s.Others("me", 10)
	if err != nil {
		t.Fatalf("Others: %v", err)
	}
	if len(others) != 2 {
		t.Fatalf("Others len = %d, want 2 (excludes me)", len(others))
	}
	if others[0].Handle != "strong" || others[1].Handle != "weak" {
		t.Fatalf("Others order wrong: %s, %s", others[0].Handle, others[1].Handle)
	}
}

func TestResetIfNewDay(t *testing.T) {
	c := NewCharacter("h", "h")
	c.ForestFights = 0
	c.PlayerFights = 0
	c.Alive = false
	c.HP = 0
	c.LastPlayed = "2000-01-01"

	if !c.ResetIfNewDay() {
		t.Fatal("expected a reset on a new day")
	}
	if c.ForestFights != DailyForestFights || c.PlayerFights != DailyPlayerFights ||
		!c.Alive || c.HP != c.MaxHP {
		t.Fatalf("reset incomplete: %+v", c)
	}
	if c.LastPlayed != time.Now().Format("2006-01-02") {
		t.Fatalf("LastPlayed not stamped: %q", c.LastPlayed)
	}
	// Second call same day is a no-op.
	if c.ResetIfNewDay() {
		t.Fatal("second reset same day should be a no-op")
	}
}

func TestEquipmentPower(t *testing.T) {
	c := NewCharacter("h", "h") // base attack 6, defense 2, gear 0
	if c.EffAttack() != 6 || c.EffDefense() != 2 {
		t.Fatalf("base eff: atk=%d def=%d", c.EffAttack(), c.EffDefense())
	}
	c.WeaponN = 2 // Dagger, power 10
	c.ArmorN = 2  // Leather Vest, power 10
	if c.EffAttack() != 16 || c.EffDefense() != 12 {
		t.Fatalf("equipped eff: atk=%d def=%d", c.EffAttack(), c.EffDefense())
	}
	// Out-of-range indices clamp, never panic.
	if WeaponPower(999) != Weapons[len(Weapons)-1].Power || WeaponPower(-1) != 0 {
		t.Fatal("WeaponPower clamp wrong")
	}
}

func TestMonsterScalesWithLevel(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	avg := func(level, n int) float64 {
		sum := 0
		for i := 0; i < n; i++ {
			sum += RollMonster(level, rng).HP
		}
		return float64(sum) / float64(n)
	}
	low, high := avg(1, 500), avg(10, 500)
	if !(high > low*3) {
		t.Fatalf("monsters should scale up: lvl1 avg HP %.1f, lvl10 avg HP %.1f", low, high)
	}
}

func TestDamageFloorAndChallenge(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 1000; i++ {
		if d := Damage(1, 9999, rng); d < 1 {
			t.Fatalf("Damage went below 1: %d", d)
		}
	}
	c := NewCharacter("h", "h") // level 1, exp 0
	if CanChallengeMaster(c) {
		t.Fatal("should not be able to challenge with 0 exp")
	}
	c.Exp = Masters[1].ExpNeeded
	if !CanChallengeMaster(c) {
		t.Fatal("should be able to challenge at the threshold")
	}
}

func TestCommas(t *testing.T) {
	cases := map[int]string{0: "0", 42: "42", 1000: "1,000", 1234567: "1,234,567", -1000: "-1,000"}
	for in, want := range cases {
		if got := commas(in); got != want {
			t.Errorf("commas(%d) = %q, want %q", in, got, want)
		}
	}
}
