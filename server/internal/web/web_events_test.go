package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vendetta-x/server/internal/store"
)

func TestAtomFeedPublicOnly(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(); err != nil {
		t.Fatalf("store.Seed: %v", err)
	}
	// A post in the s100-gated sysop base must never reach the open web.
	var sysopID int64
	bs, _ := st.Boards()
	for _, b := range bs {
		if b.Tag == "sysop" {
			sysopID = b.ID
		}
	}
	if _, err := st.PostMessage(&store.Message{
		BoardID: sysopID, From: "sysop", Subject: "zebrasecretfeed", Body: "hidden", Posted: time.Now(),
	}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	h := New(st, func() []string { return nil }, Config{BoardName: "Vendetta/X"})

	rec := do(h, http.MethodGet, "/feed.atom", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /feed.atom: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/atom+xml") {
		t.Errorf("feed Content-Type = %q, want application/atom+xml", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<feed") || !strings.Contains(body, "Welcome to Vendetta/X") {
		t.Errorf("feed should list the seeded public post; body:\n%s", body)
	}
	if strings.Contains(body, "zebrasecretfeed") {
		t.Error("Atom feed leaked a post from the gated sysop base")
	}
}

func TestAtomFeedGated(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	st.Seed()
	// Turn the feeds feature off.
	if err := st.SetSetting("feature.feeds", "false"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	h := New(st, func() []string { return nil }, Config{})

	rec := do(h, http.MethodGet, "/feed.atom", "", nil)
	// The feature gate redirects a disabled feature home rather than serving it.
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("disabled feed: status = %d, want 303 redirect", rec.Code)
	}
}

func TestSSEEmitsPresence(t *testing.T) {
	h := newTestServer(t) // online() returns []string{"nut"}

	// A pre-cancelled context lets the handler emit its initial snapshot and
	// return immediately instead of blocking on the stream loop.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("SSE Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: presence") || !strings.Contains(body, "nut") {
		t.Errorf("SSE should emit an initial presence event with the online node; got:\n%s", body)
	}
}
