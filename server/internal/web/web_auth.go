package web

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"vendetta-x/server/internal/auth"
	"vendetta-x/server/internal/store"
)

const (
	sessionCookie = "vx_session"
	sessionTTL    = 24 * time.Hour
)

// session is one logged-in web session.
type session struct {
	handle  string
	created time.Time
}

// sessionManager is the in-memory web session store: random opaque tokens map
// to a handle. Tokens are high-entropy and looked up server-side, so the cookie
// carries no signed payload.
type sessionManager struct {
	mu sync.Mutex
	m  map[string]*session
}

func newSessionManager() *sessionManager {
	return &sessionManager{m: make(map[string]*session)}
}

func (sm *sessionManager) create(handle string) string {
	tok := randToken()
	sm.mu.Lock()
	sm.m[tok] = &session{handle: handle, created: time.Now()}
	sm.mu.Unlock()
	return tok
}

func (sm *sessionManager) lookup(tok string) (string, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.m[tok]
	if !ok {
		return "", false
	}
	if time.Since(s.created) > sessionTTL {
		delete(sm.m, tok)
		return "", false
	}
	return s.handle, true
}

func (sm *sessionManager) destroy(tok string) {
	sm.mu.Lock()
	delete(sm.m, tok)
	sm.mu.Unlock()
}

func randToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is catastrophic; fall back to a timestamp so we
		// never hand out an empty (guessable) token.
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

// currentUser resolves the logged-in user from the session cookie, or nil.
func (s *server) currentUser(r *http.Request) *store.User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	handle, ok := s.sessions.lookup(c.Value)
	if !ok {
		return nil
	}
	u, err := s.st.UserByHandle(handle)
	if err != nil || u == nil {
		return nil
	}
	return u
}

func (s *server) setSessionCookie(w http.ResponseWriter, tok string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (s *server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// safeNext returns a same-site redirect target, defaulting to "/". It only
// allows local absolute paths (leading "/", but not "//") to avoid open
// redirects.
func safeNext(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	return "/"
}

// ---- handlers ----

func (s *server) loginForm(w http.ResponseWriter, r *http.Request) {
	if s.currentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderAuth(w, r, "login", "", r.URL.Query().Get("next"), "")
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	handle := strings.TrimSpace(r.FormValue("handle"))
	password := r.FormValue("password")
	next := safeNext(r.FormValue("next"))

	// Brute-force throttle: too many recent failures from this IP and we stop
	// checking credentials until the window clears.
	ip := s.clientIP(r)
	if s.loginThrottle.Blocked(ip) {
		s.renderAuth(w, r, "login", "Too many login attempts. Wait a few minutes and try again.", next, handle)
		return
	}

	if handle == "" || password == "" {
		s.renderAuth(w, r, "login", "Handle and password are required.", next, handle)
		return
	}

	u, err := s.st.UserByHandle(handle)
	if err != nil {
		log.Printf("web: login UserByHandle: %v", err)
		s.renderAuth(w, r, "login", "Something went wrong. Try again.", next, handle)
		return
	}
	if u == nil {
		s.loginThrottle.Fail(ip) // count unknown-user attempts too
		s.renderAuth(w, r, "login", "No such user.", next, handle)
		return
	}

	// An account with no password yet (seeded/legacy) sets one on first login,
	// matching the telnet face. A privileged account (the seeded sysop) may only
	// be enrolled from the console -- a loopback request -- so a remote web caller
	// can't claim a passwordless admin and take over the board.
	if u.Password == "" {
		if u.Privileged() && !isLoopbackHost(ip) {
			s.loginThrottle.Fail(ip)
			s.renderAuth(w, r, "login", "That account is reserved. Set its password from the console (a local login).", next, handle)
			return
		}
		hash, herr := auth.Hash(password)
		if herr != nil {
			s.renderAuth(w, r, "login", "Could not set password.", next, handle)
			return
		}
		if err := s.st.SetPassword(u.ID, hash); err != nil {
			s.renderAuth(w, r, "login", "Could not save password.", next, handle)
			return
		}
	} else if !auth.Verify(u.Password, password) {
		s.loginThrottle.Fail(ip)
		s.renderAuth(w, r, "login", "Bad password.", next, handle)
		return
	}

	// NB: success does NOT reset the throttle (see throttle.Reset) so an
	// attacker can't interleave a login to clear the limiter.
	if err := s.st.RecordLogin(u.ID); err != nil {
		log.Printf("web: RecordLogin: %v", err)
	}
	s.setSessionCookie(w, s.sessions.create(u.Handle))
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *server) registerForm(w http.ResponseWriter, r *http.Request) {
	if s.currentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderAuth(w, r, "register", "", r.URL.Query().Get("next"), "")
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	handle := strings.TrimSpace(r.FormValue("handle"))
	password := r.FormValue("password")
	verify := r.FormValue("verify")
	realName := strings.TrimSpace(r.FormValue("real_name"))
	location := strings.TrimSpace(r.FormValue("location"))
	next := safeNext(r.FormValue("next"))

	fail := func(msg string) { s.renderAuth(w, r, "register", msg, next, handle) }

	switch {
	case handle == "" || password == "":
		fail("Handle and password are required.")
		return
	case password != verify:
		fail("Passwords do not match.")
		return
	}
	if err := store.ValidateHandle(handle); err != nil {
		fail(err.Error())
		return
	}

	existing, err := s.st.UserByHandle(handle)
	if err != nil {
		log.Printf("web: register UserByHandle: %v", err)
		fail("Something went wrong. Try again.")
		return
	}
	if existing != nil {
		fail("That handle is taken.")
		return
	}

	hash, err := auth.Hash(password)
	if err != nil {
		fail("Could not hash password.")
		return
	}
	now := time.Now()
	u := &store.User{
		Handle:    handle,
		RealName:  realName,
		Location:  location,
		Group:     "Users",
		Tagline:   "New blood.",
		SL:        10,
		DSL:       10,
		Password:  hash,
		FirstCall: now,
		LastCall:  now,
	}
	if _, err := s.st.AddUser(u); err != nil {
		log.Printf("web: register AddUser: %v", err)
		fail("Could not create account.")
		return
	}
	s.setSessionCookie(w, s.sessions.create(u.Handle))
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.destroy(c.Value)
	}
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// renderAuth renders the login/register page (both share the "auth" page) with
// an optional error message and preserved form context.
func (s *server) renderAuth(w http.ResponseWriter, r *http.Request, mode, errMsg, next, handle string) {
	title := "login"
	if mode == "register" {
		title = "register"
	}
	s.render(w, "auth", struct {
		pageData
		Mode   string
		Error  string
		Next   string
		Handle string
	}{s.base(r, title, ""), mode, errMsg, safeNext(next), handle})
}
