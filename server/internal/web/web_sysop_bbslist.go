package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/bbslist"
)

// sysopBbslist lists every BBS-directory entry with edit/delete controls. The
// router wraps this with s.admin; no self-gating here.
func (s *server) sysopBbslist(w http.ResponseWriter, r *http.Request) {
	entries, err := s.bbslist.List()
	if err != nil {
		log.Printf("web: sysop bbslist List: %v", err)
	}
	s.render(w, "sysop_bbslist", struct {
		pageData
		Entries []bbslist.Entry
	}{s.base(r, "sysop / bbs list", "sysop"), entries})
}

// sysopBbsForm renders the new/edit form. With a parseable {id} that resolves
// to an entry it prefills for editing; otherwise it's a blank "new" form. A
// malformed id bounces back to the list.
func (s *server) sysopBbsForm(w http.ResponseWriter, r *http.Request) {
	var entry bbslist.Entry
	if raw := r.PathValue("id"); raw != "" {
		id, ok := parseID(raw)
		if !ok {
			http.Redirect(w, r, "/sysop/bbslist", http.StatusSeeOther)
			return
		}
		e, err := s.bbslist.Get(id)
		if err != nil {
			log.Printf("web: sysop bbslist Get: %v", err)
		}
		if e != nil {
			entry = *e
		}
	}
	s.render(w, "sysop_bbs_edit", struct {
		pageData
		Entry bbslist.Entry
	}{s.base(r, "sysop / bbs list", "sysop"), entry})
}

// sysopBbsSave handles the new/edit form post. A blank name bounces back to the
// form; id>0 updates an existing entry, otherwise a new one is added. On success
// it 303s to the list. Store errors are logged, never surfaced as a 500.
func (s *server) sysopBbsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/bbslist", http.StatusSeeOther)
		return
	}
	id, _ := parseID(r.FormValue("id"))
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		// Name is mandatory: bounce back to the same form.
		if id > 0 {
			http.Redirect(w, r, "/sysop/bbslist/"+r.FormValue("id")+"/edit", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/sysop/bbslist/new", http.StatusSeeOther)
		}
		return
	}
	entry := bbslist.Entry{
		ID:       id,
		Name:     name,
		Address:  strings.TrimSpace(r.FormValue("address")),
		Software: strings.TrimSpace(r.FormValue("software")),
		Sysop:    strings.TrimSpace(r.FormValue("sysop")),
		Desc:     strings.TrimSpace(r.FormValue("descr")),
	}
	if id > 0 {
		if err := s.bbslist.Update(&entry); err != nil {
			log.Printf("web: sysop bbslist Update: %v", err)
		}
	} else {
		if _, err := s.bbslist.Add(&entry); err != nil {
			log.Printf("web: sysop bbslist Add: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/bbslist", http.StatusSeeOther)
}

// sysopBbsDelete deletes a directory entry, then 303s to the list. A malformed
// id is treated as a no-op redirect.
func (s *server) sysopBbsDelete(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseID(r.PathValue("id")); ok {
		if err := s.bbslist.Delete(id); err != nil {
			log.Printf("web: sysop bbslist Delete: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/bbslist", http.StatusSeeOther)
}
