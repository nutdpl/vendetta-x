package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/editor"
	"vendetta-x/server/internal/mail"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// email is the private user-to-user mail screen (the [E]mail feature). It loops
// on the caller's INBOX -- a table of From / Subject / Age with an unread
// marker -- offering Read, Compose, Outbox and Quit, mirroring the Iniquity-
// style list screens (pickBase, listFiles).
func (b *board) email(s *term.Session, tok map[string]string, user *store.User) {
	for {
		msgs, err := b.mail.Inbox(user.Handle)
		if err != nil {
			s.Notice("Could not load your mail.")
			return
		}
		unread, _ := b.mail.UnreadCount(user.Handle)

		b.screenHeader(s, "electronic mail \xfa inbox")
		s.Print("\x1b[1;30m   #  \x1b[0;37mFrom            Subject                       \x1b[1;30mAge\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")

		if len(msgs) == 0 {
			s.Print("\x1b[0;37m  Your mailbox is empty.\x1b[0m\r\n")
		}
		for i, m := range msgs {
			mark := " "
			from := "\x1b[1;37m"
			subj := "\x1b[0;37m"
			if !m.Read {
				mark = "\x1b[1;36m\xfe"
			} else {
				from = "\x1b[1;30m"
				subj = "\x1b[1;30m"
			}
			s.Printf("  %s\x1b[1;33m%2d  %s%-14s %s%-28s \x1b[1;30m%s\x1b[0m\r\n",
				mark, i+1, from, truncStr(m.From, 14), subj, truncStr(m.Subject, 28), relTime(m.Sent))
		}
		s.Printf("\r\n\x1b[1;30m  %d %s, \x1b[1;36m%d\x1b[1;30m unread.\x1b[0m\r\n",
			len(msgs), plural(len(msgs), "message", "messages"), unread)

		s.Print("\r\n\x1b[0;37m  [\x1b[1;37mR\x1b[0;37m]ead  [\x1b[1;37mC\x1b[0;37m]ompose  [\x1b[1;37mO\x1b[0;37m]utbox  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		k, ch := s.ReadKey()
		if k == term.KeyEOF || k == term.KeyEsc {
			return
		}
		switch lc(ch) {
		case 'r':
			b.mailRead(s, user, msgs)
		case 'c':
			b.mailCompose(s, user, "", "")
		case 'o':
			b.mailOutbox(s, user)
		case 'q':
			return
		}
	}
}

// mailRead prompts for a message number (mirroring pickBase's numeric read),
// shows the message, marks it read, then offers Reply / Delete / back.
func (b *board) mailRead(s *term.Session, user *store.User, msgs []mail.Message) {
	if len(msgs) == 0 {
		s.Notice("Nothing to read.")
		return
	}
	s.Print("\r\n\x1b[0;37m  Message \x1b[1;37m#\x1b[0;37m (\x1b[1;37mEnter\x1b[0;37m to cancel) \x1b[1;36m> \x1b[1;37m")
	s.Flush()
	line := strings.TrimSpace(s.ReadLine(5))
	if line == "" {
		return
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(msgs) {
		s.Notice("No such message.")
		return
	}
	m := msgs[n-1]

	b.screenHeader(s, "electronic mail \xfa read")
	s.Printf("  \x1b[1;36mFrom    \x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", m.From)
	s.Printf("  \x1b[1;36mTo      \x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", m.To)
	s.Printf("  \x1b[1;36mSubject \x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", m.Subject)
	s.Printf("  \x1b[1;36mDate    \x1b[1;30m\xb3 \x1b[0;37m%s\x1b[0m\r\n", dateOr(m.Sent))
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n\r\n")
	for _, ln := range strings.Split(m.Body, "\n") {
		s.Print("  \x1b[0;37m" + ln + "\x1b[0m\r\n")
	}
	s.Print("\r\n\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")

	// Mark read once the caller has opened it.
	b.mail.MarkRead(m.ID)

	s.Print("\x1b[0;37m  [\x1b[1;37mR\x1b[0;37m]eply  [\x1b[1;37mD\x1b[0;37m]elete  any key to go back \x1b[1;36m> \x1b[1;37m")
	s.Flush()
	k, ch := s.ReadKey()
	if k != term.KeyChar {
		return
	}
	switch lc(ch) {
	case 'r':
		subj := m.Subject
		if !strings.HasPrefix(strings.ToLower(subj), "re:") {
			subj = "Re: " + subj
		}
		b.mailCompose(s, user, m.From, subj)
	case 'd':
		if err := b.mail.Delete(m.ID, user.Handle); err != nil {
			s.Notice("Could not delete that message.")
			return
		}
		s.Print("\x1b[1;32m  Message deleted.\x1b[0m\r\n")
		s.Pause()
	}
}

// mailCompose runs the To/Subject prompts then the full-screen body editor
// (exactly like postMessage), validating that the recipient exists before
// delivering. toPre/subjPre prefill a reply.
func (b *board) mailCompose(s *term.Session, user *store.User, toPre, subjPre string) {
	b.screenHeader(s, "electronic mail \xfa compose")

	var to string
	for {
		s.Print("\x1b[0;37m  To: \x1b[1;37m")
		if toPre != "" {
			s.Print(toPre)
		}
		s.Flush()
		entered := strings.TrimSpace(s.ReadLine(20))
		if entered == "" {
			entered = toPre
		}
		if entered == "" {
			s.Print("\x1b[1;33m  Compose cancelled.\x1b[0m\r\n")
			s.Pause()
			return
		}
		u, err := b.st.UserByHandle(entered)
		if err != nil {
			s.Notice("Could not look up that user.")
			return
		}
		if u == nil {
			s.Notice("No user by that handle. Try again.")
			toPre = ""
			continue
		}
		to = u.Handle
		break
	}

	s.Print("\x1b[0;37m  Subject: \x1b[1;37m")
	if subjPre != "" {
		s.Print(subjPre)
	}
	s.Flush()
	subject := strings.TrimSpace(s.ReadLine(50))
	if subject == "" {
		subject = subjPre
	}
	if subject == "" {
		subject = "(no subject)"
	}

	// Full-screen editor for the body.
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m mail to %s\x1b[0m\r\n", boardName, to)
	s.Printf("\x1b[1;30m  subject: \x1b[0;37m%s\x1b[0m\r\n", subject)
	s.Print("\x1b[1;30m  Ctrl-Z to save and send \xfa Esc to abort \xfa arrows to move\x1b[0m\r\n")
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	s.Flush()

	ed := editor.New(editorConsole{s}, 5, 3, 72, 16, nil)
	lines, saved := ed.Run()
	// Restore a clean cursor/attr state after the editor.
	s.Print("\x1b[0m\x1b[24;1H\r\n")
	if !saved {
		s.Print("\x1b[1;33m  Mail aborted.\x1b[0m\r\n")
		s.Pause()
		return
	}
	body := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if strings.TrimSpace(body) == "" {
		s.Print("\x1b[1;33m  Empty message -- not sent.\x1b[0m\r\n")
		s.Pause()
		return
	}

	if err := b.mail.Send(user.Handle, to, subject, body); err != nil {
		s.Notice("Could not send your mail.")
		return
	}
	s.Print("\x1b[1;32m  Mail sent to " + to + "!\x1b[0m\r\n")
	s.Pause()
}

// mailOutbox lists the caller's sent mail as a table (# / To / Subject / Age).
func (b *board) mailOutbox(s *term.Session, user *store.User) {
	msgs, err := b.mail.Outbox(user.Handle)
	if err != nil {
		s.Notice("Could not load your sent mail.")
		return
	}
	b.screenHeader(s, "electronic mail \xfa outbox")
	s.Print("\x1b[1;30m   #  \x1b[0;37mTo              Subject                       \x1b[1;30mAge\x1b[0m\r\n")
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	if len(msgs) == 0 {
		s.Print("\x1b[0;37m  You haven't sent any mail.\x1b[0m\r\n")
	}
	for i, m := range msgs {
		s.Printf("  \x1b[1;33m%2d  \x1b[1;35m%-14s \x1b[0;37m%-28s \x1b[1;30m%s\x1b[0m\r\n",
			i+1, truncStr(m.To, 14), truncStr(m.Subject, 28), relTime(m.Sent))
	}
	s.Printf("\r\n\x1b[1;30m  %d %s sent.\x1b[0m\r\n", len(msgs), plural(len(msgs), "message", "messages"))
	s.Pause()
}
