package web

import (
	"net/http"
	"strings"
	"testing"

	"vendetta-x/server/internal/store"
)

// TestProfileBadges proves earned badges render on the web profile and that the
// staff mark is gated on privilege.
func TestProfileBadges(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(); err != nil {
		t.Fatalf("store.Seed: %v", err)
	}

	// Give the ordinary "nut" account enough activity to earn a couple of
	// badges (120 posts -> Co-Conspirator, 12 uploads -> Supplier).
	if _, err := st.DB().Exec(`UPDATE users SET posts = 120, uploads = 12 WHERE handle = 'nut'`); err != nil {
		t.Fatalf("seed counters: %v", err)
	}

	h := New(st, func() []string { return nil }, Config{})

	rec := do(h, http.MethodGet, "/users/nut", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /users/nut: status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"ph-badges", "Co-Conspirator", "Supplier"} {
		if !strings.Contains(body, want) {
			t.Errorf("nut profile missing %q", want)
		}
	}
	// nut is SL 10 -- not staff.
	if strings.Contains(body, ">Staff</span>") {
		t.Error("an ordinary user's profile must not carry the Staff badge")
	}

	// The privileged sysop account does.
	rec = do(h, http.MethodGet, "/users/sysop", "", nil)
	if !strings.Contains(rec.Body.String(), "Staff") {
		t.Error("sysop profile should carry the Staff badge")
	}
}
