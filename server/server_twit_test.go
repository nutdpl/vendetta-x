package main

import (
	"strings"
	"testing"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
)

func TestFilterTwitMsgs(t *testing.T) {
	msgs := []store.Message{
		{From: "nut", Subject: "a"},
		{From: "phantom", Subject: "b"},
		{From: "Nut", Subject: "c"},
	}
	got := filterTwitMsgs(msgs, map[string]bool{"phantom": true})
	if len(got) != 2 {
		t.Fatalf("filtered = %d, want 2 (phantom hidden)", len(got))
	}
	for _, m := range got {
		if strings.EqualFold(m.From, "phantom") {
			t.Error("phantom's post survived the filter")
		}
	}
	// Empty set is a no-op.
	if len(filterTwitMsgs(msgs, nil)) != 3 {
		t.Error("nil twit set should not filter anything")
	}
}

func TestReaderHidesTwittedPoster(t *testing.T) {
	b := newTestBoard(t)
	user, _ := b.st.UserByHandle("nut")
	// Seed gen has a nut post ("Welcome...") and a phantom post ("First post").
	if err := b.st.AddTwit(user.ID, "phantom"); err != nil {
		t.Fatalf("AddTwit: %v", err)
	}
	se := runOnSession(t, b, func(s *term.Session) { b.readBoard(s, "gen", user) })
	se.expect("Welcome to Vendetta/X") // nut's post shows
	se.send("q")                       // quit the reader
	se.drain()
	se.waitDone()
	if strings.Contains(string(se.out), "First post") {
		t.Error("a twitted poster's message was shown in the reader")
	}
}

func TestTwitSettingsAddAndRemove(t *testing.T) {
	b := newTestBoard(t)
	user, _ := b.st.UserByHandle("nut")
	se := runOnSession(t, b, func(s *term.Session) { b.twitSettings(s, user) })
	se.expect("ignore list")
	se.send("a\r") // add
	se.expect("Handle to ignore")
	se.send("lamer\r") // the handle
	se.expect("Ignoring lamer")
	se.send(" ")       // dismiss the pause
	se.expect("lamer") // now listed
	se.send("1\r")     // remove #1
	se.expect("No longer ignoring lamer")
	se.send(" ")   // dismiss the pause
	se.send("q\r") // quit
	se.drain()
	se.waitDone()

	if list, _ := b.st.Twits(user.ID); len(list) != 0 {
		t.Errorf("ignore list should be empty after add+remove, got %v", list)
	}
}
