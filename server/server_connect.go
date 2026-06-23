package main

import (
	"time"

	"vendetta-x/server/internal/term"
)

// connect plays the carrier handshake a caller sees the instant the session
// comes up, before the login matrix: a short, paced modem sequence that sets the
// tone -- this is a board, not a web form. It is deliberately brief (~1s) and any
// keypress skips straight through, so it never gets in a regular's way. SSH
// callers get the same sequence (it just isn't skippable without a read
// deadline); the "modem" framing is pure theatre and all the sweeter for it.
func (b *board) connect(s *term.Session) {
	s.Print("\x1b[0m\x1b[2J\x1b[H\r\n")

	// One paced line at a time. Each pause is its own skip point.
	type beat struct {
		text  string
		pause time.Duration
	}
	beats := []beat{
		{"  \x1b[1;30minitializing tty1 ........ \x1b[0;32mok\x1b[0m", 240 * time.Millisecond},
		{"  \x1b[1;30mraising dtr .............. \x1b[0;32mok\x1b[0m", 200 * time.Millisecond},
		{"  \x1b[1;30mcarrier detect ........... \x1b[1;33mringing\x1b[0m", 360 * time.Millisecond},
		{"  \x1b[1;32m  CONNECT 57600\x1b[1;30m/ARQ/V90/LAPM/V42BIS\x1b[0m", 420 * time.Millisecond},
		{"  \x1b[1;30mnegotiating ansi-bbs ..... \x1b[0;36mCP437\x1b[0m", 260 * time.Millisecond},
	}
	for _, bt := range beats {
		s.Print(bt.text + "\r\n")
		if s.WaitKey(bt.pause) {
			return // caller skipped: bail straight into the matrix
		}
	}
	s.Sleep(180 * time.Millisecond)
}
