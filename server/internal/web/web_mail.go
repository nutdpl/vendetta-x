package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/mail"
)

// mailPage is the data for the inbox/sent listing (one template, two modes).
type mailPage struct {
	pageData
	Mode     string // "inbox" or "sent"
	Messages []mail.Message
	Unread   int
}

// mailReadPage is the data for a single message view.
type mailReadPage struct {
	pageData
	Msg      mail.Message
	IsInbox  bool // the current user is the recipient (can reply)
	ReplyTo  string
	ReplySub string
}

// mailComposePage is the data for the compose form.
type mailComposePage struct {
	pageData
	To      string
	Subject string
	Body    string
	Err     string
}

// mailInbox renders the caller's inbox (GET /mail, login required).
func (s *server) mailInbox(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
		return
	}
	msgs, err := s.mail.Inbox(u.Handle)
	if err != nil {
		log.Printf("web: mail Inbox: %v", err)
	}
	unread, _ := s.mail.UnreadCount(u.Handle)
	s.render(w, "mail", mailPage{
		pageData: s.base(r, "mail", "mail"),
		Mode:     "inbox",
		Messages: msgs,
		Unread:   unread,
	})
}

// mailSent renders the caller's sent mail (GET /mail/sent).
func (s *server) mailSent(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
		return
	}
	msgs, err := s.mail.Outbox(u.Handle)
	if err != nil {
		log.Printf("web: mail Outbox: %v", err)
	}
	unread, _ := s.mail.UnreadCount(u.Handle)
	s.render(w, "mail", mailPage{
		pageData: s.base(r, "sent", "mail"),
		Mode:     "sent",
		Messages: msgs,
		Unread:   unread,
	})
}

// mailComposeForm renders the compose form (GET /mail/compose), optionally
// prefilling To via ?to= and Subject via ?re= (a reply subject).
func (s *server) mailComposeForm(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
		return
	}
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	subj := strings.TrimSpace(r.URL.Query().Get("re"))
	if subj != "" && !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	s.render(w, "mail_compose", mailComposePage{
		pageData: s.base(r, "compose", "mail"),
		To:       to,
		Subject:  subj,
	})
}

// mailSend handles the compose POST (POST /mail/compose). On success it 303s to
// /mail; a missing recipient re-renders the form with an error.
func (s *server) mailSend(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/mail/compose", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/mail/compose", http.StatusSeeOther)
		return
	}
	to := strings.TrimSpace(r.FormValue("to"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	body := strings.TrimSpace(r.FormValue("body"))

	reRender := func(msg string) {
		s.render(w, "mail_compose", mailComposePage{
			pageData: s.base(r, "compose", "mail"),
			To:       to,
			Subject:  subject,
			Body:     body,
			Err:      msg,
		})
	}

	if to == "" {
		reRender("Enter a recipient handle.")
		return
	}
	rcpt, err := s.st.UserByHandle(to)
	if err != nil {
		log.Printf("web: mail UserByHandle: %v", err)
		reRender("Could not look up that user.")
		return
	}
	if rcpt == nil {
		reRender("No user by that handle.")
		return
	}
	if body == "" {
		reRender("Write a message body.")
		return
	}
	if subject == "" {
		subject = "(no subject)"
	}
	if err := s.mail.Send(u.Handle, rcpt.Handle, subject, body); err != nil {
		log.Printf("web: mail Send: %v", err)
		reRender("Could not send your mail.")
		return
	}
	http.Redirect(w, r, "/mail", http.StatusSeeOther)
}

// mailRead shows a single message (GET /mail/{id}). Only the sender or
// recipient may view it; anything else (bad id, foreign mail) redirects to
// /mail rather than erroring. Opening it as the recipient marks it read.
func (s *server) mailRead(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
		return
	}
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/mail", http.StatusSeeOther)
		return
	}
	m, err := s.mail.Get(id)
	if err != nil {
		log.Printf("web: mail Get: %v", err)
	}
	if m == nil {
		http.Redirect(w, r, "/mail", http.StatusSeeOther)
		return
	}
	isInbox := strings.EqualFold(m.To, u.Handle)
	isOutbox := strings.EqualFold(m.From, u.Handle)
	if !isInbox && !isOutbox {
		http.Redirect(w, r, "/mail", http.StatusSeeOther)
		return
	}
	if isInbox && !m.Read {
		if err := s.mail.MarkRead(m.ID); err != nil {
			log.Printf("web: mail MarkRead: %v", err)
		}
		m.Read = true
	}

	replySub := m.Subject
	if !strings.HasPrefix(strings.ToLower(replySub), "re:") {
		replySub = "Re: " + replySub
	}
	s.render(w, "mail_read", mailReadPage{
		pageData: s.base(r, m.Subject, "mail"),
		Msg:      *m,
		IsInbox:  isInbox,
		ReplyTo:  m.From,
		ReplySub: replySub,
	})
}

// mailDelete deletes a message the caller owns (POST /mail/{id}/delete), then
// 303s to /mail. A bad/foreign id is a no-op redirect (Delete is ownership-
// gated in the store).
func (s *server) mailDelete(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/mail", http.StatusSeeOther)
		return
	}
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/mail", http.StatusSeeOther)
		return
	}
	if err := s.mail.Delete(id, u.Handle); err != nil {
		log.Printf("web: mail Delete: %v", err)
	}
	http.Redirect(w, r, "/mail", http.StatusSeeOther)
}
