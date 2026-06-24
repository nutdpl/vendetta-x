// Package web is Vendetta/X's modern web face: a small, dependency-free
// net/http + html/template server that renders the BBS over HTTP. It reads and
// writes through the shared store and is meant to look like the elite/ACiD
// scene aesthetic the board carries everywhere else.
//
// Structure (so features can be built independently):
//
//	web.go            the shell: server type, router, template loader, css,
//	                  shared helpers + template funcs. THIS FILE owns the contract.
//	web_home.go       home / dashboard
//	web_boards.go     message boards (index, thread, post)
//	web_files.go      file areas + downloads
//	web_users.go      user directory + profiles
//	web_styleguide.go the design-system reference page
//
//	templates/base.html        the layout shell (masthead, nav, footer)
//	templates/partials/*.html  shared + feature components (namespaced defines)
//	templates/pages/*.html     one file per page; each defines {{"content"}}
//	static/*.css               concatenated in filename order (00-base.css first)
package web

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/bbslist"
	"vendetta-x/server/internal/bulletin"
	"vendetta-x/server/internal/door"
	"vendetta-x/server/internal/gfiles"
	"vendetta-x/server/internal/mail"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/throttle"
	"vendetta-x/server/internal/voting"
)

//go:embed templates static
var assets embed.FS

// Config carries server facts the sysop panel displays (addresses, version,
// start time). It is informational only.
type Config struct {
	BoardName string
	Version   string
	Telnet    string
	SSH       string
	HTTP      string
	DB        string
	Started   time.Time
	// SecureCookies marks session cookies Secure (HTTPS-only). Set when the web
	// face is served over TLS, directly or behind a terminating proxy.
	SecureCookies bool
	// TrustProxy enables honoring the X-Forwarded-For header for the client IP
	// (login throttling). Leave false unless the board sits behind a trusted
	// reverse proxy that sets XFF -- otherwise a client can spoof it and dodge
	// the throttle. When false, the real TCP peer (RemoteAddr) is always used.
	TrustProxy bool
	// LoginThrottle, when set, is shared with the telnet/ssh faces so brute-force
	// attempts are counted across all three. Nil means the web face uses its own.
	LoginThrottle *throttle.Throttle
}

