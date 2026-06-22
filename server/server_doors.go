package main

import (
	"math/rand"
	"strconv"
	"strings"

	"vendetta-x/server/internal/door"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// doors is the door menu. It lists the two native games first, then any
// sysop-configured enabled external doors (DOSBox-hosted DOS games or native
// binaries). Launching a configured door writes its drop file, execs the
// command, and bridges the caller's terminal to the process. A misconfigured or
// absent door degrades gracefully -- it never hangs or crashes the session.
func (b *board) doors(s *term.Session, tok map[string]string, user *store.User) {
	for {
		// Load enabled external doors fresh each loop so sysop edits show up.
		var ext []door.Door
		if b.doorStore != nil {
			if ds, err := b.doorStore.Enabled(); err == nil {
				ext = ds
			}
		}

		b.screenHeader(s, "doors")
		s.Print("\x1b[1;30m   #  \x1b[0;37mDoor\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
		s.Print("  \x1b[1;33m 1  \x1b[1;37mGuess the Vault   \x1b[1;30m\xfa crack the 3-digit vault code\x1b[0m\r\n")
		s.Print("  \x1b[1;33m 2  \x1b[1;37mDice Duel         \x1b[1;30m\xfa roll against the house\x1b[0m\r\n")
		for i, d := range ext {
			label := d.Name
			if len(label) > 17 {
				label = label[:17]
			}
			desc := d.Description
			if desc != "" {
				s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-17s \x1b[1;30m\xfa %s\x1b[0m\r\n", i+3, label, desc)
			} else {
				s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-17s\x1b[0m\r\n", i+3, label)
			}
		}
		if len(ext) == 0 {
			s.Print("\r\n\x1b[1;30m  DOSBox-hosted doors (LORD, TradeWars, ...) are configured by the sysop.\x1b[0m\r\n")
		} else {
			s.Printf("\r\n\x1b[1;30m  %d external %s configured by the sysop.\x1b[0m\r\n",
				len(ext), plural(len(ext), "door", "doors"))
		}
		s.Print("\r\n\x1b[0;37m  Door \x1b[1;37m#\x1b[0;37m (\x1b[1;37mQ\x1b[0;37m to quit) \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		line := strings.TrimSpace(s.ReadLine(5))
		if line == "" || lc(line[0]) == 'q' {
			return
		}
		switch line {
		case "1":
			b.doorGuessVault(s, user)
		case "2":
			b.doorDiceDuel(s, user)
		default:
			n, err := strconv.Atoi(line)
			if err != nil || n < 3 || n-3 >= len(ext) {
				continue
			}
			b.doorRun(s, user, ext[n-3])
		}
	}
}

// doorRun launches a sysop-configured external door and bridges the caller's
// raw terminal to it. Any failure (not configured, binary absent, exec error)
// shows a clean notice and returns to the menu -- never a hang or panic.
func (b *board) doorRun(s *term.Session, user *store.User, d door.Door) {
	caller := door.Caller{
		Node:        1,
		Handle:      user.Handle,
		RealName:    user.RealName,
		SL:          user.SL,
		MinutesLeft: 60,
		Emulation:   1,
		Baud:        38400,
	}
	sys := door.System{
		Name:  b.siteName(),
		Sysop: b.st.Setting("board.sysop", "SysOp"),
	}
	b.screenHeader(s, lc8(d.Name))
	s.Printf("\x1b[0;37m  Launching \x1b[1;37m%s\x1b[0;37m...\x1b[0m\r\n", d.Name)
	s.Flush()
	if err := d.Run(caller, sys, s.Raw()); err != nil {
		s.Notice("That door isn't available right now.")
		return
	}
	// Door exited cleanly; reset the terminal and pause back to the menu.
	s.Print("\x1b[0m\r\n")
	s.Pause()
}

// lc8 lowercases an ASCII string for the screen header (titles are shown lower).
func lc8(str string) string { return strings.ToLower(str) }

// doorGuessVault: crack a hidden 3-digit code (000-999) with high/low hints.
func (b *board) doorGuessVault(s *term.Session, user *store.User) {
	code := rand.Intn(1000)
	const maxTries = 10
	b.screenHeader(s, "guess the vault")
	s.Print("\x1b[0;37m  The vault holds a 3-digit code (000-999). You have ")
	s.Printf("\x1b[1;37m%d\x1b[0;37m tries.\x1b[0m\r\n\r\n", maxTries)

	for try := 1; try <= maxTries; try++ {
		s.Printf("\x1b[1;30m  try %d/%d  \x1b[0;37myour guess \x1b[1;36m> \x1b[1;37m", try, maxTries)
		s.Flush()
		raw := strings.TrimSpace(s.ReadLine(4))
		if raw == "" {
			s.Print("\x1b[1;33m  Walked away from the vault.\x1b[0m\r\n")
			s.Pause()
			return
		}
		g, err := strconv.Atoi(raw)
		if err != nil || g < 0 || g > 999 {
			s.Print("\x1b[1;31m  000-999 only.\x1b[0m\r\n")
			try--
			continue
		}
		switch {
		case g == code:
			s.Printf("\r\n\x1b[1;32m  *CLICK* The vault swings open in %d %s. Nicely done, %s.\x1b[0m\r\n",
				try, plural(try, "try", "tries"), user.Handle)
			s.Pause()
			return
		case g < code:
			s.Print("\x1b[1;36m  Too low.\x1b[0m\r\n")
		default:
			s.Print("\x1b[1;35m  Too high.\x1b[0m\r\n")
		}
	}
	s.Printf("\r\n\x1b[1;31m  Out of tries. The code was \x1b[1;37m%03d\x1b[1;31m. The vault stays shut.\x1b[0m\r\n", code)
	s.Pause()
}

// doorDiceDuel: best of one -- you and the house each roll 2d6; high roll wins.
func (b *board) doorDiceDuel(s *term.Session, user *store.User) {
	b.screenHeader(s, "dice duel")
	s.Print("\x1b[0;37m  Two dice, you against the house. Press a key to roll...\x1b[0m\r\n")
	s.Pause()

	yA, yB := rand.Intn(6)+1, rand.Intn(6)+1
	hA, hB := rand.Intn(6)+1, rand.Intn(6)+1
	you, house := yA+yB, hA+hB

	s.Printf("\r\n  \x1b[1;36m%-8s\x1b[1;30m\xb3 \x1b[1;37m%d %d \x1b[1;30m= \x1b[1;37m%d\x1b[0m\r\n", user.Handle, yA, yB, you)
	s.Printf("  \x1b[1;35m%-8s\x1b[1;30m\xb3 \x1b[1;37m%d %d \x1b[1;30m= \x1b[1;37m%d\x1b[0m\r\n\r\n", "house", hA, hB, house)
	switch {
	case you > house:
		s.Print("\x1b[1;32m  You take the pot. The house tips its hat.\x1b[0m\r\n")
	case you < house:
		s.Print("\x1b[1;31m  The house wins this one. Run it back?\x1b[0m\r\n")
	default:
		s.Print("\x1b[1;33m  A push. Nobody blinks.\x1b[0m\r\n")
	}
	s.Pause()
}
