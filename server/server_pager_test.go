package main

import (
	"fmt"
	"strings"
	"testing"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

func numberedBody(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("body line %02d of the document", i+1)
	}
	return strings.Join(lines, "\n")
}

// TestPagerPagesAndQuits drives the "more" pager: it pauses a page in, pages
// forward on a key, and Q suppresses the rest.
func TestPagerPagesAndQuits(t *testing.T) {
	b := newTestBoard(t)
	var ret bool
	se := runOnSession(t, b, func(s *term.Session) {
		ret = b.pageText(s, numberedBody(40), 10)
	})
	se.expect("-- more --") // first page paused at row 10
	se.send(" ")            // page forward
	se.expect("body line 11")
	se.expect("-- more --") // paused again
	se.send("q")            // stop
	se.drain()
	se.waitDone()

	if ret {
		t.Error("pageText should return false when the caller quits early")
	}
	if strings.Contains(string(se.out), "body line 40") {
		t.Error("quitting the pager should suppress the remaining lines")
	}
}

// TestPagerShortBodyNoPrompt: content that fits shows no prompt and completes.
func TestPagerShortBodyNoPrompt(t *testing.T) {
	b := newTestBoard(t)
	var ret bool
	se := runOnSession(t, b, func(s *term.Session) {
		ret = b.pageText(s, "one\ntwo\nthree", 10)
	})
	se.drain()
	se.waitDone()

	if !ret {
		t.Error("a short body should complete (return true)")
	}
	if strings.Contains(string(se.out), "-- more --") {
		t.Error("a body that fits should not show the more prompt")
	}
}

// TestPagerHonorsTallTerminal proves NAWS feeds the pager: a 30-line post that
// would page on the 24-row floor shows no prompt on a 50-row terminal.
func TestPagerHonorsTallTerminal(t *testing.T) {
	b := newTestBoard(t)
	bd := &store.Board{ID: 1, Name: "General", Tag: "gen"}
	msgs := []store.Message{{ID: 1, BoardID: 1, From: "nut", To: "All", Subject: "tall", Body: numberedBody(30)}}
	se := runOnSession(t, b, func(s *term.Session) {
		s.SetWinSize(120, 50)
		b.showMessage(s, bd, msgs, 0, false)
	})
	se.drain()
	se.waitDone()
	if strings.Contains(string(se.out), "-- more --") {
		t.Error("a 30-line post should not page on a 50-row terminal")
	}
}

// TestMessageReaderPagesLongBody proves the reader wires the pager in: a long
// post pauses, and [C]ont reveals the rest.
func TestMessageReaderPagesLongBody(t *testing.T) {
	b := newTestBoard(t)
	bd := &store.Board{ID: 1, Name: "General", Tag: "gen"}
	msgs := []store.Message{{ID: 1, BoardID: 1, From: "nut", To: "All", Subject: "long one", Body: numberedBody(30)}}

	se := runOnSession(t, b, func(s *term.Session) {
		b.showMessage(s, bd, msgs, 0, false)
	})
	se.expect("-- more --")
	se.send("c") // continuous: dump the remainder
	se.expect("body line 30")
	se.drain()
	se.waitDone()
}
