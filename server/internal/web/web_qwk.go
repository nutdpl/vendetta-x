package web

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/qwk"
	"vendetta-x/server/internal/store"
)

// qwkPage is the data for the QWK info page: counts plus the ?posted= flash.
type qwkPage struct {
	pageData
	Boards   int // readable conferences
	Messages int // total messages a packet would carry
	Posted   int // replies imported on the last upload (from ?posted=)
}

// readableConferences returns the caller's readable boards in a STABLE 1-based
// ordering (boards are already returned by store ordered by id). The returned
// slice index+1 is the conference number; this exact ordering is the invariant
// shared by qwkDownload (assigning numbers) and qwkUpload (mapping them back).
func (s *server) readableConferences(u *store.User) []store.Board {
	subj := acsSubjectOf(u)
	boards, err := s.st.Boards()
	if err != nil {
		log.Printf("web: qwk Boards: %v", err)
		return nil
	}
	var out []store.Board
	for i := range boards {
		if acs.Eval(boards[i].ReadACS, subj) {
			out = append(out, boards[i])
		}
	}
	return out
}

// qwkPageHandler renders the QWK info page (GET /qwk, login required).
func (s *server) qwkPageHandler(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
		return
	}

	confs := s.readableConferences(u)
	total := 0
	for _, b := range confs {
		msgs, err := s.st.Messages(b.ID, 0)
		if err != nil {
			log.Printf("web: qwk Messages board %d: %v", b.ID, err)
			continue
		}
		total += len(msgs)
	}

	posted := 0
	if p, err := strconv.Atoi(r.URL.Query().Get("posted")); err == nil && p > 0 {
		posted = p
	}

	s.render(w, "qwk", qwkPage{
		pageData: s.base(r, "qwk", "qwk"),
		Boards:   len(confs),
		Messages: total,
		Posted:   posted,
	})
}

// qwkDownload builds and serves the caller's .QWK packet (GET /qwk/download).
// Conference numbers are assigned 1-based over readableConferences -- the same
// ordering qwkUpload uses to map replies back to boards.
func (s *server) qwkDownload(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
		return
	}

	confs := s.readableConferences(u)

	pk := qwk.Packet{
		BoardName: s.cfg.BoardName,
		Sysop:     "Sysop",
		Caller:    u.Handle,
	}
	for i, b := range confs {
		num := uint16(i + 1)
		pk.Conferences = append(pk.Conferences, qwk.Conference{Number: num, Name: b.Name})

		msgs, err := s.st.Messages(b.ID, 0)
		if err != nil {
			log.Printf("web: qwk Messages board %d: %v", b.ID, err)
			continue
		}
		for _, m := range msgs {
			pk.Messages = append(pk.Messages, qwk.Message{
				Conference: num,
				To:         m.To,
				From:       m.From,
				Subject:    m.Subject,
				Body:       m.Body,
				Date:       m.Posted,
			})
		}
	}

	data, err := qwk.Build(pk)
	if err != nil {
		log.Printf("web: qwk Build: %v", err)
		http.Redirect(w, r, "/qwk", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="VENDX.QWK"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}

// qwkUpload imports a .REP reply packet (POST /qwk/upload, multipart). Each
// reply's conference number is mapped back through the SAME readableConferences
// ordering; replies the caller can post to are posted to that board. Anything
// unmappable, unauthorized, or empty is silently skipped. It never 500s.
func (s *server) qwkUpload(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/qwk", http.StatusSeeOther)
		return
	}

	// Hard-cap the request body before parsing so a client can't stream more
	// than the limit (ParseMultipartForm alone only bounds what's kept in
	// memory, not what's buffered to a temp file). Matches the file uploader.
	const maxUpload = 5 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		log.Printf("web: qwk ParseMultipartForm: %v", err)
		http.Redirect(w, r, "/qwk", http.StatusSeeOther)
		return
	}

	file, _, err := r.FormFile("packet")
	if err != nil {
		log.Printf("web: qwk FormFile: %v", err)
		http.Redirect(w, r, "/qwk", http.StatusSeeOther)
		return
	}
	defer file.Close()

	data := make([]byte, 0, 64<<10)
	buf := make([]byte, 32<<10)
	const maxBytes = 5 << 20
	for {
		n, rerr := file.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
			if len(data) > maxBytes {
				log.Printf("web: qwk upload too large")
				http.Redirect(w, r, "/qwk", http.StatusSeeOther)
				return
			}
		}
		if rerr != nil {
			break
		}
	}

	replies, err := qwk.ParseReply(data)
	if err != nil {
		log.Printf("web: qwk ParseReply: %v", err)
		http.Redirect(w, r, "/qwk", http.StatusSeeOther)
		return
	}

	// Rebuild the SAME 1-based readable-board ordering so conference numbers
	// line up with the ones qwkDownload assigned.
	confs := s.readableConferences(u)
	subj := acsSubjectOf(u)

	posted := 0
	for _, rep := range replies {
		idx := int(rep.Conference) - 1
		if idx < 0 || idx >= len(confs) {
			continue // conference doesn't map to a readable board
		}
		board := confs[idx]
		if !acs.Eval(board.PostACS, subj) {
			continue // caller can't post here
		}
		body := rep.Body
		if body == "" {
			continue
		}
		to := rep.To
		if to == "" {
			to = "All"
		}
		subject := rep.Subject
		if subject == "" {
			subject = "(no subject)"
		}
		if _, err := s.st.PostMessage(&store.Message{
			BoardID: board.ID,
			From:    u.Handle,
			To:      to,
			Subject: subject,
			Body:    body,
			Posted:  time.Now(),
		}); err != nil {
			log.Printf("web: qwk PostMessage board %d: %v", board.ID, err)
			continue
		}
		posted++
	}

	http.Redirect(w, r, "/qwk?posted="+strconv.Itoa(posted), http.StatusSeeOther)
}
