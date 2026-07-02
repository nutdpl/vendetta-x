package web

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"vendetta-x/server/internal/qwknet"
)

// sysopQwknet renders the QWK networking configuration page: hub connection
// details, the conference<->board map, the last exchange's outcome, and a
// "run exchange now" action for testing without waiting on the scheduler.
func (s *server) sysopQwknet(w http.ResponseWriter, r *http.Request) {
	confErr := ""
	if _, err := qwknet.ConfMap(s.st); err != nil {
		confErr = err.Error()
	}
	s.render(w, "sysop_qwknet", struct {
		pageData
		Enabled    bool
		Host       string
		HubUser    string
		HubPass    string
		HubID      string
		NetName    string
		ConfMap    string
		ConfErr    string
		LastStatus string
		Saved      bool
		RunResult  string
	}{
		pageData:   s.base(r, "sysop / qwk-net", "sysop"),
		Enabled:    s.st.SettingBool(qwknet.KeyEnabled, false),
		Host:       s.st.Setting(qwknet.KeyHost, ""),
		HubUser:    s.st.Setting(qwknet.KeyUser, ""),
		HubPass:    s.st.Setting(qwknet.KeyPass, ""),
		HubID:      s.st.Setting(qwknet.KeyHubID, ""),
		NetName:    s.st.Setting(qwknet.KeyNetName, ""),
		ConfMap:    s.st.Setting(qwknet.KeyConfMap, ""),
		ConfErr:    confErr,
		LastStatus: qwknet.LastStatus(s.st),
		Saved:      r.URL.Query().Get("ok") != "",
		RunResult:  r.URL.Query().Get("ran"),
	})
}

// sysopQwknetSave persists the qwknet.* settings and bounces back.
func (s *server) sysopQwknetSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/qwknet", http.StatusSeeOther)
		return
	}
	set := func(key, val string) {
		if err := s.st.SetSetting(key, val); err != nil {
			log.Printf("web: sysop set %s: %v", key, err)
		}
	}
	enabled := "0"
	if r.FormValue("enabled") != "" {
		enabled = "1"
	}
	set(qwknet.KeyEnabled, enabled)
	set(qwknet.KeyHost, strings.TrimSpace(r.FormValue("host")))
	set(qwknet.KeyUser, strings.TrimSpace(r.FormValue("user")))
	set(qwknet.KeyPass, r.FormValue("pass"))
	set(qwknet.KeyHubID, strings.ToUpper(strings.TrimSpace(r.FormValue("hubid"))))
	set(qwknet.KeyNetName, strings.ToUpper(strings.TrimSpace(r.FormValue("netname"))))
	set(qwknet.KeyConfMap, r.FormValue("confmap"))
	http.Redirect(w, r, "/sysop/qwknet?ok=1", http.StatusSeeOther)
}

// sysopQwknetRun fires one exchange immediately (the "does my config work"
// button). The outcome lands in the redirect and in the recorded last status.
func (s *server) sysopQwknetRun(w http.ResponseWriter, r *http.Request) {
	sum, err := qwknet.Exchange(s.st)
	result := sum.String()
	if err != nil {
		result = "failed: " + err.Error()
		log.Printf("web: qwk-net exchange: %v", err)
	}
	http.Redirect(w, r, "/sysop/qwknet?ran="+url.QueryEscape(result), http.StatusSeeOther)
}
