package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/schedule"
)

// actionLabel returns the catalog label for a stored action key, falling back
// to the raw key itself if it's unrecognized (e.g. left over after a Catalog
// entry was renamed/removed).
func actionLabel(key string) string {
	for _, a := range schedule.Catalog {
		if a.Key == key {
			return a.Label
		}
	}
	return key
}

// sysopEvents lists every scheduled event for the sysop panel, with per-row
// edit/delete controls and a "+ new event" affordance.
func (s *server) sysopEvents(w http.ResponseWriter, r *http.Request) {
	items, err := s.events.List()
	if err != nil {
		log.Printf("web: sysop events List: %v", err)
	}
	s.render(w, "sysop_events", struct {
		pageData
		Events []schedule.Event
	}{s.base(r, "events", "sysop"), items})
}

// sysopEventForm renders the new/edit event form, with the action dropdown
// built from schedule.Catalog. A bad/missing id falls back to the list.
func (s *server) sysopEventForm(w http.ResponseWriter, r *http.Request) {
	var e *schedule.Event
	if raw := r.PathValue("id"); raw != "" {
		id, ok := parseID(raw)
		if !ok {
			http.Redirect(w, r, "/sysop/events", http.StatusSeeOther)
			return
		}
		got, err := s.events.Get(id)
		if err != nil {
			log.Printf("web: sysop event Get: %v", err)
		}
		e = got
	}
	s.render(w, "sysop_event_edit", struct {
		pageData
		Event   *schedule.Event
		Catalog []schedule.ActionDef
	}{s.base(r, "edit event", "sysop"), e, schedule.Catalog})
}

// sysopEventSave handles the create/update post. An id>0 updates an existing
// event; otherwise a new one is added. Name, a known action, and a well-formed
// HH:MM time are required -- anything invalid bounces back to the form. Store
// errors are logged, never surfaced as a 500.
func (s *server) sysopEventSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/events", http.StatusSeeOther)
		return
	}
	id, _ := parseID(r.FormValue("id"))
	name := strings.TrimSpace(r.FormValue("name"))
	action := r.FormValue("action")
	timeOfDay := strings.TrimSpace(r.FormValue("time_of_day"))
	enabled := r.FormValue("enabled") != ""

	back := "/sysop/events/new"
	if id > 0 {
		back = "/sysop/events/" + r.FormValue("id") + "/edit"
	}
	if name == "" || !knownAction(action) || !validTimeOfDay(timeOfDay) {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}

	e := &schedule.Event{ID: id, Name: name, Action: action, TimeOfDay: timeOfDay, Enabled: enabled}
	if id > 0 {
		if err := s.events.Update(e); err != nil {
			log.Printf("web: sysop event Update: %v", err)
		}
	} else {
		if _, err := s.events.Add(e); err != nil {
			log.Printf("web: sysop event Add: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/events", http.StatusSeeOther)
}

// sysopEventDelete removes a scheduled event and returns to the list. A bad id
// or a store error never 500s.
func (s *server) sysopEventDelete(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseID(r.PathValue("id")); ok {
		if err := s.events.Delete(id); err != nil {
			log.Printf("web: sysop event Delete: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/events", http.StatusSeeOther)
}

// knownAction reports whether key is one of the catalog's action keys.
func knownAction(key string) bool {
	for _, a := range schedule.Catalog {
		if a.Key == key {
			return true
		}
	}
	return false
}

// validTimeOfDay reports whether s is a well-formed "HH:MM" 24-hour time, the
// same shape the <input type="time"> form field submits.
func validTimeOfDay(s string) bool {
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) == 0 || len(h) > 2 || len(m) != 2 {
		return false
	}
	hh, mm := 0, 0
	for _, c := range h {
		if c < '0' || c > '9' {
			return false
		}
		hh = hh*10 + int(c-'0')
	}
	for _, c := range m {
		if c < '0' || c > '9' {
			return false
		}
		mm = mm*10 + int(c-'0')
	}
	return hh <= 23 && mm <= 59
}
