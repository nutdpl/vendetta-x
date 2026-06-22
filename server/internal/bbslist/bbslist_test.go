package bbslist

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestStore opens an in-memory SQLite db and builds a migrated, seeded Store.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := New(db)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return st
}

func TestSeedPresent(t *testing.T) {
	st := newTestStore(t)
	list, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("expected seeded boards, got none")
	}
}

func TestSeedIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := New(db)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	first, _ := st.List()
	// Re-seed: should be a no-op since the list is non-empty.
	if err := st.seed(); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	second, _ := st.List()
	if len(first) != len(second) {
		t.Fatalf("seed not idempotent: %d -> %d", len(first), len(second))
	}
}

func TestAddListSorted(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&Entry{
		Name:     "AAA First Board",
		Address:  "aaa.bbs",
		Software: "Synchronet",
		Sysop:    "Tester",
		Desc:     "should sort to the top",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if id == 0 {
		t.Fatalf("add returned zero id")
	}
	list, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Verify our entry is present and the list is sorted by name (NOCASE).
	found := false
	for _, e := range list {
		if e.ID == id {
			found = true
			if e.Name != "AAA First Board" {
				t.Fatalf("name = %q", e.Name)
			}
			if e.Added.IsZero() {
				t.Fatalf("Add did not stamp Added")
			}
		}
	}
	if !found {
		t.Fatalf("added entry not in list")
	}
	for i := 1; i < len(list); i++ {
		if lc(list[i-1].Name) > lc(list[i].Name) {
			t.Fatalf("not sorted: %q before %q", list[i-1].Name, list[i].Name)
		}
	}
	// "AAA First Board" should sort to the front.
	if list[0].ID != id {
		t.Fatalf("expected new entry first, got %q", list[0].Name)
	}
}

func TestGet(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&Entry{Name: "Get Me", Address: "get.bbs", Software: "Mystic", Sysop: "Op"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	got, err := st.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatalf("get returned nil for existing id")
	}
	if got.Name != "Get Me" || got.Address != "get.bbs" || got.Software != "Mystic" || got.Sysop != "Op" {
		t.Fatalf("unexpected entry: %+v", got)
	}
}

func TestGetMissing(t *testing.T) {
	st := newTestStore(t)
	got, err := st.Get(999999)
	if err != nil {
		t.Fatalf("get missing err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing id, got %+v", got)
	}
}

func TestUpdate(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&Entry{Name: "Before", Address: "old.bbs", Software: "Mystic", Sysop: "Op", Desc: "old desc"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := st.Update(&Entry{
		ID:       id,
		Name:     "After",
		Address:  "new.bbs",
		Software: "Synchronet",
		Sysop:    "NewOp",
		Desc:     "new desc",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatalf("get returned nil after update")
	}
	if got.Name != "After" || got.Address != "new.bbs" || got.Software != "Synchronet" || got.Sysop != "NewOp" || got.Desc != "new desc" {
		t.Fatalf("update did not persist new values: %+v", got)
	}
}

func TestDelete(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&Entry{Name: "Doomed", Address: "bye.bbs"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := st.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, err := st.Get(id)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
}

// lc lower-cases an ASCII string for the sort assertion.
func lc(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
