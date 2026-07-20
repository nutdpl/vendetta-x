package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/gfiles"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// gFiles is the G-Files library: text/info documents (rules, NFOs, scene docs)
// the caller can browse and read. It lists every doc as a table and lets the
// caller [R]ead one by number, showing a header and the wrapped body. (Named
// gFiles, not gfiles, so it doesn't collide with the board's b.gfiles field.)
func (b *board) gFiles(s *term.Session, tok map[string]string, user *store.User) {
	for {
		docs, err := b.gfiles.List("")
		if err != nil {
			s.Notice("Could not load the G-Files.")
			return
		}

		b.screenHeader(s, "g-files \xfa text library")
		s.Print("\x1b[1;30m   #  \x1b[0;37mTitle                        \x1b[1;30mCategory     Author      Age\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")

		if len(docs) == 0 {
			s.Print("\x1b[0;37m  The library is empty.\x1b[0m\r\n")
		}
		for i, g := range docs {
			s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-28s \x1b[1;35m%-12s \x1b[1;36m%-11s \x1b[1;30m%s\x1b[0m\r\n",
				i+1, truncStr(g.Title, 28), truncStr(g.Category, 12),
				truncStr(g.Author, 11), relTime(g.Added))
		}

		s.Printf("\r\n\x1b[1;30m  %d %s on file.\x1b[0m\r\n",
			len(docs), plural(len(docs), "document", "documents"))
		s.Print("\r\n\x1b[0;37m  [\x1b[1;37mR\x1b[0;37m]ead #  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		cmd := strings.TrimSpace(s.ReadLine(5))
		if cmd == "" {
			return
		}
		switch lc(cmd[0]) {
		case 'q':
			return
		case 'r':
			b.gfileRead(s, docs)
		default:
			// A bare number reads that document.
			if n, perr := strconv.Atoi(cmd); perr == nil && n >= 1 && n <= len(docs) {
				b.gfileShow(s, docs[n-1].ID)
			}
		}
	}
}

// gfileRead prompts for a document number (mirroring pickBase) and shows it.
func (b *board) gfileRead(s *term.Session, docs []gfiles.GFile) {
	s.Print("\r\n\x1b[0;37m  Document \x1b[1;37m#\x1b[0;37m (\x1b[1;37mQ\x1b[0;37m to quit) \x1b[1;36m> \x1b[1;37m")
	s.Flush()
	line := strings.TrimSpace(s.ReadLine(5))
	if line == "" || lc(line[0]) == 'q' {
		return
	}
	n, perr := strconv.Atoi(line)
	if perr != nil || n < 1 || n > len(docs) {
		return
	}
	b.gfileShow(s, docs[n-1].ID)
}

// gfileShow loads a document by id (with its body) and prints it: a header with
// title/category/author, a rule, then the body split on newlines.
func (b *board) gfileShow(s *term.Session, id int64) {
	g, err := b.gfiles.Get(id)
	if err != nil || g == nil {
		s.Notice("That document is gone.")
		return
	}
	b.screenHeader(s, "g-files \xfa "+g.Title)
	s.Printf("  \x1b[1;35m%s \x1b[1;30m\xb3 \x1b[1;36m%s \x1b[1;30m\xb3 \x1b[0;37mby %s\x1b[0m\r\n",
		truncStr(g.Category, 20), dateOr(g.Added), g.Author)
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	// Page the document so a long g-file doesn't scroll off a short terminal.
	if b.pageText(s, g.Body, bodyRows(s, gfileHeaderRows)) {
		s.Pause()
	}
}
