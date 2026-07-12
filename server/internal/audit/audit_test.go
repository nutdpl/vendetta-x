package audit

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
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

func TestRecordAndRecent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Record("nut", "POST", "/sysop/boards", "10.0.0.1"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := s.Record("phantom", "DELETE", "/sysop/users/3", "10.0.0.2"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	all, err := s.Recent(10, "")
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("Recent all: got %d, want 2", len(all))
	}
	// Newest first.
	if all[0].Actor != "phantom" || all[0].Method != "DELETE" {
		t.Errorf("Recent order wrong: got %+v", all[0])
	}

	// Actor filter (case-insensitive substring).
	got, err := s.Recent(10, "NUT")
	if err != nil {
		t.Fatalf("Recent filtered: %v", err)
	}
	if len(got) != 1 || got[0].Actor != "nut" {
		t.Fatalf("actor filter: got %d %v, want 1 nut", len(got), got)
	}

	if n, _ := s.Count(); n != 2 {
		t.Fatalf("Count = %d, want 2", n)
	}
}

func TestNewIdempotent(t *testing.T) {
	s := newTestStore(t)
	if _, err := New(s.db); err != nil {
		t.Fatalf("second New should be a no-op: %v", err)
	}
}
