package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/store"
)

// Sysop CRUD for message bases and file areas -- the heart of the BBS
// configuration program. All routes here are admin-gated by the router.

// ---- message bases ---------------------------------------------------------

type sysopBoardRow struct {
	Board store.Board
	Count int
}

// sysopBoards lists every message base with edit/delete actions and a "new
// base" button.
func (s *server) sysopBoards(w http.ResponseWriter, r *http.Request) {
	var rows []sysopBoardRow
	if bs, err := s.st.Boards(); err == nil {
		for _, b := range bs {
			c := 0
			if ms, err := s.st.Messages(b.ID, 0); err == nil {
				c = len(ms)
			}
			rows = append(rows, sysopBoardRow{Board: b, Count: c})
		}
	} else {
		log.Printf("web: sysop boards: %v", err)
	}
	s.render(w, "sysop_boards", struct {
		pageData
		Boards []sysopBoardRow
	}{s.base(r, "sysop / message bases", "config"), rows})
}

// sysopBoardForm renders the new/edit form. An empty path id is the new-base
// case; a valid id prefills from the store (bad id bounces to the list).
func (s *server) sysopBoardForm(w http.ResponseWriter, r *http.Request) {
	var board store.Board
	editing := false
	if raw := r.PathValue("id"); raw != "" {
		id, ok := parseID(raw)
		if !ok {
			http.Redirect(w, r, "/sysop/boards", http.StatusSeeOther)
			return
		}
		b, err := s.st.BoardByID(id)
		if err != nil || b == nil {
			http.Redirect(w, r, "/sysop/boards", http.StatusSeeOther)
			return
		}
		board, editing = *b, true
	}
	s.render(w, "sysop_board_edit", struct {
		pageData
		Board   store.Board
		Editing bool
	}{s.base(r, "sysop / edit base", "config"), board, editing})
}

// sysopBoardSave creates or updates a base from the form, then returns to the
// list. A blank name or tag bounces back to the form.
func (s *server) sysopBoardSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/boards", http.StatusSeeOther)
		return
	}
	id, _ := parseID(r.FormValue("id"))
	b := store.Board{
		ID:        id,
		Tag:       strings.TrimSpace(r.FormValue("tag")),
		Name:      strings.TrimSpace(r.FormValue("name")),
		Desc:      strings.TrimSpace(r.FormValue("descr")),
		MinReadSL: clampLevel(r.FormValue("min_read_sl"), 0),
		MinPostSL: clampLevel(r.FormValue("min_post_sl"), 0),
		ReadACS:   strings.TrimSpace(r.FormValue("read_acs")),
		PostACS:   strings.TrimSpace(r.FormValue("post_acs")),
	}
	if b.Name == "" || b.Tag == "" {
		// Required fields missing: bounce back to the right form.
		if id > 0 {
			http.Redirect(w, r, "/sysop/boards/"+r.FormValue("id")+"/edit", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/sysop/boards/new", http.StatusSeeOther)
		}
		return
	}
	var err error
	if id > 0 {
		err = s.st.UpdateBoard(&b)
	} else {
		_, err = s.st.AddBoard(&b)
	}
	if err != nil {
		log.Printf("web: sysop board save: %v", err)
	}
	http.Redirect(w, r, "/sysop/boards", http.StatusSeeOther)
}

// sysopBoardDelete removes a base (and its messages), then returns to the list.
func (s *server) sysopBoardDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/boards", http.StatusSeeOther)
		return
	}
	if err := s.st.DeleteBoard(id); err != nil {
		log.Printf("web: sysop board delete: %v", err)
	}
	http.Redirect(w, r, "/sysop/boards", http.StatusSeeOther)
}

// ---- file areas ------------------------------------------------------------

type sysopAreaRow struct {
	Area  store.FileArea
	Count int
}

// sysopAreas lists every file area with edit/delete actions and a "new area"
// button.
func (s *server) sysopAreas(w http.ResponseWriter, r *http.Request) {
	var rows []sysopAreaRow
	if as, err := s.st.FileAreas(); err == nil {
		for _, a := range as {
			c := 0
			if fs, err := s.st.Files(a.ID); err == nil {
				c = len(fs)
			}
			rows = append(rows, sysopAreaRow{Area: a, Count: c})
		}
	} else {
		log.Printf("web: sysop areas: %v", err)
	}
	s.render(w, "sysop_areas", struct {
		pageData
		Areas []sysopAreaRow
	}{s.base(r, "sysop / file areas", "areas"), rows})
}

// sysopAreaForm renders the new/edit form for a file area.
func (s *server) sysopAreaForm(w http.ResponseWriter, r *http.Request) {
	var area store.FileArea
	editing := false
	if raw := r.PathValue("id"); raw != "" {
		id, ok := parseID(raw)
		if !ok {
			http.Redirect(w, r, "/sysop/areas", http.StatusSeeOther)
			return
		}
		a, err := s.st.FileAreaByID(id)
		if err != nil || a == nil {
			http.Redirect(w, r, "/sysop/areas", http.StatusSeeOther)
			return
		}
		area, editing = *a, true
	}
	s.render(w, "sysop_area_edit", struct {
		pageData
		Area    store.FileArea
		Editing bool
	}{s.base(r, "sysop / edit area", "areas"), area, editing})
}

// sysopAreaSave creates or updates a file area from the form.
func (s *server) sysopAreaSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/areas", http.StatusSeeOther)
		return
	}
	id, _ := parseID(r.FormValue("id"))
	a := store.FileArea{
		ID:   id,
		Tag:  strings.TrimSpace(r.FormValue("tag")),
		Name: strings.TrimSpace(r.FormValue("name")),
		Desc: strings.TrimSpace(r.FormValue("descr")),
		ACS:  strings.TrimSpace(r.FormValue("acs")),
	}
	if a.Name == "" || a.Tag == "" {
		if id > 0 {
			http.Redirect(w, r, "/sysop/areas/"+r.FormValue("id")+"/edit", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/sysop/areas/new", http.StatusSeeOther)
		}
		return
	}
	var err error
	if id > 0 {
		err = s.st.UpdateFileArea(&a)
	} else {
		_, err = s.st.AddFileArea(&a)
	}
	if err != nil {
		log.Printf("web: sysop area save: %v", err)
	}
	http.Redirect(w, r, "/sysop/areas", http.StatusSeeOther)
}

// sysopAreaDelete removes a file area (and its files), then returns to the list.
func (s *server) sysopAreaDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/areas", http.StatusSeeOther)
		return
	}
	if err := s.st.DeleteFileArea(id); err != nil {
		log.Printf("web: sysop area delete: %v", err)
	}
	http.Redirect(w, r, "/sysop/areas", http.StatusSeeOther)
}
