package web

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"vendetta-x/server/internal/store"
)

// ---- dashboard -------------------------------------------------------------

// sysopStats are the headline counts shown on the sysop dashboard.
type sysopStats struct {
	Users, Messages, Files, Boards, Areas, Oneliners, Online int
}

func (s *server) gatherStats() sysopStats {
	var st sysopStats
	if us, err := s.st.Users(); err == nil {
		st.Users = len(us)
	}
	if ms, err := s.st.RecentMessages(0); err == nil {
		st.Messages = len(ms)
	}
	if bs, err := s.st.Boards(); err == nil {
		st.Boards = len(bs)
	}
	if areas, err := s.st.FileAreas(); err == nil {
		st.Areas = len(areas)
		for _, a := range areas {
			if fs, err := s.st.Files(a.ID); err == nil {
				st.Files += len(fs)
			}
		}
	}
	if ol, err := s.st.Oneliners(0); err == nil {
		st.Oneliners = len(ol)
	}
	st.Online = len(s.online())
	return st
}

// sysop renders the sysop dashboard: live nodes, system stats, server config.
func (s *server) sysop(w http.ResponseWriter, r *http.Request) {
	s.render(w, "sysop", struct {
		pageData
		Stats  sysopStats
		Cfg    Config
		Uptime string
		Nodes  []string
	}{
		pageData: s.base(r, "sysop", "sysop"),
		Stats:    s.gatherStats(),
		Cfg:      s.cfg,
		Uptime:   uptimeStr(time.Since(s.cfg.Started)),
		Nodes:    s.online(),
	})
}

// ---- users management ------------------------------------------------------

// sysopUsers lists every user with controls to adjust security levels.
func (s *server) sysopUsers(w http.ResponseWriter, r *http.Request) {
	us, err := s.st.Users()
	if err != nil {
		log.Printf("web: sysop users: %v", err)
	}
	s.render(w, "sysop_users", struct {
		pageData
		Users []store.User
	}{s.base(r, "sysop / users", "sysop"), us})
}

// sysopSetLevel updates a user's SL/DSL (clamped 0..255), then returns to the
// user list. Admin-gated by the router; values are validated here.
func (s *server) sysopSetLevel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	u, err := s.st.UserByID(id)
	if err != nil || u == nil {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	sl := clampLevel(r.FormValue("sl"), u.SL)
	dsl := clampLevel(r.FormValue("dsl"), u.DSL)
	if err := s.st.SetLevels(id, sl, dsl); err != nil {
		log.Printf("web: sysop set levels: %v", err)
	}
	http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
}

// ---- user editing ----------------------------------------------------------

// sysopUserForm renders the full edit form for a single user (group, flags,
// security levels, profile fields). Bad/missing id bounces to the list.
func (s *server) sysopUserForm(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	u, err := s.st.UserByID(id)
	if err != nil || u == nil {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	s.render(w, "sysop_user_edit", struct {
		pageData
		U store.User
	}{s.base(r, "sysop / edit user", "users"), *u})
}

// sysopUserSave persists the full user edit (everything but handle/password/
// counters), then returns to the user list.
func (s *server) sysopUserSave(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	u, err := s.st.UserByID(id)
	if err != nil || u == nil {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	u.RealName = strings.TrimSpace(r.FormValue("real_name"))
	u.Email = strings.TrimSpace(r.FormValue("email"))
	u.Location = strings.TrimSpace(r.FormValue("location"))
	u.Tagline = strings.TrimSpace(r.FormValue("tagline"))
	u.Group = strings.TrimSpace(r.FormValue("grp"))
	u.Flags = strings.TrimSpace(r.FormValue("flags"))
	u.SL = clampLevel(r.FormValue("sl"), u.SL)
	u.DSL = clampLevel(r.FormValue("dsl"), u.DSL)
	if err := s.st.UpdateUser(u); err != nil {
		log.Printf("web: sysop user save: %v", err)
	}
	http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
}

// sysopUserDelete removes a user account. The sysop cannot delete their own
// account from here (a guard against locking yourself out).
func (s *server) sysopUserDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	if me := s.currentUser(r); me != nil && me.ID == id {
		http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
		return
	}
	if err := s.st.DeleteUser(id); err != nil {
		log.Printf("web: sysop user delete: %v", err)
	}
	http.Redirect(w, r, "/sysop/users", http.StatusSeeOther)
}

// ---- oneliners (the wall) moderation ---------------------------------------

// sysopOneliners lists the wall with a delete control per entry.
func (s *server) sysopOneliners(w http.ResponseWriter, r *http.Request) {
	liners, err := s.st.Oneliners(0)
	if err != nil {
		log.Printf("web: sysop oneliners: %v", err)
	}
	s.render(w, "sysop_oneliners", struct {
		pageData
		Liners []store.Oneliner
	}{s.base(r, "sysop / the wall", "oneliners"), liners})
}

// sysopOnelinerDelete removes one wall entry, then returns to the list.
func (s *server) sysopOnelinerDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/oneliners", http.StatusSeeOther)
		return
	}
	if err := s.st.DeleteOneliner(id); err != nil {
		log.Printf("web: sysop oneliner delete: %v", err)
	}
	http.Redirect(w, r, "/sysop/oneliners", http.StatusSeeOther)
}

// ---- helpers ---------------------------------------------------------------

// clampLevel parses a level string, clamping to 0..255; on parse failure it
// keeps the current value.
func clampLevel(raw string, current int) int {
	v, ok := parseID(raw)
	if !ok {
		return current
	}
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(v)
}

// uptimeStr renders a duration as a compact "2d 3h 14m".
func uptimeStr(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
