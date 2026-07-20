package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// readableBoardIDs returns the ids of every base the caller may read, plus a
// lookup from id to board -- the ACS-scoped set the personal scan and search
// restrict themselves to.
func (b *board) readableBoardIDs(user *store.User) ([]int64, map[int64]*store.Board) {
	subj := subjectOf(user)
	boards, err := b.st.Boards()
	if err != nil {
		return nil, nil
	}
	byID := make(map[int64]*store.Board, len(boards))
	var ids []int64
	for i := range boards {
		if acs.Eval(boards[i].ReadACS, subj) {
			ids = append(ids, boards[i].ID)
			byID[boards[i].ID] = &boards[i]
		}
	}
	return ids, byID
}

// personalScan lists the public posts addressed to the caller by name (replies
// to their posts, anything sent to their handle) across every base they can
// read, and opens any one in the reader. The classic "you have messages
// waiting" that isn't private mail.
func (b *board) personalScan(s *term.Session, user *store.User) {
	ids, byID := b.readableBoardIDs(user)
	hits, err := b.st.MessagesToHandle(user.Handle, ids, searchLimit)
	if err != nil {
		s.Notice("Could not scan for your messages.")
		return
	}
	if len(hits) == 0 {
		s.Notice("No public messages are addressed to you right now.")
		return
	}

	for {
		s.Print("\x1b[0m\x1b[2J\x1b[H")
		s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m messages to \x1b[1;37m%s\x1b[0m\r\n", boardName, user.Handle)
		s.Printf("\x1b[1;30m  %d %s addressed to you\x1b[0m\r\n", len(hits), plural(len(hits), "post", "posts"))
		s.Print("\r\n\x1b[1;30m   #  \x1b[0;37mBase          From          Subject\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
		for i, m := range hits {
			base := "?"
			if bd := byID[m.BoardID]; bd != nil {
				base = bd.Name
			}
			s.Printf("  \x1b[1;33m%2d  \x1b[0;36m%-12s \x1b[1;37m%-12s \x1b[0;37m%s\x1b[0m\r\n",
				i+1, truncStr(base, 12), truncStr(m.From, 12), truncStr(m.Subject, 30))
		}

		s.Print("\r\n\x1b[0;37m  [\x1b[1;37m#\x1b[0;37m] read  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		line := strings.TrimSpace(s.ReadLine(8))
		if line == "" || strings.EqualFold(line, "q") {
			return
		}
		n, convErr := strconv.Atoi(line)
		if convErr != nil || n < 1 || n > len(hits) {
			s.Notice("No such number.")
			continue
		}
		b.readSearchHit(s, user, byID[hits[n-1].BoardID], hits[n-1].ID)
	}
}
