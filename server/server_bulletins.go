package main

import (
	"strings"

	"vendetta-x/server/internal/term"
)

// maxLogonBulletins caps how many of the latest bulletins are shown at logon so
// the display stays to a clean single screen.
const maxLogonBulletins = 5

// showBulletins displays the latest bulletins to the caller during the logon
// sequence: each one's title (bright), its date and author (dim), then the body
// indented. It caps at the most recent few so it stays a single screen.
func (b *board) showBulletins(s *term.Session) {
	b.screenHeader(s, "bulletins")

	if b.bulletins == nil {
		s.Print("\r\n\x1b[0;37m  No bulletins right now.\x1b[0m\r\n")
		s.Pause()
		return
	}

	all, err := b.bulletins.List()
	if err != nil {
		s.Print("\r\n\x1b[0;37m  No bulletins right now.\x1b[0m\r\n")
		s.Pause()
		return
	}
	if len(all) == 0 {
		s.Print("\r\n\x1b[0;37m  No bulletins right now.\x1b[0m\r\n")
		s.Pause()
		return
	}
	if len(all) > maxLogonBulletins {
		all = all[:maxLogonBulletins]
	}

	for i, bl := range all {
		if i > 0 {
			s.Print("\r\n")
		}
		s.Printf("  \x1b[1;37m%s\x1b[0m\r\n", truncStr(bl.Title, 60))
		s.Printf("  \x1b[1;30m%s \xb7 %s \x1b[0;30m(%s)\x1b[0m\r\n",
			dateOr(bl.Posted), bl.Author, relTime(bl.Posted))
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
		for _, line := range strings.Split(bl.Body, "\n") {
			s.Printf("  \x1b[0;37m%s\x1b[0m\r\n", line)
		}
	}

	s.Pause()
}
