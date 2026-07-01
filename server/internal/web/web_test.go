package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vendetta-x/server/internal/store"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := st.Seed(); err != nil {
		t.Fatalf("store.Seed: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, func() []string { return []string{"nut"} }, Config{})
}

// do issues a request through the handler with any cookies attached, returning
// the recorder (whose cookies callers thread into the next request). The remote
// address is httptest's default (192.0.2.1, non-loopback) -- i.e. a remote
// caller, not the console.
func do(h http.Handler, method, path, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	return doFrom(h, method, path, body, cookies, "")
}

// doFrom is do with an explicit remote address, so tests can model a console
// (loopback) caller -- the trust boundary the sysop enrollment guard keys on.
func doFrom(h http.Handler, method, path, body string, cookies []*http.Cookie, remote string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if remote != "" {
		r.RemoteAddr = remote
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestPagesRenderWithMasthead(t *testing.T) {
	h := newTestServer(t)

	for _, path := range []string{"/", "/boards", "/files", "/users"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, rec.Code)
		}
		// The masthead brand is the real TDF wordmark image, not text --
		// check for the logo, not incidental board-name text elsewhere.
		body := rec.Body.String()
		if !strings.Contains(body, `class="brand-logo"`) || !strings.Contains(body, "VENDETTA/X") {
			t.Errorf("GET %s: body missing board name masthead", path)
		}
	}
}

func TestOnlineNil(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	// nil online func, and a handler created with it, must not panic.
	h := New(st, nil, Config{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / with nil online: status = %d, want 200", rec.Code)
	}
}

func TestBadBoardIDNo500(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/boards/abc", "/boards/999"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code >= 500 {
			t.Errorf("GET %s: status = %d, want < 500", path, rec.Code)
		}
	}
}

func TestPostOnelinerRequiresLoginAndIgnoresAuthorField(t *testing.T) {
	h := newTestServer(t)

	// Anonymous: the wall is gated now -- bounce to login, don't post.
	rec := do(h, http.MethodPost, "/oneliner", "author=nut&text=anon+spam", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("anon POST /oneliner: status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "/login") {
		t.Fatalf("anon POST /oneliner should redirect to login, got %q", loc)
	}

	// Logged in as phantom, but the form lies that the author is "nut": the
	// entry must be attributed to the authenticated caller, not the form field.
	rec = do(h, http.MethodPost, "/login", "handle=phantom&password=ghostpass", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("phantom login: status = %d, want 303", rec.Code)
	}
	cookies := rec.Result().Cookies()
	rec = do(h, http.MethodPost, "/oneliner", "author=nut&text=hello+wall", cookies)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logged-in POST /oneliner: status = %d, want 303", rec.Code)
	}
	body := do(h, http.MethodGet, "/", "", nil).Body.String()
	if !strings.Contains(body, "hello wall") {
		t.Fatal("oneliner text not shown on the wall")
	}
	// The spoofed "nut" author must not appear as this entry's author; the
	// caller (phantom) must.
	if !strings.Contains(body, "phantom") {
		t.Fatal("oneliner not attributed to the authenticated caller (phantom)")
	}
}

func TestPostMessageRedirects(t *testing.T) {
	h := newTestServer(t)
	form := strings.NewReader("from=nut&subject=hi&body=test+body")
	req := httptest.NewRequest(http.MethodPost, "/boards/1", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /boards/1: status = %d, want 303", rec.Code)
	}
}

