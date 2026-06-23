package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/bulletin"
)

// sysopBulletins lists every bulletin (newest first) for the sysop panel, with
// per-row edit/delete controls and a "+ new bulletin" affordance.
func (s *server) sysopBulletins(w http.ResponseWriter, r *http.Request) {
	items, err := s.bulletins.List()
	if err != nil {
		log.Printf("web: sysop bulletins List: %v", err)
	}
	s.render(w, "sysop_bulletins", struct {
		pageData
		Bulletins []bulletin.Bulletin
	}{s.base(r, "bulletins", "sysop"), items})
}

// sysopBulletinForm renders the new/edit bulletin form. When the path carries a
// valid id of an existing bulletin, the form is prefilled for editing;
// otherwise it is a blank create form. A bad/missing id falls back to the list.
func (s *server) sysopBulletinForm(w http.ResponseWriter, r *http.Request) {
	var b *bulletin.Bulletin
	if raw := r.PathValue("id"); raw != "" {
		id, ok := parseID(raw)
		if !ok {
			http.Redirect(w, r, "/sysop/bulletins", http.StatusSeeOther)
			return
		}
		got, err := s.bulletins.Get(id)
		if err != nil {
			log.Printf("web: sysop bulletin Get: %v", err)
		}
		b = got
	}
	s.render(w, "sysop_bulletin_edit", struct {
		pageData
		Bulletin *bulletin.Bulletin
	}{s.base(r, "edit bulletin", "sysop"), b})
}

// sysopBulletinSave handles the create/update post. An id>0 updates an existing
// bulletin; otherwise a new one is added. Title is required -- a blank title
// bounces back to the form. Store errors are logged, never surfaced as a 500.
func (s *server) sysopBulletinSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/bulletins", http.StatusSeeOther)
		return
	}
	id, _ := parseID(r.FormValue("id"))
	title := strings.TrimSpace(r.FormValue("title"))
	author := strings.TrimSpace(r.FormValue("author"))
	body := r.FormValue("body")

	if title == "" {
		if id > 0 {
			http.Redirect(w, r, "/sysop/bulletins/"+r.FormValue("id")+"/edit", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/sysop/bulletins/new", http.StatusSeeOther)
		}
		return
	}

	if id > 0 {
		if err := s.bulletins.Update(&bulletin.Bulletin{
			ID:     id,
			Title:  title,
			Body:   body,
			Author: author,
		}); err != nil {
			log.Printf("web: sysop bulletin Update: %v", err)
		}
	} else {
		if _, err := s.bulletins.Add(&bulletin.Bulletin{
			Title:  title,
			Body:   body,
			Author: author,
		}); err != nil {
			log.Printf("web: sysop bulletin Add: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/bulletins", http.StatusSeeOther)
}

// sysopBulletinDelete removes a bulletin and returns to the list. A bad id or a
// store error never 500s.
func (s *server) sysopBulletinDelete(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseID(r.PathValue("id")); ok {
		if err := s.bulletins.Delete(id); err != nil {
			log.Printf("web: sysop bulletin Delete: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/bulletins", http.StatusSeeOther)
}