// New builds the HTTP handler for the web face. st is the shared data store;
// online returns the handles currently connected over the telnet/ssh faces (it
// may be nil, and may return nil -- both are handled); cfg is server info shown
// in the sysop panel.
func New(st *store.Store, online func() []string, cfg Config) http.Handler {
	if online == nil {
		online = func() []string { return nil }
	}
	if cfg.Started.IsZero() {
		cfg.Started = time.Now()
	}

	s := &server{st: st, online: online, sessions: newSessionManager(), cfg: cfg}
	s.dlSecret = newDownloadSecret()
	// Share the telnet/ssh limiter when given, so an IP's failures count across
	// every face; otherwise stand up a local one.
	if cfg.LoginThrottle != nil {
		s.loginThrottle = cfg.LoginThrottle
	} else {
		s.loginThrottle = throttle.New(8, 10*time.Minute) // 8 failed logins / 10 min / IP
	}
	// feature data layers over the shared DB (tables already created by main;
	// New is idempotent so constructing here is safe either way).
	s.mail, _ = mail.New(st.DB())
	s.voting, _ = voting.New(st.DB())
	s.bbslist, _ = bbslist.New(st.DB())
	s.gfiles, _ = gfiles.New(st.DB())
	s.doorStore, _ = door.New(st.DB())
	s.bulletins, _ = bulletin.New(st.DB())
	s.tmpl = parseTemplates()

	mux := http.NewServeMux()

	// static assets (css concatenated from static/*.css in name order).
	mux.HandleFunc("GET /static/style.css", s.css)

	// pages. Handlers live in the per-feature web_*.go files.
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /boards", s.boards)
	mux.HandleFunc("GET /boards/{id}", s.board)
	mux.HandleFunc("POST /boards/{id}", s.postMessage)
	mux.HandleFunc("GET /files", s.files)
	mux.HandleFunc("POST /files/{id}/upload", s.uploadFile)
	mux.HandleFunc("GET /users", s.users)
	mux.HandleFunc("GET /users/{handle}", s.userProfile)
	mux.HandleFunc("GET /styleguide", s.styleguide)
	mux.HandleFunc("POST /oneliner", s.postOneliner)
	mux.HandleFunc("GET /dl/{token}", s.download) // signed temp download links

	// private mail (login-gated in the handlers; feature-gated by the sysop).
	mux.HandleFunc("GET /mail", s.feature("email", s.mailInbox))
	mux.HandleFunc("GET /mail/sent", s.feature("email", s.mailSent))
	mux.HandleFunc("GET /mail/compose", s.feature("email", s.mailComposeForm))
	mux.HandleFunc("POST /mail/compose", s.feature("email", s.mailSend))
	mux.HandleFunc("GET /mail/{id}", s.feature("email", s.mailRead))
	mux.HandleFunc("POST /mail/{id}/delete", s.feature("email", s.mailDelete))

	// voting booth. The literal /voting/new segment outranks /voting/{id}.
	mux.HandleFunc("GET /voting", s.feature("voting", s.votingList))
	mux.HandleFunc("GET /voting/new", s.feature("voting", s.votingNewForm))
	mux.HandleFunc("POST /voting/new", s.feature("voting", s.votingCreate))
	mux.HandleFunc("GET /voting/{id}", s.feature("voting", s.votingShow))
	mux.HandleFunc("POST /voting/{id}", s.feature("voting", s.votingVote))

	// bbs list (directory of other boards).
	mux.HandleFunc("GET /bbslist", s.feature("bbslist", s.bbsList))
	mux.HandleFunc("GET /bbslist/add", s.feature("bbslist", s.bbsAddForm))
	mux.HandleFunc("POST /bbslist/add", s.feature("bbslist", s.bbsAdd))

	// g-files (text library).
	mux.HandleFunc("GET /gfiles", s.feature("gfiles", s.gfilesList))
	mux.HandleFunc("GET /gfiles/{id}", s.feature("gfiles", s.gfileRead))

	// qwk offline mail (login-gated in the handlers; feature-gated by the sysop).
	mux.HandleFunc("GET /qwk", s.feature("qwk", s.qwkPageHandler))
	mux.HandleFunc("GET /qwk/download", s.feature("qwk", s.qwkDownload))
	mux.HandleFunc("POST /qwk/upload", s.feature("qwk", s.qwkUpload))

	// settings (profile + password) and public system info.
	mux.HandleFunc("GET /settings", s.settings)
	mux.HandleFunc("POST /settings", s.settingsSave)
	mux.HandleFunc("POST /settings/password", s.settingsPassword)
	mux.HandleFunc("GET /sysinfo", s.sysinfo)

	// auth (sessions): login / register / logout.
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("GET /register", s.registerForm)
	mux.HandleFunc("POST /register", s.register)
	mux.HandleFunc("POST /logout", s.logout)

	// sysop panel: every route is admin-gated by s.admin (SL >= 100 / flag A).
	// This is the board's configuration program -- full CRUD over every entity.
	mux.HandleFunc("GET /sysop", s.admin(s.sysop))

	// users
	mux.HandleFunc("GET /sysop/users", s.admin(s.sysopUsers))
	mux.HandleFunc("POST /sysop/users/{id}/level", s.admin(s.sysopSetLevel))
	mux.HandleFunc("GET /sysop/users/{id}/edit", s.admin(s.sysopUserForm))
	mux.HandleFunc("POST /sysop/users/{id}", s.admin(s.sysopUserSave))
	mux.HandleFunc("POST /sysop/users/{id}/delete", s.admin(s.sysopUserDelete))

	// message bases (CRUD)
	mux.HandleFunc("GET /sysop/boards", s.admin(s.sysopBoards))
	mux.HandleFunc("GET /sysop/boards/new", s.admin(s.sysopBoardForm))
	mux.HandleFunc("GET /sysop/boards/{id}/edit", s.admin(s.sysopBoardForm))
	mux.HandleFunc("POST /sysop/boards", s.admin(s.sysopBoardSave))
	mux.HandleFunc("POST /sysop/boards/{id}/delete", s.admin(s.sysopBoardDelete))

	// file areas (CRUD)
	mux.HandleFunc("GET /sysop/areas", s.admin(s.sysopAreas))
	mux.HandleFunc("GET /sysop/areas/new", s.admin(s.sysopAreaForm))
	mux.HandleFunc("GET /sysop/areas/{id}/edit", s.admin(s.sysopAreaForm))
	mux.HandleFunc("POST /sysop/areas", s.admin(s.sysopAreaSave))
	mux.HandleFunc("POST /sysop/areas/{id}/delete", s.admin(s.sysopAreaDelete))

	// g-files (CRUD)
	mux.HandleFunc("GET /sysop/gfiles", s.admin(s.sysopGfiles))
	mux.HandleFunc("GET /sysop/gfiles/new", s.admin(s.sysopGfileForm))
	mux.HandleFunc("GET /sysop/gfiles/{id}/edit", s.admin(s.sysopGfileForm))
	mux.HandleFunc("POST /sysop/gfiles", s.admin(s.sysopGfileSave))
	mux.HandleFunc("POST /sysop/gfiles/{id}/delete", s.admin(s.sysopGfileDelete))

	mux.HandleFunc("GET /sysop/bulletins", s.admin(s.sysopBulletins))
	mux.HandleFunc("GET /sysop/bulletins/new", s.admin(s.sysopBulletinForm))
	mux.HandleFunc("GET /sysop/bulletins/{id}/edit", s.admin(s.sysopBulletinForm))
	mux.HandleFunc("POST /sysop/bulletins", s.admin(s.sysopBulletinSave))
	mux.HandleFunc("POST /sysop/bulletins/{id}/delete", s.admin(s.sysopBulletinDelete))

	// bbs list (CRUD)
	mux.HandleFunc("GET /sysop/bbslist", s.admin(s.sysopBbslist))
	mux.HandleFunc("GET /sysop/bbslist/new", s.admin(s.sysopBbsForm))
	mux.HandleFunc("GET /sysop/bbslist/{id}/edit", s.admin(s.sysopBbsForm))
	mux.HandleFunc("POST /sysop/bbslist", s.admin(s.sysopBbsSave))
	mux.HandleFunc("POST /sysop/bbslist/{id}/delete", s.admin(s.sysopBbsDelete))

	// voting (moderation)
	mux.HandleFunc("GET /sysop/voting", s.admin(s.sysopVoting))
	mux.HandleFunc("POST /sysop/voting/{id}/close", s.admin(s.sysopVotingClose))
	mux.HandleFunc("POST /sysop/voting/{id}/delete", s.admin(s.sysopVotingDelete))

	// doors (CRUD)
	mux.HandleFunc("GET /sysop/doors", s.admin(s.sysopDoors))
	mux.HandleFunc("GET /sysop/doors/new", s.admin(s.sysopDoorForm))
	mux.HandleFunc("GET /sysop/doors/{id}/edit", s.admin(s.sysopDoorForm))
	mux.HandleFunc("POST /sysop/doors", s.admin(s.sysopDoorSave))
	mux.HandleFunc("POST /sysop/doors/{id}/delete", s.admin(s.sysopDoorDelete))

	// the wall (moderation)
	mux.HandleFunc("GET /sysop/oneliners", s.admin(s.sysopOneliners))
	mux.HandleFunc("POST /sysop/oneliners/{id}/delete", s.admin(s.sysopOnelinerDelete))

	// global settings (board identity, new-user defaults, feature toggles)
	mux.HandleFunc("GET /sysop/settings", s.admin(s.sysopSettings))
	mux.HandleFunc("POST /sysop/settings", s.admin(s.sysopSettingsSave))

	// Wrap the whole router in the CSRF guard so every state-changing request
	// is origin-checked in one place.
	return s.csrfGuard(mux)
}

