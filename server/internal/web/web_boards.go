package web

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/store"
)

// boardCard pairs a board with its message count and most-recent post, so the
// index can show activity at a glance without the template touching the store.
type boardCard struct {
	Board   store.Board
	Count   int
	Last    store.Message
	HasLast bool
}

// boards renders the board index. Owned by the boards feature.
func (s *server) boards(w http.ResponseWriter, r *http.Request) {
	bs, err := s.st.Boards()
	if err != nil {
		log.Printf("web: Boards: %v", err)
	}

	cards := make([]boardCard, 0, len(bs))
	for _, b := range bs {
		c := boardCard{Board: b}
		msgs, err := s.st.Messages(b.ID, 0)
		if err != nil {
			log.Printf("web: Messages: %v", err)
		}
		c.Count = len(msgs)
		if len(msgs) > 0 {
			// Messages returns newest-first, so the first row is the latest.
			c.Last = msgs[0]
			c.HasLast = true
		}
		cards = append(cards, c)
	}

	s.render(w, "boards", struct {
		pageData
		Cards []boardCard
	}{s.base(r, "boards", "boards"), cards})
}

// board renders a single board's messages, ACS-gated: a board with a ReadACS
// the caller doesn't satisfy shows a "restricted" panel instead of its posts.
func (s *server) board(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		s.render(w, "board", boardPage{pageData: s.base(r, "board", "boards")})
		return
	}

	base := s.base(r, "board", "boards")
	board := s.boardByID(id)
	subj := acsSubjectOf(base.User)

	canRead := board == nil || acs.Eval(board.ReadACS, subj)
	canPost := board != nil && base.User != nil && acs.Eval(board.PostACS, subj)

	var msgs []store.Message
	if board != nil && canRead {
		var err error
		if msgs, err = s.st.Messages(id, 50); err != nil {
			log.Printf("web: Messages: %v", err)
		}
	}

	if board != nil {
		base.Title = board.Name
	}
	s.render(w, "board", boardPage{
		pageData: base,
		ID:       id,
		Board:    board,
		Messages: msgs,
		CanRead:  canRead,
		CanPost:  canPost,
	})
}

// boardPage is the data for the single-board view.
type boardPage struct {
	pageData
	ID       int64
	Board    *store.Board
	Messages []store.Message
	CanRead  bool
	CanPost  bool
}

// postMessage handles the web compose form, then 303s back to the board. It
// requires a logged-in user (the post is attributed to them) and enforces the
// board's PostACS.
func (s *server) postMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/boards", http.StatusSeeOther)
		return
	}
	dest := "/boards/" + strconv.FormatInt(id, 10)

	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+dest, http.StatusSeeOther)
		return
	}

	board := s.boardByID(id)
	if board != nil && !acs.Eval(board.PostACS, acsSubjectOf(u)) {
		// No post access: bounce back; the page will show it read-only.
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	subject := strings.TrimSpace(r.FormValue("subject"))
	body := strings.TrimSpace(r.FormValue("body"))
	if subject == "" {
		subject = "(no subject)"
	}
	if body != "" {
		m := &store.Message{
			BoardID: id,
			From:    u.Handle, // attributed to the logged-in user
			To:      "All",
			Subject: subject,
			Body:    body,
			Posted:  time.Now(),
		}
		if _, err := s.st.PostMessage(m); err != nil {
			log.Printf("web: PostMessage: %v", err)
		} else if err := s.st.IncPosts(u.Handle); err != nil {
			log.Printf("web: IncPosts: %v", err)
		}
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
