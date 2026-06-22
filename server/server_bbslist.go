package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/bbslist"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// bbsList is the BBS List: a directory of other boards in the classic
// "call these too" tradition. It lists the directory as a table and lets the
// caller [V]iew a board's full details or [A]dd one of their own.
func (b *board) bbsList(s *term.Session, tok map[string]string, user *store.User) {
	for {
		entries, err := b.bbslist.List()
		if err != nil {
			s.Notice("Could not load the BBS list.")
			return
		}

		b.screenHeader(s, "bbs list \xfa other boards")
		s.Print("\x1b[1;30m   #  \x1b[0;37mName               \x1b[1;30mAddress              Software        Sysop\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")

		if len(entries) == 0 {
			s.Print("\x1b[0;37m  No boards on file. Add the first one.\x1b[0m\r\n")
		}
		for i, e := range entries {
			s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-18s \x1b[0;37m%-20s \x1b[1;36m%-15s \x1b[1;35m%s\x1b[0m\r\n",
				i+1, truncStr(e.Name, 18), truncStr(e.Address, 20),
				truncStr(e.Software, 15), truncStr(e.Sysop, 12))
		}

		s.Printf("\r\n\x1b[1;30m  %d %s on file.\x1b[0m\r\n",
			len(entries), plural(len(entries), "board", "boards"))
		s.Print("\r\n\x1b[0;37m  [\x1b[1;37mV\x1b[0;37m]iew #  [\x1b[1;37mA\x1b[0;37m]dd  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		cmd := strings.TrimSpace(s.ReadLine(5))
		if cmd == "" {
			return
		}
		switch lc(cmd[0]) {
		case 'q':
			return
		case 'a':
			b.bbsAddEntry(s, user)
		case 'v':
			b.bbsView(s, entries)
		default:
			// A bare number is treated as a view shortcut.
			if n, perr := strconv.Atoi(cmd); perr == nil && n >= 1 && n <= len(entries) {
				b.bbsDetails(s, &entries[n-1])
			}
		}
	}
}

// bbsView prompts for a board number (mirroring pickBase) and shows its details.
func (b *board) bbsView(s *term.Session, entries []bbslist.Entry) {
	s.Print("\r\n\x1b[0;37m  Board \x1b[1;37m#\x1b[0;37m (\x1b[1;37mQ\x1b[0;37m to quit) \x1b[1;36m> \x1b[1;37m")
	s.Flush()
	line := strings.TrimSpace(s.ReadLine(5))
	if line == "" || lc(line[0]) == 'q' {
		return
	}
	n, perr := strconv.Atoi(line)
	if perr != nil || n < 1 || n > len(entries) {
		return
	}
	b.bbsDetails(s, &entries[n-1])
}

// bbsDetails shows one board's full record as a key/value block.
func (b *board) bbsDetails(s *term.Session, e *bbslist.Entry) {
	b.screenHeader(s, "bbs list \xfa "+e.Name)
	row := func(label, val string) {
		if val == "" {
			val = "--"
		}
		s.Printf("  \x1b[1;36m%-12s\x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", label, val)
	}
	row("Name", e.Name)
	row("Address", e.Address)
	row("Software", e.Software)
	row("Sysop", e.Sysop)
	row("Added", dateOr(e.Added))
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	if e.Desc != "" {
		s.Printf("  \x1b[0;37m%s\x1b[0m\r\n", e.Desc)
	}
	s.Pause()
}

// bbsAddEntry prompts for the fields of a new board and inserts it.
func (b *board) bbsAddEntry(s *term.Session, user *store.User) {
	b.screenHeader(s, "bbs list \xfa add a board")

	field := func(label string, max int) string {
		s.Printf("\x1b[0;37m  %-12s\x1b[1;36m> \x1b[1;37m", label)
		s.Flush()
		return strings.TrimSpace(s.ReadLine(max))
	}

	name := field("Name", 60)
	if name == "" {
		s.Notice("A name is required.")
		return
	}
	address := field("Address", 80)
	software := field("Software", 40)
	sysop := field("Sysop", 40)
	descr := field("Description", 120)

	if _, err := b.bbslist.Add(&bbslist.Entry{
		Name:     name,
		Address:  address,
		Software: software,
		Sysop:    sysop,
		Desc:     descr,
	}); err != nil {
		s.Notice("Could not add the board.")
		return
	}
	_ = user
	s.Print("\r\n\x1b[1;32m  Board added to the list.\x1b[0m\r\n")
	s.Pause()
}
