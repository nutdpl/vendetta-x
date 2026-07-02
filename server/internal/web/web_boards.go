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

// boardCard pairs a board with its message count, most-recent post, and the
// viewer's unread count, so the index can show activity at a glance without
// the template touching the store.
type boardCard struct {
	Board   store.Board
	Count   int
	New     int
	Last    store.Message
	HasLast bool
}

// boards renders the board index. Owned by the boards feature.
func (s *server) boards(w http.ResponseWriter, r *http.Request) {
	bs, err := s.st.Boards()
	if err != nil {
		log.Printf("web: Boards: %v", err)
	}

	// Per-viewer unread counts (the qscan pointers) light up the "new" badges;
	// an anonymous visitor just doesn't get them.
	var unread map[int64]int
	u := s.currentUser(r)
	if u != nil {
		if unread, err = s.st.UnreadCounts(u.ID); err != nil {
			log.Printf("web: UnreadCounts: %v", err)
		}
	}
	subj := acsSubjectOf(u)

	cards := make([]boardCard, 0, len(bs))
	for _, b := range bs {
		c := boardCard{Board: b}
		msgs, err := s.st.Messages(b.ID, 0)
		if err != nil {
			log.Printf("web: Messages: %v", err)
		}
		c.Count = len(msgs)
		if u != nil && acs.Eval(b.ReadACS, subj) {
			c.New = unread[b.ID]
		}
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
		// Viewing the board catches the caller up: advance their qscan pointer
		// to the newest message shown (monotonic, so a stale tab can't rewind).
		if base.User != nil && len(msgs) > 0 {
			if err := s.st.SetLastRead(base.User.ID, id, msgs[0].ID); err != nil {
				log.Printf("web: SetLastRead: %v", err)
			}
		}
	}

	// ?re=<msgid> opens the post form as a threaded reply: subject prefilled,
	// the original >-quoted in the textarea, reply_to carried in a hidden
	// field. Only honored for a message actually on this board.
	var reply *store.Message
	var replyQuote, replySubj string
	if canPost {
		if reID, ok := parseID(r.URL.Query().Get("re")); ok {
			if m, err := s.st.MessageByID(reID); err == nil && m != nil && m.BoardID == id {
				reply = m
				replyQuote = quoteWeb(m)
				replySubj = m.Subject
				if !strings.HasPrefix(strings.ToLower(replySubj), "re:") {
					replySubj = "Re: " + replySubj
				}
			}
		}
	}

	if board != nil {
		base.Title = board.Name
	}
	s.render(w, "board", boardPage{
		pageData:   base,
		ID:         id,
		Board:      board,
		Messages:   msgs,
		Nodes:      threadNodes(msgs),
		CanRead:    canRead,
		CanPost:    canPost,
		Reply:      reply,
		ReplyQuote: replyQuote,
		ReplySubj:  replySubj,
	})
}

// boardPage is the data for the single-board view.
type boardPage struct {
	pageData
	ID       int64
	Board    *store.Board
	Messages []store.Message
	Nodes    []msgNode
	CanRead  bool
	CanPost  bool
	// Reply is the message the post form is answering (nil = fresh post),
	// with the prefilled >-quoted body and "Re:" subject alongside.
	Reply      *store.Message
	ReplyQuote string
	ReplySubj  string
}

// msgNode is one message in thread order: roots newest-first, each root's
// replies nested under it oldest-first. Reply carries the indent (a single
// visual level, however deep the chain goes) and ParentFrom the "re:" credit.
type msgNode struct {
	store.Message
	Reply      bool
	ParentFrom string
}

// threadNodes arranges a newest-first message window into display order:
// thread roots keep their newest-first order, and every reply chain hangs
// under its root in conversation (oldest-first) order. A reply whose parent
// has scrolled out of the window stands as a root.
func threadNodes(msgs []store.Message) []msgNode {
	inWindow := make(map[int64]*store.Message, len(msgs))
	for i := range msgs {
		inWindow[msgs[i].ID] = &msgs[i]
	}
	kids := map[int64][]*store.Message{}
	var roots []*store.Message
	for i := range msgs {
		m := &msgs[i]
		if m.ReplyTo != 0 && inWindow[m.ReplyTo] != nil {
			kids[m.ReplyTo] = append(kids[m.ReplyTo], m) // newest-first
		} else {
			roots = append(roots, m)
		}
	}
	nodes := make([]msgNode, 0, len(msgs))
	var hang func(parent *store.Message)
	hang = func(parent *store.Message) {
		ks := kids[parent.ID]
		for j := len(ks) - 1; j >= 0; j-- { // reverse: oldest-first
			nodes = append(nodes, msgNode{Message: *ks[j], Reply: true, ParentFrom: parent.From})
			hang(ks[j])
		}
	}
	for _, root := range roots {
		nodes = append(nodes, msgNode{Message: *root})
		hang(root)
	}
	return nodes
}

// quoteWeb renders a message as >-quoted text for the web reply textarea,
// mirroring the terminal composer's classic "Handle> line" prefill.
func quoteWeb(m *store.Message) string {
	lines := strings.Split(m.Body, "\n")
	if len(lines) > 20 {
		lines = append(lines[:20], "[...]")
	}
	var sb strings.Builder
	for _, ln := range lines {
		sb.WriteString(m.From + "> " + strings.TrimRight(ln, " \t") + "\n")
	}
	sb.WriteString("\n")
	return sb.String()
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
	// A threaded reply carries the parent's id; it only sticks when the
	// parent really is a message on this board (a forged or stale id posts
	// as a fresh thread root instead).
	var replyTo int64
	to := "All"
	if reID, ok := parseID(r.FormValue("reply_to")); ok {
		if parent, err := s.st.MessageByID(reID); err == nil && parent != nil && parent.BoardID == id {
			replyTo = parent.ID
			to = parent.From
		}
	}
	if body != "" {
		m := &store.Message{
			BoardID: id,
			From:    u.Handle, // attributed to the logged-in user
			To:      to,
			Subject: subject,
			Body:    body,
			Posted:  time.Now(),
			ReplyTo: replyTo,
		}
		if _, err := s.st.PostMessage(m); err != nil {
			log.Printf("web: PostMessage: %v", err)
		} else if err := s.st.IncPosts(u.Handle); err != nil {
			log.Printf("web: IncPosts: %v", err)
		}
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
