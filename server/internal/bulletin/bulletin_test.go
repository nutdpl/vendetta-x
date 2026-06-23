package bulletin

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB opens a fresh in-memory SQLite database.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newTestStore opens an in-memory db and builds a migrated, seeded Store.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(newTestDB(t))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return st
}

func TestSeedPresent(t *testing.T) {
	st := newTestStore(t)
	all, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected exactly one seeded bulletin, got %d", len(all))
	}
	if all[0].Title != "Welcome to Vendetta/X" || all[0].Author != "SysOp" {
		t.Fatalf("unexpected seed bulletin: %+v", all[0])
	}
	if all[0].Body == "" {
		t.Fatalf("seed bulletin should include a body")
	}
}

func TestSeedIdempotent(t *testing.T) {
	db := newTestDB(t)
	if _, err := New(db); err != nil {
		t.Fatalf("new (1): %v", err)
	}
	st, err := New(db)
	if err != nil {
		t.Fatalf("new (2): %v", err)
	}
	all, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected still one bulletin after second New, got %d", len(all))
	}
}

func TestAddListNewestFirst(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Add(&Bulletin{Title: "Older", Body: "b1", Author: "a"}); err != nil {
		t.Fatalf("add older: %v", err)
	}
	if _, err := st.Add(&Bulletin{Title: "Newer", Body: "b2", Author: "a"}); err != nil {
		t.Fatalf("add newer: %v", err)
	}
	all, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Seed + two added.
	if len(all) != 3 {
		t.Fatalf("expected 3 bulletins, got %d", len(all))
	}
	// Newest first: posted timestamps non-increasing.
	for i := 1; i < len(all); i++ {
		if all[i-1].Posted.Before(all[i].Posted) {
			t.Fatalf("not newest-first at %d", i)
		}
	}
	// Most recently added (with same-or-later timestamp) wins by id DESC.
	if all[0].Title != "Newer" {
		t.Fatalf("expected newest 'Newer' first, got %q", all[0].Title)
	}
	// Body is included by List.
	if all[0].Body == "" {
		t.Fatalf("List should include body")
	}
	// Add stamped Posted.
	if all[0].Posted.IsZero() {
		t.Fatalf("Add did not stamp Posted")
	}
}

func TestGet(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&Bulletin{Title: "Findable", Body: "line one\nline two", Author: "tester"})
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
	if got.Title != "Findable" || got.Body != "line one\nline two" || got.Author != "tester" {
		t.Fatalf("unexpected bulletin: %+v", got)
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
	id, err := st.Add(&Bulletin{Title: "Original", Body: "original body", Author: "orig"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	before, err := st.Get(id)
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	if err := st.Update(&Bulletin{ID: id, Title: "Edited", Body: "edited body", Author: "editor"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatalf("updated bulletin not found")
	}
	if got.Title != "Edited" || got.Body != "edited body" || got.Author != "editor" {
		t.Fatalf("update did not persist: %+v", got)
	}
	// Posted is left untouched by Update.
	if !got.Posted.Equal(before.Posted) {
		t.Fatalf("Update changed Posted: was %v, now %v", before.Posted, got.Posted)
	}
}

func TestDelete(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&Bulletin{Title: "Doomed", Body: "doomed", Author: "tester"})
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
