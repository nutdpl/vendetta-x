package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/bbslist"
)

// bbsList renders the BBS List: a directory of other boards as a .table, with
// an "add a board" action for logged-in callers. Owned by the bbslist feature.
func (s *server) bbsList(w http.ResponseWriter, r *http.Request) {
	entries, err := s.bbslist.List()
	if err != nil {
		log.Printf("web: bbslist List: %v", err)
	}
	s.render(w, "bbslist", struct {
		pageData
		Entries []bbslist.Entry
	}{s.base(r, "bbs list", "bbslist"), entries})
}

// bbsAddForm renders the "add a board" form. Login is required: an anonymous
// caller is sent to login with a return path.
func (s *server) bbsAddForm(w http.ResponseWriter, r *http.Request) {
	if s.currentUser(r) == nil {
		http.Redirect(w, r, "/login?next=/bbslist/add", http.StatusSeeOther)
		return
	}
	s.render(w, "bbslist_add", struct {
		pageData
	}{s.base(r, "add a board", "bbslist")})
}

// bbsAdd handles the add-a-board form post, then 303s back to the list. Login
// is required and a Name is mandatory.
func (s *server) bbsAdd(w http.ResponseWriter, r *http.Request) {
	if s.currentUser(r) == nil {
		http.Redirect(w, r, "/login?next=/bbslist/add", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/bbslist/add", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		// Name is mandatory: bounce back to the form.
		http.Redirect(w, r, "/bbslist/add", http.StatusSeeOther)
		return
	}
	if _, err := s.bbslist.Add(&bbslist.Entry{
		Name:     name,
		Address:  strings.TrimSpace(r.FormValue("address")),
		Software: strings.TrimSpace(r.FormValue("software")),
		Sysop:    strings.TrimSpace(r.FormValue("sysop")),
		Desc:     strings.TrimSpace(r.FormValue("descr")),
	}); err != nil {
		log.Printf("web: bbslist Add: %v", err)
	}
	http.Redirect(w, r, "/bbslist", http.StatusSeeOther)
}