// csrfGuard rejects cross-origin state-changing requests. Modern browsers send
// an Origin header on every unsafe-method request (including top-level form
// navigations, the gap SameSite=Lax leaves open), so verifying it against the
// request's own Host blocks the classic forged-POST attack -- most importantly
// the privilege-escalation routes under /sysop -- without threading a token
// through every form. Safe methods (GET/HEAD/OPTIONS) pass through untouched.
func (s *server) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !sameOrigin(r) {
				http.Error(w, "cross-origin request blocked", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin reports whether an unsafe request originates from the board itself.
// It prefers the Origin header and falls back to Referer; a cross-site forged
// POST always carries one of them with a foreign host, which is rejected. When
// neither is present (non-browser clients; some same-origin legacy cases) the
// request is allowed -- a browser can't be coerced into a header-less
// cross-origin POST, so this isn't a bypass.
func sameOrigin(r *http.Request) bool {
	for _, h := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		if h == "" {
			continue
		}
		u, err := url.Parse(h)
		if err != nil || u.Host == "" {
			return false
		}
		return u.Host == r.Host
	}
	return true
}

type server struct {
	st       *store.Store
	online   func() []string
	tmpl     map[string]*template.Template
	sessions *sessionManager
	cfg      Config
	dlSecret []byte // HMAC key for signed download links (random per process)

	loginThrottle *throttle.Throttle // per-IP failed-login limiter

	// feature data layers (each owns its own table over st.DB()).
	mail      *mail.Store
	voting    *voting.Store
	bbslist   *bbslist.Store
	gfiles    *gfiles.Store
	doorStore *door.Store
	bulletins *bulletin.Store
}

// parseTemplates builds one isolated template set per page file, each set being
// base.html + every partial + that one page. Isolation keeps each page's
// {{"content"}} block from colliding with the others'. The set is named by the
// page's filename (sans extension); render executes "base" within it.
func parseTemplates() map[string]*template.Template {
	partials, _ := fs.Glob(assets, "templates/partials/*.html")
	pages, err := fs.Glob(assets, "templates/pages/*.html")
	if err != nil || len(pages) == 0 {
		log.Printf("web: no page templates found: %v", err)
	}
	out := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		name := strings.TrimSuffix(path.Base(page), ".html")
		files := append([]string{"templates/base.html"}, partials...)
		files = append(files, page)
		t := template.Must(
			template.New(name).Funcs(funcs).ParseFS(assets, files...))
		out[name] = t
	}
	return out
}

