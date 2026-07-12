package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/auth"
)

// settingsData is the view model for the settings page: the current user (for
// prefilling the profile form) plus optional ok/error notices.
type settingsData struct {
	pageData
	OK    string
	PwOK  string
	PwErr string
}

// settings renders the self-service screen: a profile form (real name, email,
// location, tagline) and a separate change-password form. Login required.
func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/settings", http.StatusSeeOther)
		return
	}
	q := r.URL.Query()
	data := settingsData{
		pageData: s.base(r, "settings", ""),
	}
	switch q.Get("ok") {
	case "profile":
		data.OK = "Profile saved."
	case "password":
		data.PwOK = "Password changed."
	}
	if msg := q.Get("err"); msg != "" {
		data.PwErr = msg
	}
	s.render(w, "settings", data)
}

// settingsSave handles the profile form post, writing the self-editable fields
// through the store and redirecting back with a success indicator.
func (s *server) settingsSave(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/settings", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?err=Bad+form.", http.StatusSeeOther)
		return
	}
	realName := strings.TrimSpace(r.FormValue("real_name"))
	email := strings.TrimSpace(r.FormValue("email"))
	location := strings.TrimSpace(r.FormValue("location"))
	tagline := strings.TrimSpace(r.FormValue("tagline"))
	// Birthday is validated before anything is written, so a bad date reports
	// cleanly instead of half-saving the profile.
	if err := s.st.SetBirthday(u.ID, r.FormValue("birthday")); err != nil {
		http.Redirect(w, r, "/settings?err=Birthday+must+look+like+MM-DD.", http.StatusSeeOther)
		return
	}
	if err := s.st.UpdateProfile(u.ID, realName, email, location, tagline); err != nil {
		log.Printf("web: settings UpdateProfile: %v", err)
		http.Redirect(w, r, "/settings?err=Could+not+save+your+profile.", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?ok=profile", http.StatusSeeOther)
}

// settingsPassword handles the change-password form post: it verifies the
// current password (when one is on file), requires a non-empty matching new
// password, then stores the hash. Login required.
func (s *server) settingsPassword(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/settings", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?err=Bad+form.", http.StatusSeeOther)
		return
	}
	current := r.FormValue("current")
	newPw := r.FormValue("new")
	verify := r.FormValue("verify")

	if u.Password != "" && !auth.Verify(u.Password, current) {
		http.Redirect(w, r, "/settings?err=Current+password+is+wrong.", http.StatusSeeOther)
		return
	}
	if newPw == "" || newPw != verify {
		http.Redirect(w, r, "/settings?err=New+passwords+must+match+and+be+non-empty.", http.StatusSeeOther)
		return
	}
	hash, err := auth.Hash(newPw)
	if err != nil {
		http.Redirect(w, r, "/settings?err=Could+not+set+password.", http.StatusSeeOther)
		return
	}
	if err := s.st.SetPassword(u.ID, hash); err != nil {
		log.Printf("web: settings SetPassword: %v", err)
		http.Redirect(w, r, "/settings?err=Could+not+save+password.", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?ok=password", http.StatusSeeOther)
}
