package gfiles

import (
	"database/sql"
	"sort"
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
	all, err := st.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) == 0 {
		t.Fatalf("expected seeded docs, got none")
	}
}

func TestCategoriesDistinctSorted(t *testing.T) {
	st := newTestStore(t)
	cats, err := st.Categories()
	if err != nil {
		t.Fatalf("categories: %v", err)
	}
	if len(cats) == 0 {
		t.Fatalf("expected categories, got none")
	}
	// Distinct.
	seen := map[string]bool{}
	for _, c := range cats {
		if seen[c] {
			t.Fatalf("duplicate category %q", c)
		}
		seen[c] = true
	}
	// Sorted.
	if !sort.StringsAreSorted(cats) {
		t.Fatalf("categories not sorted: %v", cats)
	}
}

func TestListAllNoBody(t *testing.T) {
	st := newTestStore(t)
	all, err := st.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, g := range all {
		if g.Body != "" {
			t.Fatalf("List should omit body, got %q for %q", g.Body, g.Title)
		}
	}
	// Newest first: added timestamps should be non-increasing.
	for i := 1; i < len(all); i++ {
		if all[i-1].Added.Before(all[i].Added) {
			t.Fatalf("not newest-first")
		}
	}
}

func TestListFiltersByCategory(t *testing.T) {
	st := newTestStore(t)
	cats, err := st.Categories()
	if err != nil {
		t.Fatalf("categories: %v", err)
	}
	cat := cats[0]
	got, err := st.List(cat)
	if err != nil {
		t.Fatalf("list cat: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected docs in category %q", cat)
	}
	for _, g := range got {
		if g.Category != cat {
			t.Fatalf("filter leaked: got category %q, want %q", g.Category, cat)
		}
	}
	// Filtered result must be a subset of all.
	all, _ := st.List("")
	if len(got) > len(all) {
		t.Fatalf("filtered (%d) > all (%d)", len(got), len(all))
	}
}

func TestGetIncludesBody(t *testing.T) {
	st := newTestStore(t)
	all, err := st.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	doc, err := st.Get(all[0].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if doc == nil {
		t.Fatalf("get returned nil for existing id")
	}
	if doc.Body == "" {
		t.Fatalf("Get should include body")
	}
}

func TestGetMissing(t *testing.T) {
	st := newTestStore(t)
	doc, err := st.Get(999999)
	if err != nil {
		t.Fatalf("get missing err: %v", err)
	}
	if doc != nil {
		t.Fatalf("expected nil for missing id, got %+v", doc)
	}
}

func TestAdd(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&GFile{
		Category: "Test",
		Title:    "A New Doc",
		Body:     "line one\nline two",
		Author:   "tester",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if id == 0 {
		t.Fatalf("add returned zero id")
	}
	doc, err := st.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if doc == nil {
		t.Fatalf("added doc not found")
	}
	if doc.Title != "A New Doc" || doc.Body != "line one\nline two" || doc.Author != "tester" || doc.Category != "Test" {
		t.Fatalf("unexpected doc: %+v", doc)
	}
	if doc.Added.IsZero() {
		t.Fatalf("Add did not stamp Added")
	}
	// New category should now appear in the distinct, sorted list.
	cats, _ := st.Categories()
	found := false
	for _, c := range cats {
		if c == "Test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("new category not in Categories(): %v", cats)
	}
}

func TestUpdate(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&GFile{
		Category: "Orig",
		Title:    "Original Title",
		Body:     "original body",
		Author:   "orig",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := st.Update(&GFile{
		ID:       id,
		Category: "Edited",
		Title:    "Edited Title",
		Body:     "edited body",
		Author:   "editor",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	doc, err := st.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if doc == nil {
		t.Fatalf("updated doc not found")
	}
	if doc.Category != "Edited" || doc.Title != "Edited Title" || doc.Body != "edited body" || doc.Author != "editor" {
		t.Fatalf("update did not persist: %+v", doc)
	}
}

func TestDelete(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Add(&GFile{
		Category: "Test",
		Title:    "To Be Deleted",
		Body:     "doomed",
		Author:   "tester",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := st.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	doc, err := st.Get(id)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if doc != nil {
		t.Fatalf("expected nil after delete, got %+v", doc)
	}
}
