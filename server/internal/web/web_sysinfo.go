package web

import (
	"net/http"
)

// sysinfo renders the public credits / stats screen: a row of headline stat
// tiles plus a panel with software/version and the credits blurb. No login
// required.
func (s *server) sysinfo(w http.ResponseWriter, r *http.Request) {
	s.render(w, "sysinfo", struct {
		pageData
		Stats sysopStats
		Cfg   Config
	}{
		pageData: s.base(r, "system info", ""),
		Stats:    s.gatherStats(),
		Cfg:      s.cfg,
	})
}
