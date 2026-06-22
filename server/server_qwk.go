package main

import (
	"strconv"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// qwk is the QWK offline-mail screen. Classic QWK bundles new messages into a
// packet you read offline; on Vendetta/X the web face is your offline reader, so
// this screen is an honest "packet summary": your private-mail and message
// counts plus a digest of the latest posts across the bases you can read.
func (b *board) qwk(s *term.Session, tok map[string]string, user *store.User) {
	b.screenHeader(s, "qwk offline mail")

	unread := 0
	if b.mail != nil {
		if n, err := b.mail.UnreadCount(user.Handle); err == nil {
			unread = n
		}
	}
	allPub, _ := b.st.RecentMessages(0)

	kv := func(label, val string) {
		s.Printf("  \x1b[1;36m%-16s\x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", label, val)
	}
	kv("Private mail", strconv.Itoa(unread)+" unread")
	kv("Public messages", strconv.Itoa(len(allPub))+" on the boards")
	kv("Last call", dateOr(user.LastCall))

	s.Print("\r\n\x1b[0;37m  A QWK packet bundles new messages into a file you read offline. A real\r\n")
	s.Print("  \x1b[1;37mVENDX.QWK\x1b[0;37m packet -- and \x1b[1;37m.REP\x1b[0;37m reply upload -- live on the web face at\r\n")
	s.Print("  \x1b[1;36m/qwk\x1b[0;37m, since this ANSI wire has no file transfer. Dial in over HTTP to\r\n")
	s.Print("  download the packet or post your offline replies back to the boards.\x1b[0m\r\n\r\n")

	// Digest of the latest posts across readable bases.
	subj := subjectOf(user)
	boards, _ := b.st.Boards()
	readable := map[int64]string{}
	for i := range boards {
		if acs.Eval(boards[i].ReadACS, subj) {
			readable[boards[i].ID] = boards[i].Name
		}
	}
	s.Print("\x1b[1;30m  packet digest \xfa latest posts\x1b[0m\r\n")
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	shown := 0
	for _, m := range allPub {
		base, ok := readable[m.BoardID]
		if !ok {
			continue
		}
		s.Printf("  \x1b[1;33m%-14s \x1b[1;37m%-28s \x1b[1;36m%-12s \x1b[0;37m%s\x1b[0m\r\n",
			truncStr(base, 14), truncStr(m.Subject, 28), truncStr(m.From, 12), relTime(m.Posted))
		shown++
		if shown >= 15 {
			break
		}
	}
	if shown == 0 {
		s.Print("\x1b[0;37m  Nothing in the packet yet.\x1b[0m\r\n")
	}
	s.Pause()
}
