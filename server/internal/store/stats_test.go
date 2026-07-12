package store

import (
	"testing"
	"time"
)

func TestTotalsAndTopBoards(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	tot, err := s.Totals()
	if err != nil {
		t.Fatalf("Totals: %v", err)
	}
	if tot.Users != 3 {
		t.Errorf("Totals.Users = %d, want 3 (seeded)", tot.Users)
	}
	if tot.Posts != 3 {
		t.Errorf("Totals.Posts = %d, want 3 (seeded)", tot.Posts)
	}
	if tot.Files != 8 {
		t.Errorf("Totals.Files = %d, want 8 (seeded)", tot.Files)
	}
	if tot.StoredBytes <= 0 {
		t.Errorf("Totals.StoredBytes = %d, want > 0", tot.StoredBytes)
	}

	top, err := s.TopBoards(10)
	if err != nil {
		t.Fatalf("TopBoards: %v", err)
	}
	if len(top) == 0 {
		t.Fatal("TopBoards returned nothing")
	}
	// Busiest first: each count >= the next.
	for i := 1; i < len(top); i++ {
		if top[i-1].Count < top[i].Count {
			t.Errorf("TopBoards not sorted descending: %+v", top)
		}
	}
}

func TestDailyActivityZeroFilledAndCountsToday(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	// Post one message stamped now, so today's bucket picks it up.
	if _, err := s.PostMessage(&Message{BoardID: 1, From: "nut", Subject: "hi", Body: "now", Posted: time.Now()}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	days, err := s.DailyActivity(14)
	if err != nil {
		t.Fatalf("DailyActivity: %v", err)
	}
	if len(days) != 14 {
		t.Fatalf("DailyActivity len = %d, want 14 (zero-filled)", len(days))
	}
	// Oldest first, newest last.
	today := time.Now().Format("2006-01-02")
	last := days[len(days)-1]
	if last.Day != today {
		t.Errorf("last bucket = %s, want today %s", last.Day, today)
	}
	if last.Posts < 1 {
		t.Errorf("today's post count = %d, want >= 1", last.Posts)
	}
}
