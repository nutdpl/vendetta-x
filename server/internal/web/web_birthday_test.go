package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestBirthdaySettingsAndProfile drives the birthday through the web settings
// form and asserts it shows on the profile, and that a bad date is rejected.
func TestBirthdaySettingsAndProfile(t *testing.T) {
	h := newTestServer(t)

	// Register a fresh caller and grab the session.
	rec := do(h, http.MethodPost, "/register", "handle=bdayuser&password=passw0rd&verify=passw0rd", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("register: status = %d, want 303", rec.Code)
	}
	cookies := rec.Result().Cookies()

	// Save a valid birthday via the profile form.
	rec = do(h, http.MethodPost, "/settings", "real_name=&email=&location=&tagline=&birthday=7-12", cookies)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "ok=profile") {
		t.Fatalf("valid birthday save: Location = %q, want ok=profile", loc)
	}

	// It normalizes to 07-12 and shows on the profile.
	rec = do(h, http.MethodGet, "/users/bdayuser", "", cookies)
	body := rec.Body.String()
	if !strings.Contains(body, "birthday") || !strings.Contains(body, "07-12") {
		t.Errorf("profile should show the normalized birthday 07-12")
	}

	// A bad date is refused (no half-save; the form reports the error).
	rec = do(h, http.MethodPost, "/settings", "real_name=&email=&location=&tagline=&birthday=99-99", cookies)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("invalid birthday: Location = %q, want an err redirect", loc)
	}
	// The stored value is unchanged.
	rec = do(h, http.MethodGet, "/users/bdayuser", "", cookies)
	if !strings.Contains(rec.Body.String(), "07-12") {
		t.Error("a rejected birthday update should leave 07-12 in place")
	}
}
