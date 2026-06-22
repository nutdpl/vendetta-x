package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/gfiles"
)

// sysopGfiles lists every G-Files document (no body) for the sysop panel, with
// per-row edit/delete controls and a "+ new doc" affordance.
func (s *server) sysopGfiles(w http.ResponseWriter, r *http.Request) {
	docs, err := s.gfiles.List("")
	if err != nil {
		log.Printf("web: sysop gfiles List: %v", err)
	}
	s.render(w, "sysop_gfiles", struct {
		pageData
		Docs []gfiles.GFile
	}{s.base(r, "g-files", "sysop"), docs})
}

// sysopGfileForm renders the new/edit document form. When the path carries a
// valid id of an existing doc, the form is prefilled for editing; otherwise it
// is a blank create form. A bad/missing id falls back to the list.
func (s *server) sysopGfileForm(w http.ResponseWriter, r *http.Request) {
	var doc *gfiles.GFile
	if raw := r.PathValue("id"); raw != "" {
		id, ok := parseID(raw)
		if !ok {
			http.Redirect(w, r, "/sysop/gfiles", http.StatusSeeOther)
			return
		}
		g, err := s.gfiles.Get(id)
		if err != nil {
			log.Printf("web: sysop gfile Get: %v", err)
		}
		doc = g
	}
	s.render(w, "sysop_gfile_edit", struct {
		pageData
		Doc *gfiles.GFile
	}{s.base(r, "edit g-file", "sysop"), doc})
}

// sysopGfileSave handles the create/update post. An id>0 updates an existing
// doc; otherwise a new one is added. Title is required -- a blank title bounces
// back to the form. Store errors are logged, never surfaced as a 500.
func (s *server) sysopGfileSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/gfiles", http.StatusSeeOther)
		return
	}
	id, _ := parseID(r.FormValue("id"))
	category := strings.TrimSpace(r.FormValue("category"))
	title := strings.TrimSpace(r.FormValue("title"))
	body := r.FormValue("body")
	author := strings.TrimSpace(r.FormValue("author"))

	if title == "" {
		if id > 0 {
			http.Redirect(w, r, "/sysop/gfiles/"+r.FormValue("id")+"/edit", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/sysop/gfiles/new", http.StatusSeeOther)
		}
		return
	}

	if id > 0 {
		if err := s.gfiles.Update(&gfiles.GFile{
			ID:       id,
			Category: category,
			Title:    title,
			Body:     body,
			Author:   author,
		}); err != nil {
			log.Printf("web: sysop gfile Update: %v", err)
		}
	} else {
		if _, err := s.gfiles.Add(&gfiles.GFile{
			Category: category,
			Title:    title,
			Body:     body,
			Author:   author,
		}); err != nil {
			log.Printf("web: sysop gfile Add: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/gfiles", http.StatusSeeOther)
}

// sysopGfileDelete removes a document and returns to the list. A bad id or a
// store error never 500s.
func (s *server) sysopGfileDelete(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseID(r.PathValue("id")); ok {
		if err := s.gfiles.Delete(id); err != nil {
			log.Printf("web: sysop gfile Delete: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/gfiles", http.StatusSeeOther)
}
