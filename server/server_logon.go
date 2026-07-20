package main

import (
	"time"

	"vendetta-x/server/internal/acs"
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
			dateOrClock(user.LastCall, user.Clock12), relTime(user.LastCall))
	}
	s.Printf("  \x1b[1;30mCall no. \x1b[0;37m\xb7 \x1b[1;37m#%d\x1b[0m\r\n", user.Calls+1)

	// The front-porch numbers a regular actually wants on arrival: what's new
	// since their read pointers, and whether anyone wrote to them directly.
	if counts, err := b.st.UnreadCounts(user.ID); err == nil {
		subj := subjectOf(user)
		boards, _ := b.st.Boards()
		total, bases := 0, 0
		for i := range boards {
			if n := counts[boards[i].ID]; n > 0 && acs.Eval(boards[i].ReadACS, subj) {
				total += n
				bases++
			}
		}
		if total > 0 {
			s.Printf("  \x1b[1;30mNew msgs \x1b[0;37m\xb7 \x1b[1;36m%d\x1b[0;37m in %d %s \x1b[1;30m-- [N]ew scan reads them\x1b[0m\r\n",
				total, bases, plural(bases, "base", "bases"))
		} else {
			s.Print("  \x1b[1;30mNew msgs \x1b[0;37m\xb7 \x1b[1;30mnone -- all caught up\x1b[0m\r\n")
		}
	}
	if b.st.FeatureEnabled("email") {
		if n, err := b.mail.UnreadCount(user.Handle); err == nil && n > 0 {
			s.Printf("  \x1b[1;30mMail     \x1b[0;37m\xb7 \x1b[1;33m%d unread\x1b[0m\r\n", n)
		}
	}

	// The rest of the since-your-last-call digest: fresh files (in areas the
	// caller can see) and fresh blood. First-time callers skip it -- on call
	// one, everything is new.
	if !user.LastCall.IsZero() {
		if counts, err := b.st.FileCountsAfter(user.LastCall); err == nil && len(counts) > 0 {
			total := 0
			if areas, err := b.st.FileAreas(); err == nil {
				subj := subjectOf(user)
				for i := range areas {
					if acs.Eval(areas[i].ACS, subj) {
						total += counts[areas[i].ID]
					}
				}
			}
			if total > 0 {
				s.Printf("  \x1b[1;30mFiles    \x1b[0;37m\xb7 \x1b[1;36m%d\x1b[0;37m new since your last call \x1b[1;30m-- [N]ew files lists them\x1b[0m\r\n", total)
			}
		}
		if n, err := b.st.NewUsersSince(user.LastCall); err == nil && n > 0 {
			s.Printf("  \x1b[1;30mNew blood\x1b[0;37m\xb7 \x1b[1;37m%d\x1b[0;37m %s joined since you were on\x1b[0m\r\n",
				n, plural(n, "caller", "callers"))
		}
	}

	// Public posts addressed to the caller by name (replies, direct posts) that
	// they haven't caught up on -- offered inline, the classic "you have
	// messages waiting; read them now?"
	if ids, _ := b.readableBoardIDs(user); len(ids) > 0 {
		if n, err := b.st.UnreadToHandle(user.ID, user.Handle, ids); err == nil && n > 0 {
			s.Printf("  \x1b[1;30mTo you   \x1b[0;37m\xb7 \x1b[1;33m%d\x1b[0;37m %s addressed to you \x1b[1;30m-- read now? \x1b[1;36m[\x1b[1;37my\x1b[1;36m/\x1b[1;33mN\x1b[1;36m] \x1b[1;37m",
				n, plural(n, "post", "posts"))
			s.Flush()
			k, ch := s.ReadKey()
			s.Print("\r\n")
			if k == term.KeyChar && lc(ch) == 'y' {
				b.personalScan(s, user)
				b.screenHeader(s, "logon") // repaint after the scan takes over the screen
			}
		}
	}

	// A little board culture: ring the caller's birthday and their board
	// anniversary (years since first call) when today is the day.
	now := time.Now()
	if isBirthdayToday(user.Birthday, now) {
		s.Printf("  \x1b[1;35mBirthday \x1b[0;37m\xb7 \x1b[1;33m-!- happy birthday, %s! -!-\x1b[0m\r\n", user.Handle)
	}
	if y := anniversaryYears(user.FirstCall, now); y > 0 {
		s.Printf("  \x1b[1;35mAnniv.   \x1b[0;37m\xb7 \x1b[1;36m%d %s on the board today \x1b[1;30m-- thanks for still calling.\x1b[0m\r\n",
			y, plural(y, "year", "years"))
	}

	// Expert-mode callers know the board; skip straight to the menu without the
	// quick-logon prompt or the scenic tour.
	if user.Expert {
		s.Print("\x1b[0m\r\n")
		s.Flush()
		return
	}

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
