package main

import (
	"strings"

	"vendetta-x/server/internal/auth"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// settings is the user self-service screen: edit your own profile fields and
// change your password, all writing through the shared store. It mirrors the
// profile key/value look, then drops to a command prompt.
func (b *board) settings(s *term.Session, tok map[string]string, user *store.User) {
	for {
		// re-read so the block always reflects the latest stored values.
		if u, err := b.st.UserByHandle(user.Handle); err == nil && u != nil {
			user = u
		}

		b.screenHeader(s, "settings")
		row := func(label, val string) {
			s.Printf("  \x1b[1;36m%-12s\x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", label, val)
		}
		row("Handle", user.Handle)
		row("Real Name", user.RealName)
		row("Location", user.Location)
		row("Email", user.Email)
		row("Tagline", user.Tagline)
		row("Birthday", orNotSet(user.Birthday))
		row("Expert", onOff(user.Expert))
		row("Clock", clockLabel(user.Clock12))
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
		s.Print("\r\n\x1b[0;37m  [\x1b[1;37mN\x1b[0;37m]ame  [\x1b[1;37mL\x1b[0;37m]ocation  [\x1b[1;37mE\x1b[0;37m]mail  [\x1b[1;37mT\x1b[0;37m]agline  [\x1b[1;37mB\x1b[0;37m]irthday\x1b[0m\r\n")
		s.Print("\x1b[0;37m  [\x1b[1;37mX\x1b[0;37m]pert mode  [\x1b[1;37mK\x1b[0;37m] clock  [\x1b[1;37mP\x1b[0;37m]assword  [\x1b[1;37mQ\x1b[0;37m]uit\x1b[0m\r\n")
		s.Print("\x1b[0;37m  Choice \x1b[1;36m> \x1b[1;37m")
		s.Flush()

		k, ch := s.ReadKey()
		if k == term.KeyEOF {
			return
		}
		if k != term.KeyChar {
			continue
		}
		s.Print("\r\n")

		switch lc(ch) {
		case 'n':
			s.Print("\x1b[0;37m  New real name: \x1b[1;37m")
			s.Flush()
			val := strings.TrimSpace(s.ReadLine(30))
			if err := b.st.UpdateProfile(user.ID, val, user.Email, user.Location, user.Tagline); err != nil {
				s.Notice("Could not update your real name.")
				continue
			}
			s.Print("\x1b[1;32m  Saved.\x1b[0m\r\n")
			s.Pause()
		case 'l':
			s.Print("\x1b[0;37m  New location: \x1b[1;37m")
			s.Flush()
			val := strings.TrimSpace(s.ReadLine(30))
			if err := b.st.UpdateProfile(user.ID, user.RealName, user.Email, val, user.Tagline); err != nil {
				s.Notice("Could not update your location.")
				continue
			}
			s.Print("\x1b[1;32m  Saved.\x1b[0m\r\n")
			s.Pause()
		case 'e':
			s.Print("\x1b[0;37m  New email: \x1b[1;37m")
			s.Flush()
			val := strings.TrimSpace(s.ReadLine(60))
			if err := b.st.UpdateProfile(user.ID, user.RealName, val, user.Location, user.Tagline); err != nil {
				s.Notice("Could not update your email.")
				continue
			}
			s.Print("\x1b[1;32m  Saved.\x1b[0m\r\n")
			s.Pause()
		case 't':
			s.Print("\x1b[0;37m  New tagline: \x1b[1;37m")
			s.Flush()
			val := strings.TrimSpace(s.ReadLine(60))
			if err := b.st.UpdateProfile(user.ID, user.RealName, user.Email, user.Location, val); err != nil {
				s.Notice("Could not update your tagline.")
				continue
			}
			s.Print("\x1b[1;32m  Saved.\x1b[0m\r\n")
			s.Pause()
		case 'b':
			s.Print("\x1b[0;37m  Birthday \x1b[1;30m(MM-DD, blank to clear)\x1b[0;37m: \x1b[1;37m")
			s.Flush()
			val := strings.TrimSpace(s.ReadLine(10))
			if err := b.st.SetBirthday(user.ID, val); err != nil {
				s.Notice("That's not a date I recognize -- try MM-DD, like 07-12.")
				continue
			}
			s.Print("\x1b[1;32m  Saved.\x1b[0m\r\n")
			s.Pause()
		case 'x':
			if err := b.st.SetPrefs(user.ID, !user.Expert, user.Clock12); err != nil {
				s.Notice("Could not update expert mode.")
				continue
			}
			s.Printf("\x1b[1;32m  Expert mode %s.\x1b[0m\r\n", onOff(!user.Expert))
			s.Pause()
		case 'k':
			if err := b.st.SetPrefs(user.ID, user.Expert, !user.Clock12); err != nil {
				s.Notice("Could not update your clock.")
				continue
			}
			s.Printf("\x1b[1;32m  Clock set to %s.\x1b[0m\r\n", clockLabel(!user.Clock12))
			s.Pause()
		case 'p':
			b.changePassword(s, user)
		case 'q':
			return
		}
	}
}

// orNotSet returns s, or a dim "(not set)" marker when it's empty -- for the
// settings rows a caller hasn't filled in yet.
func orNotSet(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(not set)"
	}
	return s
}

// onOff renders a boolean toggle as "on"/"off" for the settings rows.
func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// clockLabel names a caller's clock preference.
func clockLabel(clock12 bool) string {
	if clock12 {
		return "12-hour"
	}
	return "24-hour"
}

// changePassword runs the password change flow on the settings screen: verify
// the current password (unless none is on file), then take + confirm a new one.
func (b *board) changePassword(s *term.Session, user *store.User) {
	if user.Password != "" {
		s.Print("\x1b[0;37m  Current password: \x1b[1;37m")
		s.Flush()
		old := s.ReadPassword(40)
		if !auth.Verify(user.Password, old) {
			s.Notice("Current password is wrong.")
			return
		}
	}
	s.Print("\x1b[0;37m  New password: \x1b[1;37m")
	s.Flush()
	pw := s.ReadPassword(40)
	if pw == "" {
		s.Notice("Password unchanged -- empty password.")
		return
	}
	s.Print("\x1b[0;37m  Verify: \x1b[1;37m")
	s.Flush()
	pw2 := s.ReadPassword(40)
	if pw != pw2 {
		s.Notice("Passwords did not match.")
		return
	}
	hash, err := auth.Hash(pw)
	if err != nil {
		s.Notice("Could not set password.")
		return
	}
	if err := b.st.SetPassword(user.ID, hash); err != nil {
		s.Notice("Could not save password.")
		return
	}
	user.Password = hash
	s.Print("\x1b[1;32m  Password changed.\x1b[0m\r\n")
	s.Pause()
}
