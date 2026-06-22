package store

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateAndSeed(t *testing.T) {
	s := newTestStore(t)

	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	// Seed must be idempotent.
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed (second call): %v", err)
	}

	boards, err := s.Boards()
	if err != nil {
		t.Fatalf("Boards: %v", err)
	}
	if len(boards) != 4 {
		t.Fatalf("expected 4 boards after seed, got %d", len(boards))
	}

	areas, err := s.FileAreas()
	if err != nil {
		t.Fatalf("FileAreas: %v", err)
	}
	if len(areas) != 3 {
		t.Fatalf("expected 3 file areas, got %d", len(areas))
	}

	liners, err := s.Oneliners(0)
	if err != nil {
		t.Fatalf("Oneliners: %v", err)
	}
	if len(liners) < 3 {
		t.Fatalf("expected at least 3 oneliners, got %d", len(liners))
	}
}

func TestUserRoundTrip(t *testing.T) {
	s := newTestStore(t)

	id, err := s.AddUser(&User{
		Handle:    "Acidburn",
		RealName:  "Kate",
		SL:        100,
		FirstCall: time.Now(),
		LastCall:  time.Now(),
	})
	if err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if id == 0 {
		t.Fatal("AddUser returned zero id")
	}

	// Exact case.
	u, err := s.UserByHandle("Acidburn")
	if err != nil {
		t.Fatalf("UserByHandle exact: %v", err)
	}
	if u == nil || u.ID != id || u.SL != 100 {
		t.Fatalf("unexpected user: %+v", u)
	}

	// Case-insensitive.
	u2, err := s.UserByHandle("ACIDBURN")
	if err != nil {
		t.Fatalf("UserByHandle ci: %v", err)
	}
	if u2 == nil || u2.ID != id {
		t.Fatalf("case-insensitive lookup failed: %+v", u2)
	}

	// Miss returns nil, nil.
	missing, err := s.UserByHandle("nobody")
	if err != nil {
		t.Fatalf("UserByHandle miss: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil on miss, got %+v", missing)
	}

	users, err := s.Users()
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
}

func TestPostAndReadMessages(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	boards, err := s.Boards()
	if err != nil {
		t.Fatalf("Boards: %v", err)
	}
	bid := boards[0].ID

	before, err := s.Messages(bid, 0)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}

	id, err := s.PostMessage(&Message{
		BoardID: bid,
		From:    "tester",
		To:      "All",
		Subject: "hello",
		Body:    "world",
		Posted:  time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if id == 0 {
		t.Fatal("PostMessage returned zero id")
	}

	after, err := s.Messages(bid, 0)
	if err != nil {
		t.Fatalf("Messages after: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("expected %d messages, got %d", len(before)+1, len(after))
	}
	// Newest-first: our post should be first.
	if after[0].Subject != "hello" || after[0].ID != id {
		t.Fatalf("expected newest message first, got %+v", after[0])
	}

	// limit honored.
	one, err := s.Messages(bid, 1)
	if err != nil {
		t.Fatalf("Messages limit: %v", err)
	}
	if len(one) != 1 {
		t.Fatalf("expected 1 message with limit, got %d", len(one))
	}

	recent, err := s.RecentMessages(5)
	if err != nil {
		t.Fatalf("RecentMessages: %v", err)
	}
	if len(recent) == 0 {
		t.Fatal("RecentMessages returned none")
	}
	if recent[0].ID != id {
		t.Fatalf("expected newest message across boards first, got %+v", recent[0])
	}
}

func TestOnelinerRoundTrip(t *testing.T) {
	s := newTestStore(t)

	o := &Oneliner{Author: "tester", Text: "ratio is law"}
	if err := s.AddOneliner(o); err != nil {
		t.Fatalf("AddOneliner: %v", err)
	}
	if o.ID == 0 {
		t.Fatal("AddOneliner did not set ID")
	}

	liners, err := s.Oneliners(10)
	if err != nil {
		t.Fatalf("Oneliners: %v", err)
	}
	if len(liners) != 1 {
		t.Fatalf("expected 1 oneliner, got %d", len(liners))
	}
	if liners[0].Text != "ratio is law" || liners[0].Author != "tester" {
		t.Fatalf("unexpected oneliner: %+v", liners[0])
	}
}
