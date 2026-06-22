package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/door"
)

// Sysop CRUD for external doors. All routes here are admin-gated by the router.

// sysopDoors lists every configured door with edit/delete actions and a "new
// door" button.
func (s *server) sysopDoors(w http.ResponseWriter, r *http.Request) {
	var doors []door.Door
	if s.doorStore != nil {
		if ds, err := s.doorStore.List(); err == nil {
			doors = ds
		} else {
			log.Printf("web: sysop doors: %v", err)
		}
	}
	s.render(w, "sysop_doors", struct {
		pageData
		Doors []door.Door
	}{s.base(r, "sysop / doors", "doors"), doors})
}

// sysopDoorForm renders the new/edit form. An empty path id is the new-door
// case; a valid id prefills from the store (bad id bounces to the list).
func (s *server) sysopDoorForm(w http.ResponseWriter, r *http.Request) {
	var d door.Door
	editing := false
	if raw := r.PathValue("id"); raw != "" {
		id, ok := parseID(raw)
		if !ok || s.doorStore == nil {
			http.Redirect(w, r, "/sysop/doors", http.StatusSeeOther)
			return
		}
		got, err := s.doorStore.Get(id)
		if err != nil || got == nil {
			http.Redirect(w, r, "/sysop/doors", http.StatusSeeOther)
			return
		}
		d, editing = *got, true
	}
	s.render(w, "sysop_door_edit", struct {
		pageData
		Door    door.Door
		Editing bool
	}{s.base(r, "sysop / edit door", "doors"), d, editing})
}

// sysopDoorSave creates or updates a door from the form, then returns to the
// list. A blank name bounces back to the form.
func (s *server) sysopDoorSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/doors", http.StatusSeeOther)
		return
	}
	id, _ := parseID(r.FormValue("id"))
	dropType := strings.TrimSpace(r.FormValue("drop_type"))
	if dropType != "DORINFO1.DEF" {
		dropType = "DOOR.SYS"
	}
	d := door.Door{
		ID:          id,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Command:     strings.TrimSpace(r.FormValue("command")),
		WorkDir:     strings.TrimSpace(r.FormValue("workdir")),
		DropType:    dropType,
		DOSPath:     strings.TrimSpace(r.FormValue("dos_path")),
		Enabled:     r.FormValue("enabled") != "",
	}
	if d.Name == "" {
		if id > 0 {
			http.Redirect(w, r, "/sysop/doors/"+r.FormValue("id")+"/edit", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/sysop/doors/new", http.StatusSeeOther)
		}
		return
	}
	if s.doorStore != nil {
		var err error
		if id > 0 {
			err = s.doorStore.Update(&d)
		} else {
			_, err = s.doorStore.Add(&d)
		}
		if err != nil {
			log.Printf("web: sysop door save: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/doors", http.StatusSeeOther)
}

// sysopDoorDelete removes a door, then returns to the list.
func (s *server) sysopDoorDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/doors", http.StatusSeeOther)
		return
	}
	if s.doorStore != nil {
		if err := s.doorStore.Delete(id); err != nil {
			log.Printf("web: sysop door delete: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/doors", http.StatusSeeOther)
}
