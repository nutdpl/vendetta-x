package main

import (
	"strings"
	"testing"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

func TestPersonalScanListsAndReads(t *testing.T) {
	b := newTestBoard(t)
	// Someone addresses nut by name in the open General base.
	genID := int64(1)
	if _, err := b.st.PostMessage(&store.Message{
		BoardID: genID, From: "phantom", To: "nut", Subject: "you around?", Body: "ping me back",
	}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	user, _ := b.st.UserByHandle("nut")

	se := runOnSession(t, b, func(s *term.Session) { b.personalScan(s, user) })
	se.expect("addressed to you")
	se.expect("you around?")
	se.send("1\r") // open it in the reader
	se.expect("ping me back")
	se.send("q")   // leave the reader
	se.send("q\r") // leave the scan list
	se.drain()
	se.waitDone()
}

func TestPersonalScanEmpty(t *testing.T) {
	b := newTestBoard(t)
	user, _ := b.st.UserByHandle("nut")
	se := runOnSession(t, b, func(s *term.Session) { b.personalScan(s, user) })
	se.expect("No public messages are addressed to you")
	se.send(" ") // dismiss the notice's pause
	se.drain()
	se.waitDone()
}

func TestLogonOffersPersonalMessages(t *testing.T) {
	b := newTestBoard(t)
	// A prior last-call so the digest runs, and a message addressed to nut.
	b.st.DB().Exec(`UPDATE users SET last_call=strftime('%s','2026-07-01 12:00:00'), calls=5 WHERE handle='nut'`)
	if _, err := b.st.PostMessage(&store.Message{BoardID: 1, From: "phantom", To: "nut", Subject: "hi", Body: "x"}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	user, _ := b.st.UserByHandle("nut")

	se := runOnSession(t, b, func(s *term.Session) { b.logon(s, map[string]string{}, user) })
	se.expect("addressed to you")
	se.send("n") // decline reading now
	se.expect("Quick logon?")
	se.send("y") // skip the tour
	se.drain()
	se.waitDone()
	if !strings.Contains(string(se.out), "read now?") {
		t.Error("logon should offer to read personal messages")
	}
}
