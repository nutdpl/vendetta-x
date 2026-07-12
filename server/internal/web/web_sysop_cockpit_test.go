package web

import (
	"net/http"
	"strings"
	"testing"
)

// adminCookies enrolls and logs in the seeded sysop from the console (loopback),
// returning the session cookies for admin-gated requests.
func adminCookies(t *testing.T, h http.Handler) []*http.Cookie {
	t.Helper()
	rec := doFrom(h, http.MethodPost, "/login", "handle=sysop&password=secretpw", nil, "127.0.0.1:1234")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("admin console login: status = %d, want 303", rec.Code)
	}
	return rec.Result().Cookies()
}

func TestSysopStatsPage(t *testing.T) {
	h := newTestServer(t)
	cookies := adminCookies(t, h)

	rec := do(h, http.MethodGet, "/sysop/stats", "", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sysop/stats: status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"the board at a glance", "messages", "busiest bases", "spark"} {
		if !strings.Contains(body, want) {
			t.Errorf("/sysop/stats missing %q", want)
		}
	}

	// A non-admin can't reach it.
	rec = do(h, http.MethodGet, "/sysop/stats", "", nil)
	if rec.Code == http.StatusOK {
		t.Error("anonymous should not reach /sysop/stats")
	}
}

func TestSysopAuditRecordsMutations(t *testing.T) {
	h := newTestServer(t)
	cookies := adminCookies(t, h)

	// A state-changing admin action should land in the audit trail.
	rec := do(h, http.MethodPost, "/sysop/users/2/level", "sl=20&dsl=20", cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("set level: status = %d, want 303", rec.Code)
	}

	rec = do(h, http.MethodGet, "/sysop/audit", "", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sysop/audit: status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/sysop/users/2/level") {
		t.Errorf("audit trail missing the level-change action:\n%s", body)
	}
	if !strings.Contains(body, "sysop") {
		t.Error("audit trail missing the acting sysop")
	}

	// Actor filter that matches nobody shows the empty notice.
	rec = do(h, http.MethodGet, "/sysop/audit?actor=zzznobody", "", cookies)
	if !strings.Contains(rec.Body.String(), "No actions by an actor") {
		t.Error("expected empty-filter notice for an unknown actor")
	}
}