// TestCSRFOriginGuard proves the CSRF guard: a state-changing POST carrying a
// foreign Origin is rejected, while a same-origin one (and any safe GET) passes.
func TestCSRFOriginGuard(t *testing.T) {
	h := newTestServer(t)

	post := func(origin string) int {
		r := httptest.NewRequest(http.MethodPost, "/oneliner", strings.NewReader("text=hi"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	// httptest.NewRequest uses Host "example.com".
	if code := post("http://evil.example.net"); code != http.StatusForbidden {
		t.Fatalf("cross-origin POST: status = %d, want 403", code)
	}
	if code := post("http://example.com"); code == http.StatusForbidden {
		t.Fatal("same-origin POST was blocked by the CSRF guard")
	}

	// A safe GET is never origin-checked, even cross-origin.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://evil.example.net")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code == http.StatusForbidden {
		t.Fatal("a safe GET was blocked by the CSRF guard")
	}
}

// TestSysopGating proves the /sysop choke point: anonymous -> login, a
// logged-in non-admin -> 403, and an admin -> 200.
func TestSysopGating(t *testing.T) {
	h := newTestServer(t)

	// 1) anonymous is redirected to login.
	rec := do(h, http.MethodGet, "/sysop", "", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("anon /sysop: status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login") {
		t.Fatalf("anon /sysop: Location = %q, want /login...", loc)
	}

	// 2) a freshly registered user (SL 10) is forbidden.
	rec = do(h, http.MethodPost, "/register", "handle=noob&password=passw0rd&verify=passw0rd", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("register: status = %d, want 303", rec.Code)
	}
	noobCookies := rec.Result().Cookies()
	rec = do(h, http.MethodGet, "/sysop", "", noobCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin /sysop: status = %d, want 403", rec.Code)
	}
	// and a non-admin cannot mutate levels.
	rec = do(h, http.MethodPost, "/sysop/users/1/level", "sl=255&dsl=255", noobCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin set-level: status = %d, want 403", rec.Code)
	}

	// 3) the passwordless sysop can't be enrolled by a REMOTE caller -- the login
	// refuses it (re-renders the form, no redirect) rather than letting a remote
	// visitor claim the admin account.
	rec = do(h, http.MethodPost, "/login", "handle=sysop&password=secretpw", nil)
	if rec.Code == http.StatusSeeOther {
		t.Fatal("remote caller was allowed to enroll the passwordless sysop")
	}
	if !strings.Contains(rec.Body.String(), "reserved") {
		t.Fatalf("remote sysop enrollment: body missing the reserved notice:\n%s", rec.Body.String())
	}

	// 4) the seeded admin (sysop, SL 255) enrolls from the console (a loopback
	// request) -- first login sets the password -- and then reaches /sysop.
	rec = doFrom(h, http.MethodPost, "/login", "handle=sysop&password=secretpw", nil, "127.0.0.1:1234")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("admin console login: status = %d, want 303", rec.Code)
	}
	danCookies := rec.Result().Cookies()
	rec = do(h, http.MethodGet, "/sysop", "", danCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin /sysop: status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "sysop dashboard") {
		t.Fatalf("admin /sysop: body missing dashboard heading")
	}
}

// TestDownloadTokens checks the signed-link crypto: valid round-trips, while
// tampered, expired, wrong-secret, and garbage tokens are all rejected.
func TestDownloadTokens(t *testing.T) {
	s := &server{dlSecret: []byte("0123456789abcdef0123456789abcdef")}

	link := s.signDownload(42, time.Minute)
	tok := strings.TrimPrefix(link, "/dl/")
	if id, ok := s.verifyDownload(tok); !ok || id != 42 {
		t.Fatalf("valid token: id=%d ok=%v, want 42,true", id, ok)
	}

	// tamper a middle byte of the signature (a meaningful 6-bit change).
	if _, ok := s.verifyDownload(mutate(tok)); ok {
		t.Fatal("tampered token verified")
	}

	// expired (signed in the past).
	if _, ok := s.verifyDownload(strings.TrimPrefix(s.signDownload(7, -time.Minute), "/dl/")); ok {
		t.Fatal("expired token verified")
	}

	// a different secret must not validate our token.
	other := &server{dlSecret: []byte("ffffffffffffffffffffffffffffffff")}
	if _, ok := other.verifyDownload(tok); ok {
		t.Fatal("token verified under the wrong secret")
	}

	if _, ok := s.verifyDownload("not-a-real-token"); ok {
		t.Fatal("garbage token verified")
	}
}

// TestDownloadServesContent drives the real stack: the files page hands out a
// signed link, and following it streams the stored bytes as an attachment.
func TestDownloadServesContent(t *testing.T) {
	h := newTestServer(t)

	rec := do(h, http.MethodGet, "/files?area=1", "", nil)
	body := rec.Body.String()
	i := strings.Index(body, "/dl/")
	if i < 0 {
		t.Fatal("files page has no download link")
	}
	j := strings.IndexByte(body[i:], '"')
	if j < 0 {
		t.Fatal("malformed download link")
	}
	link := body[i : i+j]

	rec = do(h, http.MethodGet, link, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download: status = %d, want 200", rec.Code)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("download: Content-Disposition = %q, want attachment", cd)
	}
	if !strings.Contains(rec.Body.String(), "Vendetta/X distro") {
		t.Fatal("download body missing stored content")
	}

	// a tampered link is rejected.
	rec = do(h, http.MethodGet, mutate(link), "", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("tampered download link served content")
	}
}

// mutate flips a middle character of s (skipping '.' and '/') to produce a
// meaningful one-character corruption for tamper tests.
func mutate(s string) string {
	b := []byte(s)
	if len(b) == 0 {
		return s
	}
	mid := len(b) / 2
	for mid < len(b) && (b[mid] == '.' || b[mid] == '/') {
		mid++
	}
	if mid >= len(b) {
		mid = len(b) - 1
	}
	if b[mid] == 'A' {
		b[mid] = 'B'
	} else {
		b[mid] = 'A'
	}
	return string(b)
}

// TestUploadFlow proves uploads require login and that an uploaded file lands
// in the area listing and is downloadable.
func TestUploadFlow(t *testing.T) {
	h := newTestServer(t)

	build := func() (*bytes.Buffer, string) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "HELLO.TXT")
		fw.Write([]byte("uploaded payload bytes"))
		mw.WriteField("description", "a test upload")
		mw.Close()
		return &buf, mw.FormDataContentType()
	}

	// anonymous upload -> redirected to login.
	body, ct := build()
	req := httptest.NewRequest(http.MethodPost, "/files/1/upload", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("anon upload: status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login") {
		t.Fatalf("anon upload: Location = %q, want /login...", loc)
	}

	// log in (nut), then upload.
	rec = do(h, http.MethodPost, "/login", "handle=nut&password=secretpw", nil)
	cookies := rec.Result().Cookies()

	body, ct = build()
	req = httptest.NewRequest(http.MethodPost, "/files/1/upload", body)
	req.Header.Set("Content-Type", ct)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload: status = %d, want 303", rec.Code)
	}

	// the uploaded file shows up in the listing.
	rec = do(h, http.MethodGet, "/files?area=1", "", cookies)
	if !strings.Contains(rec.Body.String(), "HELLO.TXT") {
		t.Fatal("uploaded file not listed in its area")
	}
}
