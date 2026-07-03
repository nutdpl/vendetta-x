package web

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"vendetta-x/server/internal/guard"
)

// The ban list: durable door policy for when the throttle isn't enough.
// Three kinds -- single IP, CIDR range, and handle patterns (the trashcan) --
// enforced on every face. The loopback console can never be banned.

func (s *server) sysopBans(w http.ResponseWriter, r *http.Request) {
	bans, err := s.guard.List()
	if err != nil {
		log.Printf("web: guard list: %v", err)
	}
	s.render(w, "sysop_bans", struct {
		pageData
		Bans []guard.Ban
		Err  string
	}{s.base(r, "sysop / bans", "bans"), bans, r.URL.Query().Get("err")})
}

func (s *server) sysopBanAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/bans", http.StatusSeeOther)
		return
	}
	days := 0
	if d, ok := parseID(r.FormValue("days")); ok && d > 0 && d < 36500 {
		days = int(d)
	}
	err := s.guard.Add(
		strings.TrimSpace(r.FormValue("kind")),
		r.FormValue("value"),
		strings.TrimSpace(r.FormValue("reason")),
		days)
	if err != nil {
		http.Redirect(w, r, "/sysop/bans?err="+strconv.Itoa(1), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/sysop/bans", http.StatusSeeOther)
}

func (s *server) sysopBanDelete(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseID(r.PathValue("id")); ok {
		if err := s.guard.Delete(id); err != nil {
			log.Printf("web: guard delete: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/bans", http.StatusSeeOther)
}
