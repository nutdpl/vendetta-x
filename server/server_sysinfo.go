package main

import (
	"strconv"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// sysInfo is the board's credits / stats screen: what this thing is, what it's
// built on, and the live counts behind it -- a key/value block in the same
// style as profile, capped with a short greets blurb.
func (b *board) sysInfo(s *term.Session, tok map[string]string, user *store.User) {
	b.screenHeader(s, "system info")

	row := func(label, val string) {
		s.Printf("  \x1b[1;36m%-12s\x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", label, val)
	}

	users, _ := b.st.Users()
	msgs, _ := b.st.RecentMessages(0)
	boards, _ := b.st.Boards()
	areas, _ := b.st.FileAreas()
	liners, _ := b.st.Oneliners(0)

	files := 0
	for _, a := range areas {
		fs, _ := b.st.Files(a.ID)
		files += len(fs)
	}

	row("Software", "Vendetta/X")
	row("Version", version)
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	row("Users", strconv.Itoa(len(users)))
	row("Messages", strconv.Itoa(len(msgs)))
	row("Files", strconv.Itoa(files))
	row("Boards", strconv.Itoa(len(boards)))
	row("File Areas", strconv.Itoa(len(areas)))
	row("Oneliners", strconv.Itoa(len(liners)))
	row("Nodes Online", strconv.Itoa(len(b.pres.list())))
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n\r\n")

	s.Print("\x1b[1;30m  One Go binary serving telnet + ssh + web off one SQLite spine.\x1b[0m\r\n")
	s.Print("\x1b[1;30m  Real .pp art, lightbar menus, bcrypt logins, a full-screen editor,\x1b[0m\r\n")
	s.Print("\x1b[1;30m  and a multi-node teleconference -- all reading the same dataset.\x1b[0m\r\n")
	s.Print("\x1b[1;30m  Greets to the scene. The scene never died -- it just got an IP.\x1b[0m\r\n")

	s.Pause()
}
