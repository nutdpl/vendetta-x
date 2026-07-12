package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"vendetta-x/server/internal/store"
)

// TestSearchPage exercises the board-wide search page: the empty form, a
// message hit, a file hit, and -- the security-critical property -- that an
// anonymous viewer never surfaces content from an ACS-restricted base.
func TestSearchPage(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(); err != nil {
		t.Fatalf("store.Seed: %v", err)
	}

	// Plant a uniquely-worded post in the seeded, s100-gated "sysop" base.
	var sysopID int64
	boards, _ := st.Boards()
	for _, b := range boards {
		if b.Tag == "sysop" {
			sysopID = b.ID
		}
	}
	if sysopID == 0 {
		t.Fatal("seed did not create the sysop board")
	}
	if _, err := st.PostMessage(&store.Message{
		BoardID: sysopID, From: "sysop", To: "All",
		Subject: "zebraphantomsecret plans", Body: "for staff eyes only",
		Posted: time.Now(),
	}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	h := New(st, func() []string { return nil }, Config{})

	// Empty form renders.
	rec := do(h, http.MethodGet, "/search", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /search: status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "search the board") {
		t.Error("GET /search: missing the search form heading")
	}

	// A message in an open base is found.
	rec = do(h, http.MethodGet, "/search?q=welcome", "", nil)
	if body := rec.Body.String(); !strings.Contains(body, "Welcome to Vendetta/X") {
		t.Error("search for 'welcome': expected the seeded General post in results")
	}

	// A seeded file is found by filename.
	rec = do(h, http.MethodGet, "/search?q=PKZIP", "", nil)
	if body := rec.Body.String(); !strings.Contains(body, "PKZIP204G.EXE") {
		t.Error("search for 'PKZIP': expected the seeded utility in file results")
	}

	// ACS scoping: an anonymous viewer must NOT surface the restricted post,
	// even searching its exact unique term.
	rec = do(h, http.MethodGet, "/search?q=zebraphantomsecret", "", nil)
	body := rec.Body.String()
	if strings.Contains(body, "zebraphantomsecret plans") {
		t.Error("anonymous search leaked a post from the s100-gated sysop base")
	}
	if !strings.Contains(body, "No messages match") {
		t.Error("expected an empty-message-results notice for the restricted term")
	}
}
