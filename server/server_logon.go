package main

import (
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// logon runs the between-the-door welcome sequence a caller sees after the
// matrix/login but before the main menu: a quick greeting with their last call,
// a "quick logon" escape hatch to skip straight to the menu, and otherwise the
// classic walk-through -- who's online, system info, the sysop's bulletins, and
// the wall. Each screen pauses on its own. A "quick logon" yes jumps to the
// menu; anything else takes the scenic route.
//
// user.LastCall / user.Calls still hold the PRIOR values here: runBoard records
// the new login against the database, not the in-memory struct, so this is the
// last time the caller was on and the count before tonight.
func (b *board) logon(s *term.Session, tok map[string]string, user *store.User) {
	b.screenHeader(s, "logon")

	s.Printf("\r\n  \x1b[1;37mWelcome back, \x1b[1;36m%s\x1b[1;37m.\x1b[0m\r\n\r\n", user.Handle)
	if user.LastCall.IsZero() {
		s.Print("  \x1b[1;30mLast on  \x1b[0;37m\xb7 \x1b[1;32mfirst call -- welcome aboard.\x1b[0m\r\n")
	} else {
		s.Printf("  \x1b[1;30mLast on  \x1b[0;37m\xb7 \x1b[1;37m%s \x1b[1;30m(%s)\x1b[0m\r\n",
			dateOr(user.LastCall), relTime(user.LastCall))
	}
	s.Printf("  \x1b[1;30mCall no. \x1b[0;37m\xb7 \x1b[1;37m#%d\x1b[0m\r\n", user.Calls+1)

	s.Print("\r\n\x1b[0;37m  Quick logon? \x1b[1;30m(\x1b[1;37mY\x1b[1;30m skips to the menu, \x1b[1;37mN\x1b[1;30m takes the tour) \x1b[1;36m[\x1b[1;37my/\x1b[1;33mN\x1b[1;36m] \x1b[1;37m")
	s.Flush()
	if k, ch := s.ReadKey(); k == term.KeyChar && lc(ch) == 'y' {
		s.Print("\x1b[0m\r\n")
		return
	}

	// The scenic route: who's on, the stats, the sysop's bulletins, the wall.
	b.whosOnline(s)
	b.sysInfo(s, tok, user)
	b.showBulletins(s)
	b.oneliners(s, user)
}
