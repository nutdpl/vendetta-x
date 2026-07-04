package main

import (
	"strings"
	"time"

	"vendetta-x/server/internal/sanitize"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// pageSysop is the classic doorbell: the caller states their business, the
// board runs the paging beat, and the page lands in the operator's mailbox
// (there is no local console bell to ring on a cloud box -- mail IS the
// bell that always works). The wording tells the caller whether an operator
// is on the board right now, and points them at the teleconference if so.
func (b *board) pageSysop(s *term.Session, user *store.User) {
	b.screenHeader(s, "page the sysop")

	target := b.sysopAccount()
	if target == nil {
		s.Notice("No operator account is configured on this board.")
		return
	}

	s.Print("\r\n\x1b[0;37m  Why are you paging? \x1b[1;30m(a line; enter alone cancels)\x1b[0m\r\n")
	s.Print("\x1b[1;36m  > \x1b[1;37m")
	s.Flush()
	reason := sanitize.Line(strings.TrimSpace(s.ReadLine(60)))
	if reason == "" {
		s.Print("\x1b[1;33m  Page cancelled.\x1b[0m\r\n")
		s.Pause()
		return
	}

	// The paging beat: a few unhurried rings, skippable like every other
	// animation on the board (WaitKey returns early on a keypress).
	s.Print("\r\n\x1b[1;35m  paging " + target.Handle + " ")
	s.Flush()
	for i := 0; i < 6; i++ {
		if s.WaitKey(350 * time.Millisecond) {
			break
		}
		s.Print("\x1b[1;30m\xfa\x1b[1;35m\xf9 ")
		s.Flush()
	}
	s.Print("\x1b[0m\r\n\r\n")

	body := "PAGE from " + user.Handle + " at " + time.Now().Format("15:04") +
		"\n\n" + reason
	if err := b.mail.Send(user.Handle, target.Handle, "PAGE: "+reason, body); err != nil {
		s.Notice("Could not deliver your page.")
		return
	}

	if b.sysopOnline() {
		s.Print("\x1b[0;37m  An operator is \x1b[1;32mon the board right now\x1b[0;37m -- your page is in their\r\n" +
			"  mailbox. Try the \x1b[1;37mteleConference\x1b[0;37m if you want to talk live.\x1b[0m\r\n")
	} else {
		s.Print("\x1b[0;37m  The operator isn't around. Your page is in their mailbox --\r\n" +
			"  they'll see it the moment they next call.\x1b[0m\r\n")
	}
	s.Pause()
}

// sysopAccount returns the account pages are delivered to: the seeded
// "sysop" handle when it exists, else the first privileged user.
func (b *board) sysopAccount() *store.User {
	if u, err := b.st.UserByHandle("sysop"); err == nil && u != nil {
		return u
	}
	users, err := b.st.Users()
	if err != nil {
		return nil
	}
	for i := range users {
		if users[i].Privileged() {
			return &users[i]
		}
	}
	return nil
}

// sysopOnline reports whether any currently-connected caller is privileged.
// Presence entries are "handle@host"; the handle is looked up fresh so a
// mid-session validation counts.
func (b *board) sysopOnline() bool {
	for _, w := range b.pres.list() {
		handle, _, _ := strings.Cut(w, "@")
		if u, err := b.st.UserByHandle(handle); err == nil && u != nil && u.Privileged() {
			return true
		}
	}
	return false
}
