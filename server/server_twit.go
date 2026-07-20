package main

import (
	"strconv"
	"strings"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// twitSet loads the caller's ignore list as a lower-cased set (nil on error,
// which just means nothing is hidden -- a filter failure never blanks a base).
func (b *board) twitSet(user *store.User) map[string]bool {
	set, err := b.st.TwitSet(user.ID)
	if err != nil {
		return nil
	}
	return set
}

// filterTwitMsgs drops messages whose sender is on the caller's ignore set.
// An empty/nil set returns the slice untouched (no allocation).
func filterTwitMsgs(msgs []store.Message, twits map[string]bool) []store.Message {
	if len(twits) == 0 {
		return msgs
	}
	out := msgs[:0:0]
	for _, m := range msgs {
		if !twits[strings.ToLower(m.From)] {
			out = append(out, m)
		}
	}
	return out
}

// filterTwitLiners drops wall lines whose author is ignored.
func filterTwitLiners(liners []store.Oneliner, twits map[string]bool) []store.Oneliner {
	if len(twits) == 0 {
		return liners
	}
	out := liners[:0:0]
	for _, l := range liners {
		if !twits[strings.ToLower(l.Author)] {
			out = append(out, l)
		}
	}
	return out
}

// twitSettings is the ignore-list editor on the settings screen: show the list,
// add a handle, remove a handle.
func (b *board) twitSettings(s *term.Session, user *store.User) {
	for {
		twits, _ := b.st.Twits(user.ID)

		s.Print("\x1b[0m\x1b[2J\x1b[H")
		s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m ignore list\x1b[0m\r\n", boardName)
		s.Print("\x1b[1;30m  Handles here have their posts and wall lines hidden from you.\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
		if len(twits) == 0 {
			s.Print("\x1b[0;37m  Your ignore list is empty.\x1b[0m\r\n")
		}
		for i, h := range twits {
			s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%s\x1b[0m\r\n", i+1, h)
		}

		s.Print("\r\n\x1b[0;37m  [\x1b[1;37mA\x1b[0;37m]dd  [\x1b[1;37mR\x1b[0;37m]emove #  [\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		line := strings.TrimSpace(s.ReadLine(8))
		switch {
		case line == "" || strings.EqualFold(line, "q"):
			return
		case strings.EqualFold(line, "a"):
			b.addTwit(s, user)
		default:
			// [R]emove #, "r N", or a bare number all remove that entry.
			idx := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "r"))
			if idx == "" {
				s.Print("\r\n\x1b[0;37m  Remove which #: \x1b[1;37m")
				s.Flush()
				idx = strings.TrimSpace(s.ReadLine(4))
			}
			n, err := strconv.Atoi(idx)
			if err != nil || n < 1 || n > len(twits) {
				s.Notice("No such number.")
				continue
			}
			if err := b.st.RemoveTwit(user.ID, twits[n-1]); err != nil {
				s.Notice("Could not remove that handle.")
				continue
			}
			s.Printf("\x1b[1;32m  No longer ignoring %s.\x1b[0m\r\n", twits[n-1])
			s.Pause()
		}
	}
}

// addTwit prompts for a handle and adds it to the caller's ignore list.
func (b *board) addTwit(s *term.Session, user *store.User) {
	s.Print("\r\n\x1b[0;37m  Handle to ignore: \x1b[1;37m")
	s.Flush()
	h := strings.TrimSpace(s.ReadLine(30))
	if h == "" {
		return
	}
	if strings.EqualFold(h, user.Handle) {
		s.Notice("You can't ignore yourself.")
		return
	}
	if err := b.st.AddTwit(user.ID, h); err != nil {
		s.Notice("Could not add that handle.")
		return
	}
	s.Printf("\x1b[1;32m  Ignoring %s.\x1b[0m\r\n", h)
	s.Pause()
}
