package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// showMessage renders one message full-screen in the classic leet-board style:
// a double-line framed header carrying the message group, the From/To, the
// subject, and the date, then the body, then a navigation footer. The caller's
// read loop (readBoard) handles the keystrokes; this only paints the screen.
func (b *board) showMessage(s *term.Session, bd *store.Board, msgs []store.Message, i int, canPost bool) {
	const inner = 70 // printable columns between the box borders

	m := msgs[i]
	s.Print("\x1b[0m\x1b[2J\x1b[H")

	top := "\xc9" + strings.Repeat("\xcd", inner) + "\xbb"
	mid := "\xcc" + strings.Repeat("\xcd", inner) + "\xb9"
	bot := "\xc8" + strings.Repeat("\xcd", inner) + "\xbc"

	s.Printf("  \x1b[1;36m%s\x1b[0m\r\n", top)

	// Title bar: the message group (board) on the left, a "msg #N of T" counter
	// pinned to the right.
	counter := "msg #" + strconv.Itoa(i+1) + " of " + strconv.Itoa(len(msgs))
	group := truncStr(bd.Name, inner-len(counter)-4)
	gap := inner - 2 - len([]rune(group)) - len(counter)
	if gap < 1 {
		gap = 1
	}
	s.Printf("  \x1b[1;36m\xba \x1b[1;35m%s%s\x1b[1;33m%s \x1b[1;36m\xba\x1b[0m\r\n",
		group, strings.Repeat(" ", gap), counter)

	s.Printf("  \x1b[1;36m%s\x1b[0m\r\n", mid)

	// A labelled field row, the value clipped + padded to a fixed width so the
	// right border always lines up.
	row := func(label, val string) {
		v := []rune(val)
		if len(v) > 59 {
			v = v[:59]
		}
		valStr := string(v) + strings.Repeat(" ", 59-len(v))
		s.Printf("  \x1b[1;36m\xba \x1b[1;33m%-8s\x1b[1;30m: \x1b[1;37m%s\x1b[1;36m\xba\x1b[0m\r\n",
			label, valStr)
	}
	row("From", m.From)
	row("To", m.To)
	row("Subject", m.Subject)
	row("Date", m.Posted.Format("Mon 2006-01-02 15:04"))

	s.Printf("  \x1b[1;36m%s\x1b[0m\r\n\r\n", bot)

	// The body, plainly indented under the header.
	for _, ln := range strings.Split(m.Body, "\n") {
		s.Print("  \x1b[0;37m" + ln + "\x1b[0m\r\n")
	}

	// Navigation footer.
	s.Print("\r\n\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	reply := ""
	if canPost {
		reply = "  \x1b[0;37m[\x1b[1;37mR\x1b[0;37m]eply"
	}
	s.Printf("  \x1b[0;37m[\x1b[1;37mP\x1b[0;37m]rev  [\x1b[1;37mN\x1b[0;37m]ext%s  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;30m\xb7 \x1b[0;37mmsg \x1b[1;37m%d\x1b[0;37m/\x1b[1;37m%d \x1b[1;36m> \x1b[1;37m",
		reply, i+1, len(msgs))
	s.Flush()
}

// postReply composes a reply to m: addressed back to its sender, subject
// pre-filled with a "Re:" of the original (not doubled if it already has one).
func (b *board) postReply(s *term.Session, bd *store.Board, user *store.User, m store.Message) {
	subj := strings.TrimSpace(m.Subject)
	if !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	b.compose(s, bd, user, m.From, subj)
}
