package main

import (
	"strings"
	"time"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

// beat is one paced line of a ceremony: print it, then pause (skippable).
type beat struct {
	text  string
	pause time.Duration
}

// play prints each beat in turn, pausing between them. A keypress during any
// pause skips the rest of the sequence (and is pushed back for what follows).
func play(s *term.Session, beats []beat) {
	for _, bt := range beats {
		s.Print(bt.text + "\r\n")
		if s.WaitKey(bt.pause) {
			return
		}
	}
}

// connect plays the carrier handshake a caller sees the instant the session
// comes up, before the login matrix: a short, paced modem sequence that sets the
// tone -- this is a board, not a web form. It is deliberately brief (~1s) and any
// keypress skips straight through, so it never gets in a regular's way. SSH
// callers get the same sequence (it just isn't skippable without a read
// deadline); the "modem" framing is pure theatre and all the sweeter for it.
func (b *board) connect(s *term.Session, tok map[string]string) {
	s.Print("\x1b[0m\x1b[2J\x1b[H\r\n")

	// Probe the terminal's charset before the show starts, so the handshake's
	// "negotiating" line reports the real result. Telnet only (it needs a read
	// deadline); the SSH face already decided from the client's TERM string.
	s.DetectCharset(400 * time.Millisecond)

	// The dialing handshake: one paced line at a time, each pause its own skip
	// point. A keypress jumps past the dialing straight to the splash.
	play(s, []beat{
		{"  \x1b[1;30minitializing tty1 ........ \x1b[0;32mok\x1b[0m", 240 * time.Millisecond},
		{"  \x1b[1;30mraising dtr .............. \x1b[0;32mok\x1b[0m", 200 * time.Millisecond},
		{"  \x1b[1;30mcarrier detect ........... \x1b[1;33mringing\x1b[0m", 360 * time.Millisecond},
		{"  \x1b[1;32m  CONNECT 57600\x1b[1;30m/ARQ/V90/LAPM/V42BIS\x1b[0m", 420 * time.Millisecond},
		{"  \x1b[1;30mnegotiating ansi-bbs ..... \x1b[0;36m" +
			strings.ToUpper(s.CharsetName()) + "\x1b[0m", 260 * time.Millisecond},
	})
	s.Sleep(180 * time.Millisecond)

	// The flagship loginscreen: the board's pride piece (the cyan VENDETTA art),
	// painted on line by line, gated on a keypress before the login matrix. Any
	// key pressed during the paint both finishes it and dismisses the splash.
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Reveal(b.art+"/loginscreen.ans", tok, 22*time.Millisecond)
	s.WaitAnyKey(8 * time.Second)
}

// welcomeNewUser is the arrival ceremony a brand-new caller sees the instant
// their account is created: a short "credentials issued / ACCESS GRANTED" beat,
// the entry-granted banner painted in, and the access they were just assigned
// printed out like a credential block. It turns a one-line "account created"
// into a moment worth remembering -- the new user's first taste of the board.
func (b *board) welcomeNewUser(s *term.Session, tok map[string]string, u *store.User, loc string) {
	tok["UH"] = u.Handle
	tok["UL"] = loc
	tok["UC"] = "1" // their very first call

	s.Print("\x1b[0m\x1b[2J\x1b[H\r\n")
	play(s, []beat{
		{"  \x1b[1;30mvalidating application .... \x1b[0;32mok\x1b[0m", 360 * time.Millisecond},
		{"  \x1b[1;30mallocating user record .... \x1b[0;32mok\x1b[0m", 320 * time.Millisecond},
		{"  \x1b[1;30missuing credentials ....... \x1b[0;32mok\x1b[0m", 320 * time.Millisecond},
		{"\r\n  \x1b[1;32m  ACCESS GRANTED\x1b[0m", 520 * time.Millisecond},
	})
	s.Sleep(220 * time.Millisecond)

	// The entry-granted banner (handle / location / first call), painted in.
	s.Print("\x1b[2J\x1b[H")
	s.Reveal(b.art+"/welcome.pp", tok, 18*time.Millisecond)

	// The access just granted, as a key/value credential line.
	s.Printf("\r\n  \x1b[1;30maccess \x1b[0;37m\xb7 \x1b[1;37mlevel %d\x1b[0m    \x1b[1;30mgroup \x1b[0;37m\xb7 \x1b[1;37m%s\x1b[0m\r\n",
		u.SL, u.Group)
	s.Print("\r\n  \x1b[0;36mYou're in. The board is yours -- explore.\x1b[0m\r\n")
	s.Pause()
}
