package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/audit"
	"vendetta-x/server/internal/store"
)

// dayBar is one column of the activity sparkline: the day plus each metric's
// height as a 0..100 percentage of the busiest day in the window, so the
// template can draw bars without knowing the peak.
type dayBar struct {
	store.DayStat
	PostsPct   int
	UploadsPct int
	SignupsPct int
}

// sysopStats renders the read-only board dashboard: headline totals, a 30-day
// activity sparkline (posts/uploads/signups), and the busiest bases.
func (s *server) sysopStats(w http.ResponseWriter, r *http.Request) {
	totals, err := s.st.Totals()
	if err != nil {
		log.Printf("web: Totals: %v", err)
	}
	days, err := s.st.DailyActivity(30)
	if err != nil {
		log.Printf("web: DailyActivity: %v", err)
	}
	topBoards, err := s.st.TopBoards(8)
	if err != nil {
		log.Printf("web: TopBoards: %v", err)
	}

	// Scale every bar against the single busiest metric-day in the window, so
	// the three series stay comparable to each other.
	peak := 1
	for _, d := range days {
		for _, v := range []int{d.Posts, d.Uploads, d.Signups} {
			if v > peak {
				peak = v
			}
		}
	}
	bars := make([]dayBar, len(days))
	var sumPosts, sumUploads, sumSignups int
	for i, d := range days {
		bars[i] = dayBar{
			DayStat:    d,
			PostsPct:   d.Posts * 100 / peak,
			UploadsPct: d.Uploads * 100 / peak,
			SignupsPct: d.Signups * 100 / peak,
		}
		sumPosts += d.Posts
		sumUploads += d.Uploads
		sumSignups += d.Signups
	}

	s.render(w, "sysop_stats", struct {
		pageData
		Totals     store.Totals
		Bars       []dayBar
		TopBoards  []store.BoardCount
		SumPosts   int
		SumUploads int
		SumSignups int
		WindowDays int
	}{s.base(r, "sysop / stats", "stats"), totals, bars, topBoards,
		sumPosts, sumUploads, sumSignups, len(days)})
}

// sysopAudit renders the durable audit trail, newest first, with an optional
// actor filter (?actor=nut).
func (s *server) sysopAudit(w http.ResponseWriter, r *http.Request) {
	actor := strings.TrimSpace(r.URL.Query().Get("actor"))
	var entries []audit.Entry
	var total int
	if s.audit != nil {
		var err error
		if entries, err = s.audit.Recent(500, actor); err != nil {
			log.Printf("web: audit recent: %v", err)
		}
		if total, err = s.audit.Count(); err != nil {
			log.Printf("web: audit count: %v", err)
		}
	}
	s.render(w, "sysop_audit", struct {
		pageData
		Entries []audit.Entry
		Actor   string
		Total   int
	}{s.base(r, "sysop / audit", "audit"), entries, actor, total})
}
