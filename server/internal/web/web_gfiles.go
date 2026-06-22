package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/gfiles"
)

// gfilesList renders the G-Files library: a .tabs row of categories (All + each)
// and a list of documents for the selected category (or all). Owned by the
// gfiles feature.
func (s *server) gfilesList(w http.ResponseWriter, r *http.Request) {
	cats, err := s.gfiles.Categories()
	if err != nil {
		log.Printf("web: gfiles Categories: %v", err)
	}

	// The selected category comes from ?cat=; only honor it if it exists.
	selected := strings.TrimSpace(r.URL.Query().Get("cat"))
	if selected != "" {
		ok := false
		for _, c := range cats {
			if c == selected {
				ok = true
				break
			}
		}
		if !ok {
			selected = ""
		}
	}

	docs, err := s.gfiles.List(selected)
	if err != nil {
		log.Printf("web: gfiles List: %v", err)
	}

	s.render(w, "gfiles", struct {
		pageData
		Categories []string
		Selected   string
		Docs       []gfiles.GFile
	}{s.base(r, "g-files", "gfiles"), cats, selected, docs})
}

// gfileRead renders one document with its body inside a panel. A bad id never
// 500s -- it 303s back to the index.
func (s *server) gfileRead(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/gfiles", http.StatusSeeOther)
		return
	}
	doc, err := s.gfiles.Get(id)
	if err != nil {
		log.Printf("web: gfiles Get: %v", err)
	}
	if doc == nil {
		http.Redirect(w, r, "/gfiles", http.StatusSeeOther)
		return
	}
	base := s.base(r, doc.Title, "gfiles")
	s.render(w, "gfile_read", struct {
		pageData
		Doc *gfiles.GFile
	}{base, doc})
}
