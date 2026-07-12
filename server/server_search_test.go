package main

import (
	"net"
	"testing"

	"vendetta-x/server/internal/term"
)

// runOnSession runs fn against a real term.Session over an in-memory net.Pipe,
// so a single board flow can be driven end-to-end with the shared session
// helpers (send/expect/drain) without walking the whole login + menu tree.
func runOnSession(t *testing.T, b *board, fn func(s *term.Session)) *session {
	t.Helper()
	serverEnd, clientEnd := net.Pipe()
	se := &session{t: t, client: clientEnd, done: make(chan struct{})}
	go func() {
		defer close(se.done)
		s := term.NewRW(serverEnd, "test:0")
		fn(s)
		serverEnd.Close()
	}()
	return se
}

// TestTerminalMessageSearch drives the terminal message search end-to-end: the
// results list renders the hit, opening it lands in the reader on that message,
// and quitting walks back out cleanly.
func TestTerminalMessageSearch(t *testing.T) {
	b := newTestBoard(t)
	user, err := b.st.UserByHandle("nut")
	if err != nil || user == nil {
		t.Fatalf("UserByHandle(nut): %v", err)
	}

	se := runOnSession(t, b, func(s *term.Session) {
		// "ratio" hits the seeded warez post ("...Maintain ratio...").
		b.searchMessages(s, user, "ratio")
	})

	se.expect("match")          // the "N match(es) across your bases" header
	se.expect("Trade rules")    // the hit's subject in the results list
	se.send("1\r")              // open the first hit in the reader
	se.expect("Maintain ratio") // the message body, proving readSearchHit landed
	se.send("q")                // leave the reader (single-key ReadKey)
	se.send("q\r")              // leave the results list (ReadLine)
	se.drain()
	se.waitDone()
}

// TestTerminalFileSearch drives the terminal file search: the results list
// renders the matching seeded file, and quitting returns cleanly.
func TestTerminalFileSearch(t *testing.T) {
	b := newTestBoard(t)
	user, err := b.st.UserByHandle("nut")
	if err != nil || user == nil {
		t.Fatalf("UserByHandle(nut): %v", err)
	}

	se := runOnSession(t, b, func(s *term.Session) {
		// "keygen" matches the seeded VENDETTA-KEYGEN.ZIP by name + description.
		b.searchFiles(s, user, "keygen")
	})

	se.expect("VENDETTA-KEYGEN.ZIP")
	se.send("q\r") // leave the results list
	se.drain()
	se.waitDone()
}
