package web

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"vendetta-x/server/internal/ftn"
)

// FTN networking config: one page listing every uplink the board has joined
// (fsxNet, FidoNet, AgoraNet, ... simultaneously), an editor per link with
// its echo<->board map, and the run-now button. The scheduler's
// ftn.exchange action does the unattended polling.

type ftnLinkRow struct {
	Link   ftn.Link
	Echoes int
}

func (s *server) sysopFTN(w http.ResponseWriter, r *http.Request) {
	links, err := s.ftn.Links()
	if err != nil {
		log.Printf("web: ftn links: %v", err)
	}
	rows := make([]ftnLinkRow, 0, len(links))
	for _, l := range links {
		echoes, _ := s.ftn.Echoes(l.ID)
		rows = append(rows, ftnLinkRow{Link: l, Echoes: len(echoes)})
	}
	s.render(w, "sysop_ftn", struct {
		pageData
		Links      []ftnLinkRow
		LastStatus string
		RunResult  string
		Err        string
	}{s.base(r, "sysop / networks", "ftn"), rows,
		s.st.Setting("ftn.last_status", ""),
		r.URL.Query().Get("ran"),
		r.URL.Query().Get("err")})
}

func (s *server) sysopFTNEdit(w http.ResponseWriter, r *http.Request) {
	var link ftn.Link
	var echoLines string
	if id, ok := parseID(r.PathValue("id")); ok && id > 0 {
		l, err := s.ftn.LinkByID(id)
		if err != nil || l == nil {
			http.Redirect(w, r, "/sysop/ftn", http.StatusSeeOther)
			return
		}
		link = *l
		echoes, _ := s.ftn.Echoes(id)
		var sb strings.Builder
		for _, e := range echoes {
			sb.WriteString(e.Tag + " = " + e.BoardTag + "\n")
		}
		echoLines = sb.String()
	}
	s.render(w, "sysop_ftn_edit", struct {
		pageData
		Link  ftn.Link
		Echos string
	}{s.base(r, "sysop / networks", "ftn"), link, echoLines})
}

// sysopFTNSave creates (id 0) or updates a link plus its echo map.
func (s *server) sysopFTNSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/ftn", http.StatusSeeOther)
		return
	}
	link := ftn.Link{
		Name:     r.FormValue("name"),
		Host:     r.FormValue("host"),
		OurAddr:  strings.TrimSpace(r.FormValue("our_addr")),
		HubAddr:  strings.TrimSpace(r.FormValue("hub_addr")),
		Password: r.FormValue("password"),
		Enabled:  r.FormValue("enabled") != "",
	}
	if id, ok := parseID(r.FormValue("id")); ok {
		link.ID = id
	}
	if err := s.ftn.SaveLink(&link); err != nil {
		http.Redirect(w, r, "/sysop/ftn?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	pairs := map[string]string{}
	for _, ln := range strings.Split(r.FormValue("echoes"), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		if tag, board, ok := strings.Cut(ln, "="); ok {
			pairs[strings.TrimSpace(tag)] = strings.TrimSpace(board)
		}
	}
	if err := s.ftn.SetEchoes(link.ID, pairs); err != nil {
		log.Printf("web: ftn set echoes: %v", err)
	}
	http.Redirect(w, r, "/sysop/ftn", http.StatusSeeOther)
}

func (s *server) sysopFTNDelete(w http.ResponseWriter, r *http.Request) {
	if id, ok := parseID(r.PathValue("id")); ok {
		if err := s.ftn.DeleteLink(id); err != nil {
			log.Printf("web: ftn delete: %v", err)
		}
	}
	http.Redirect(w, r, "/sysop/ftn", http.StatusSeeOther)
}

// sysopFTNRun fires an exchange across all enabled links right now.
func (s *server) sysopFTNRun(w http.ResponseWriter, r *http.Request) {
	sum, err := ftn.Exchange(s.st, s.ftn)
	if err != nil {
		http.Redirect(w, r, "/sysop/ftn?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/sysop/ftn?ran="+url.QueryEscape(sum.String()), http.StatusSeeOther)
}
