package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestSettingsPrefsPersist(t *testing.T) {
	h := newTestServer(t)
	rec := do(h, http.MethodPost, "/register", "handle=prefuser&password=passw0rd&verify=passw0rd", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("register: status = %d, want 303", rec.Code)
	}
	cookies := rec.Result().Cookies()

	// Tick both preferences.
	do(h, http.MethodPost, "/settings", "real_name=&email=&location=&tagline=&birthday=&expert=1&clock12=1", cookies)
	body := do(h, http.MethodGet, "/settings", "", cookies).Body.String()
	if !strings.Contains(body, `name="expert" value="1" checked`) {
		t.Error("expert checkbox should be checked after saving it on")
	}
	if !strings.Contains(body, `name="clock12" value="1" checked`) {
		t.Error("clock12 checkbox should be checked after saving it on")
	}

	// Unticking (fields absent) turns them back off.
	do(h, http.MethodPost, "/settings", "real_name=&email=&location=&tagline=&birthday=", cookies)
	body = do(h, http.MethodGet, "/settings", "", cookies).Body.String()
	if strings.Contains(body, `name="expert" value="1" checked`) {
		t.Error("expert checkbox should be unchecked after saving it off")
	}
}
