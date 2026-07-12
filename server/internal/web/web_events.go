package web

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"vendetta-x/server/internal/store"
)

// events is the Server-Sent Events stream that makes the board feel live: it
// pushes the who's-online list whenever it changes, so the masthead's node bar
// updates without a reload. Stdlib only -- no framework, no websocket. The
// write deadline is cleared per connection (the HTTP server sets a 120s
// WriteTimeout for ordinary responses; a long-lived stream must opt out).
func (s *server) sseEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	rc := http.NewResponseController(w)
	// Clear the write deadline so the stream can outlive WriteTimeout.
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // don't let a proxy buffer the stream

	send := func() bool {
		nodes := s.online()
		payload, _ := json.Marshal(map[string]any{"count": len(nodes), "nodes": nodes})
		if _, err := fmt.Fprintf(w, "event: presence\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// Push the current state immediately, then only on change, with a periodic
	// heartbeat comment so idle proxies keep the connection open.
	if !send() {
		return
	}
	last := strings.Join(s.online(), "\x00")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	beats := 0
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			cur := strings.Join(s.online(), "\x00")
			if cur != last {
				last = cur
				if !send() {
					return
				}
				beats = 0
				continue
			}
			if beats++; beats >= 10 { // ~20s heartbeat
				beats = 0
				if _, err := fmt.Fprint(w, ": beat\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// ---- Atom feed --------------------------------------------------------------

type atomFeed struct {
	XMLName  xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle,omitempty"`
	ID       string      `xml:"id"`
	Updated  string      `xml:"updated"`
	Links    []atomLink  `xml:"link"`
	Entries  []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	ID      string     `xml:"id"`
	Updated string     `xml:"updated"`
	Links   []atomLink `xml:"link"`
	Author  atomAuthor `xml:"author"`
	Summary string     `xml:"summary"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

// feed serves an Atom feed of recent public posts -- only from world-readable
// (empty-ACS) bases, so nothing gated ever reaches the open web. Feature-gated
// ("feeds").
func (s *server) feed(w http.ResponseWriter, r *http.Request) {
	// Which boards are world-readable (empty ReadACS).
	open := map[int64]string{}
	if bs, err := s.st.Boards(); err == nil {
		for _, b := range bs {
			if strings.TrimSpace(b.ReadACS) == "" {
				open[b.ID] = b.Name
			}
		}
	}

	var msgs []store.Message
	if all, err := s.st.RecentMessages(100); err == nil {
		for _, m := range all {
			if _, ok := open[m.BoardID]; ok {
				msgs = append(msgs, m)
				if len(msgs) >= 30 {
					break
				}
			}
		}
	}

	base := s.absBase(r)
	updated := time.Now().UTC().Format(time.RFC3339)
	if len(msgs) > 0 && !msgs[0].Posted.IsZero() {
		updated = msgs[0].Posted.UTC().Format(time.RFC3339)
	}
	name := s.cfg.BoardName
	if name == "" {
		name = "Vendetta/X"
	}

	feed := atomFeed{
		Title:    name,
		Subtitle: "recent posts from the public message bases",
		ID:       base + "/feed.atom",
		Updated:  updated,
		Links: []atomLink{
			{Href: base + "/feed.atom", Rel: "self", Type: "application/atom+xml"},
			{Href: base + "/", Rel: "alternate", Type: "text/html"},
		},
	}
	for _, m := range msgs {
		posted := m.Posted
		if posted.IsZero() {
			posted = time.Now()
		}
		subject := m.Subject
		if strings.TrimSpace(subject) == "" {
			subject = "(no subject)"
		}
		feed.Entries = append(feed.Entries, atomEntry{
			Title:   subject,
			ID:      fmt.Sprintf("urn:vendetta:msg:%d", m.ID),
			Updated: posted.UTC().Format(time.RFC3339),
			Links:   []atomLink{{Href: fmt.Sprintf("%s/boards/%d", base, m.BoardID), Rel: "alternate", Type: "text/html"}},
			Author:  atomAuthor{Name: firstNonEmpty(m.From, "anon")},
			Summary: summarize(m.Body, 280),
		})
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		return
	}
	w.Write([]byte("\n"))
}

// absBase returns the feed's absolute base URL (scheme + host) from the request.
func (s *server) absBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || s.cfg.SecureCookies || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// summarize trims a body to a short, single-line summary for a feed entry.
func summarize(body string, max int) string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r", ""))
	body = strings.ReplaceAll(body, "\n", " ")
	r := []rune(body)
	if len(r) > max {
		return strings.TrimSpace(string(r[:max-1])) + "…"
	}
	return body
}