// render executes a page's set through the base layout with a 200 status.
func (s *server) render(w http.ResponseWriter, page string, data any) {
	s.renderStatus(w, http.StatusOK, page, data)
}

// renderStatus executes a page's set through the base layout with an explicit
// status code. Errors are logged, not surfaced as a half-written 500.
func (s *server) renderStatus(w http.ResponseWriter, code int, page string, data any) {
	t := s.tmpl[page]
	if t == nil {
		log.Printf("web: render: unknown page %q", page)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("web: render %s: %v", page, err)
	}
}

// admin wraps a handler so only sysop-level users reach it. An anonymous caller
// is sent to login (with a return path); a logged-in non-admin gets a 403
// "forbidden" page. This is the single choke point for every /sysop route.
func (s *server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.currentUser(r)
		if u == nil {
			http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
			return
		}
		if !isAdmin(u) {
			s.renderStatus(w, http.StatusForbidden, "forbidden", s.base(r, "forbidden", ""))
			return
		}
		// Audit every sysop mutation (state-changing requests) with the actor
		// and source, so config/user/content changes leave a trail.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			log.Printf("[audit] %s %s by %s from %s", r.Method, r.URL.Path, u.Handle, s.clientIP(r))
		}
		h(w, r)
	}
}

// css concatenates every static/*.css file in filename order (so 00-base.css,
// the design system, comes first and feature sheets layer on top).
func (s *server) css(w http.ResponseWriter, r *http.Request) {
	files, _ := fs.Glob(assets, "static/*.css")
	sort.Strings(files)
	if len(files) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	for _, f := range files {
		b, err := assets.ReadFile(f)
		if err != nil {
			continue
		}
		w.Write(b)
		w.Write([]byte("\n"))
	}
}

// ---- shared page data ----

type pageData struct {
	Title  string
	Active string
	Online []string
	// User is the logged-in user for this request, or nil when anonymous. The
	// masthead and gated actions key off it.
	User    *store.User
	IsAdmin bool
	// Features maps each toggleable feature key to whether the sysop has it on,
	// so the masthead can hide links to closed features.
	Features map[string]bool
}

// base builds the shared page data for a request, including the current user
// (resolved from the session cookie) so every page's masthead reflects login
// state.
func (s *server) base(r *http.Request, title, active string) pageData {
	u := s.currentUser(r)
	feats := make(map[string]bool, len(store.Features))
	for _, f := range store.Features {
		feats[f.Key] = s.st.FeatureEnabled(f.Key)
	}
	return pageData{
		Title:    title,
		Active:   active,
		Online:   s.online(),
		User:     u,
		IsAdmin:  isAdmin(u),
		Features: feats,
	}
}

// feature wraps a public handler so a sysop-disabled feature is hidden from the
// web face: a request to a closed feature redirects home rather than serving
// it. The sysop's own /sysop admin routes are never gated this way.
func (s *server) feature(key string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.st.FeatureEnabled(key) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		h(w, r)
	}
}

// acsSubjectOf maps a web user (possibly nil/anonymous) onto an ACS subject.
// Web callers are treated as ANSI-capable; an anonymous caller has zero access.
func acsSubjectOf(u *store.User) acs.Subject {
	if u == nil {
		return acs.Subject{Ansi: true}
	}
	return acs.Subject{
		SL:     u.SL,
		DSL:    u.DSL,
		Flags:  u.Flags,
		UserID: u.ID,
		Group:  u.Group,
		Ansi:   true,
	}
}

