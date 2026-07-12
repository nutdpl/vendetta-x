package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// searchLimit caps how many hits a single search returns on the terminal, so a
// broad term can't paint an unbounded list.
const searchLimit = 200

// searchMessages runs a board-wide message search on the terminal face: it
// takes a term (prompting if preset is empty), searches subject/body/author
// across only the bases this caller can read, lists the hits Iniquity-style
// (Num / base / from / subject / date), and opens any hit in its own base's
// reader on select. ACS scoping happens here -- the store only sees the
// readable id set.
func (b *board) searchMessages(s *term.Session, user *store.User, preset string) {
	query := preset
	if query == "" {
		s.Print("\r\n\x1b[0;37m  Search messages for \x1b[1;30m(subject, body, or handle)\x1b[0;37m: \x1b[1;37m")
		s.Flush()
		query = strings.TrimSpace(s.ReadLine(60))
	}
	if query == "" {
		return
	}

	subj := subjectOf(user)
	boards, err := b.st.Boards()
	if err != nil {
		s.Notice("Could not load boards.")
		return
	}
	byID := map[int64]*store.Board{}
	var ids []int64
	for i := range boards {
		if acs.Eval(boards[i].ReadACS, subj) {
			ids = append(ids, boards[i].ID)
			byID[boards[i].ID] = &boards[i]
		}
	}

	hits, err := b.st.SearchMessages(query, ids, searchLimit)
	if err != nil {
		s.Notice("Search failed.")
		return
	}
	if len(hits) == 0 {
		s.Notice("No messages match \"" + query + "\".")
		return
	}

	for {
		s.Print("\x1b[0m\x1b[2J\x1b[H")
		s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m search \x1b[1;30m\xfa\x1b[1;37m %s\x1b[0m\r\n", boardName, truncStr(query, 30))
		s.Printf("\x1b[1;30m  %d %s across your bases\x1b[0m\r\n", len(hits), plural(len(hits), "match", "matches"))
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
			s.Notice("No such result number.")
			continue
		}
		b.readSearchHit(s, user, byID[hits[n-1].BoardID], hits[n-1].ID)
	}
}

// readSearchHit opens a single search hit in its own base's reader, positioned
// on the matched message, with the full base loaded so thread walking and
// prev/next work exactly as they do from the base itself.
func (b *board) readSearchHit(s *term.Session, user *store.User, bd *store.Board, msgID int64) {
	if bd == nil {
		s.Notice("That base is no longer available.")
		return
	}
	msgs, err := b.st.Messages(bd.ID, 0)
	if err != nil || len(msgs) == 0 {
		s.Notice("Could not load that message.")
		return
	}
	// Messages is newest-first; find the hit's index.
	start := 0
	for i := range msgs {
		if msgs[i].ID == msgID {
			start = i
			break
		}
	}
	canPost := acs.Eval(bd.PostACS, subjectOf(user))
	b.runReader(s, bd, user, msgs, start, canPost)
}

// searchFiles runs a board-wide file search on the terminal face: it takes a
// term (prompting if preset is empty), searches filename/description/uploader
// across only the areas this caller can access, lists the hits (Num / filename
// / size / area / description), and downloads any hit over ZMODEM on select.
func (b *board) searchFiles(s *term.Session, user *store.User, preset string) {
	query := preset
	if query == "" {
		s.Print("\r\n\x1b[0;37m  Find files matching \x1b[1;30m(name, description, or uploader)\x1b[0;37m: \x1b[1;37m")
		s.Flush()
		query = strings.TrimSpace(s.ReadLine(60))
	}
	if query == "" {
		return
	}

	subj := subjectOf(user)
	areas, err := b.st.FileAreas()
	if err != nil {
		s.Notice("Could not load file areas.")
		return
	}
	areaName := map[int64]string{}
	var ids []int64
	for i := range areas {
		if acs.Eval(areas[i].ACS, subj) {
			ids = append(ids, areas[i].ID)
			areaName[areas[i].ID] = areas[i].Name
		}
	}

	hits, err := b.st.SearchFiles(query, ids, searchLimit)
	if err != nil {
		s.Notice("Search failed.")
		return
	}
	if len(hits) == 0 {
		s.Notice("No files match \"" + query + "\".")
		return
	}

	for {
		s.Print("\x1b[0m\x1b[2J\x1b[H")
		s.Printf("\x1b[1;35m  %s \x1b[1;30m\xfa\x1b[0;36m find \x1b[1;30m\xfa\x1b[1;37m %s\x1b[0m\r\n", boardName, truncStr(query, 30))
		s.Printf("\x1b[1;30m  %d %s across your areas\x1b[0m\r\n", len(hits), plural(len(hits), "match", "matches"))
		s.Print("\r\n\x1b[1;35m   #  \x1b[0;36mFilename             \x1b[1;30m   Size  Area          Description\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
		for i, f := range hits {
			s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-20s \x1b[0;37m%7s  \x1b[0;36m%-12s \x1b[1;30m%s\x1b[0m\r\n",
				i+1, truncStr(f.Filename, 20), sizeStr(f.Size), truncStr(areaName[f.AreaID], 12), truncStr(f.Desc, 22))
		}
		if b.ratioEnabled() && !b.ratioExempt(user) {
			s.Printf("\r\n\x1b[1;30m  %s credit left \xfa upload to earn more.\x1b[0m\r\n",
				sizeStr(b.downloadAllowance(user)))
		}

		s.Print("\r\n\x1b[0;37m  [\x1b[1;37m#\x1b[0;37m] download  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		line := strings.TrimSpace(s.ReadLine(8))
		if line == "" || strings.EqualFold(line, "q") {
			return
		}
		n, convErr := strconv.Atoi(line)
		if convErr != nil || n < 1 || n > len(hits) {
			s.Notice("No such result number.")
			continue
		}
		b.downloadFile(s, user, &hits[n-1])
		s.Pause()
	}
}