// isAdmin reports whether u has sysop-level access (SL >= 100 or the "A" flag).
func isAdmin(u *store.User) bool {
	if u == nil {
		return false
	}
	if u.SL >= 100 {
		return true
	}
	for _, f := range u.Flags {
		if f == 'A' || f == 'a' {
			return true
		}
	}
	return false
}

// ---- shared helpers ----

// clientIP returns the caller's IP for rate-limiting. The X-Forwarded-For
// header is honored ONLY when the operator has declared a trusted proxy
// (cfg.TrustProxy) -- otherwise any client could spoof XFF and key every login
// attempt to a fresh throttle bucket, defeating the limiter. With no trusted
// proxy the real TCP peer (RemoteAddr) is always used.
func (s *server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// parseID parses a positive int64 path/query id. Returns false on garbage.
func parseID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 0 {
		return 0, false
	}
	return id, true
}

// boardByID returns the board with the given id, or nil.
func (s *server) boardByID(id int64) *store.Board {
	bs, err := s.st.Boards()
	if err != nil {
		log.Printf("web: Boards: %v", err)
		return nil
	}
	for i := range bs {
		if bs[i].ID == id {
			return &bs[i]
		}
	}
	return nil
}

// areaByID returns the file area with the given id, or nil.
func (s *server) areaByID(id int64) *store.FileArea {
	as, err := s.st.FileAreas()
	if err != nil {
		log.Printf("web: FileAreas: %v", err)
		return nil
	}
	for i := range as {
		if as[i].ID == id {
			return &as[i]
		}
	}
	return nil
}

// ---- template funcs (kept rich so feature pages never need to add their own) ----

var funcs = template.FuncMap{
	"fdate": func(t time.Time) string {
		if t.IsZero() {
			return "----------"
		}
		return t.Format("2006-01-02 15:04")
	},
	"ddate": func(t time.Time) string {
		if t.IsZero() {
			return "never"
		}
		return t.Format("2006-01-02")
	},
	"since":    since,
	"fsize":    humanSize,
	"upper":    strings.ToUpper,
	"lower":    strings.ToLower,
	"title":    titleize,
	"initials": initials,
	"hue":      hue,
	"trunc":    trunc,
	"plural":   plural,
	"add":      func(a, b int) int { return a + b },
	"pct":      pct,
	"dict":     dict,
	"nlbr":     nlbr,
}

// pct returns 100*n/max clamped to 0..100 (for ratio/leaderboard bars).
func pct(n, max int) int {
	if max <= 0 {
		return 0
	}
	v := n * 100 / max
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// dict builds a map from alternating key/value args, letting templates pass
// structured data into a reusable partial: {{template "c-badge" dict "label" x}}.
func dict(kv ...any) map[string]any {
	m := make(map[string]any, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	return m
}

// since renders a compact relative time like "3h ago" / "just now".
func since(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	default:
		return strconv.Itoa(int(d.Hours()/24/365)) + "y ago"
	}
}

// humanSize renders a byte count in a compact retro style (e.g. "1.2M").
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + "B"
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	units := []string{"K", "M", "G", "T", "P"}
	v := float64(n) / float64(div)
	return strconv.FormatFloat(v, 'f', 1, 64) + units[exp]
}

// titleize upper-cases the first letter of each space-separated word.
func titleize(s string) string {
	out := []rune(strings.ToLower(s))
	start := true
	for i, r := range out {
		if r == ' ' {
			start = true
			continue
		}
		if start && r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
		start = false
	}
	return string(out)
}

// initials returns up to two upper-case letters/digits from a handle, for
// deterministic avatar chips.
func initials(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "??"
	}
	r := []rune(s)
	first := strings.ToUpper(string(r[0]))
	if len(r) > 1 {
		return first + strings.ToUpper(string(r[1]))
	}
	return first
}

// hue maps a string to a stable 0..359 hue, for per-handle accent colors.
func hue(s string) int {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return int(h % 360)
}

// trunc shortens s to n runes, adding an ellipsis when cut.
func trunc(n int, s string) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// plural picks the singular or plural word for a count.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// nlbr escapes text and turns newlines into <br>, for safe multi-line bodies.
func nlbr(s string) template.HTML {
	esc := template.HTMLEscapeString(s)
	return template.HTML(strings.ReplaceAll(esc, "\n", "<br>"))
}
