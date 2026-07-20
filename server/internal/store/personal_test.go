package store

import "testing"

func TestMessagesToHandleAndUnread(t *testing.T) {
	s := newTestStore(t)
	gen, _ := s.db.Exec(`INSERT INTO boards (tag, name) VALUES ('gen','General')`)
	genID, _ := gen.LastInsertId()
	sys, _ := s.db.Exec(`INSERT INTO boards (tag, name, read_acs) VALUES ('sysop','Sysop','s100')`)
	sysID, _ := sys.LastInsertId()

	mk := func(board int64, from, to, subj string) int64 {
		m := &Message{BoardID: board, From: from, To: to, Subject: subj, Body: "x"}
		id, err := s.PostMessage(m)
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		return id
	}
	mk(genID, "phantom", "All", "broadcast")     // not personal
	mk(genID, "phantom", "nut", "re: your post") // to nut
	toNut2 := mk(genID, "sysop", "Nut", "hey")   // to nut (case-insensitive)
	mk(sysID, "sysop", "nut", "secret")          // to nut but in a gated base

	// Scan across the readable base only.
	got, err := s.MessagesToHandle("nut", []int64{genID}, 0)
	if err != nil {
		t.Fatalf("MessagesToHandle: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 personal messages in [gen], got %d", len(got))
	}
	// Newest first.
	if got[0].ID != toNut2 {
		t.Errorf("expected newest-first ordering, got %+v", got)
	}
	// The broadcast to "All" must never count.
	for _, m := range got {
		if m.To == "All" {
			t.Error("a message to All leaked into the personal scan")
		}
	}

	// A user (id 1) who has read nothing sees both as unread.
	n, err := s.UnreadToHandle(1, "nut", []int64{genID})
	if err != nil {
		t.Fatalf("UnreadToHandle: %v", err)
	}
	if n != 2 {
		t.Fatalf("unread to nut = %d, want 2", n)
	}
	// After advancing the read pointer past them, none remain unread.
	if err := s.SetLastRead(1, genID, toNut2); err != nil {
		t.Fatalf("SetLastRead: %v", err)
	}
	if n, _ := s.UnreadToHandle(1, "nut", []int64{genID}); n != 0 {
		t.Fatalf("after catch-up, unread to nut = %d, want 0", n)
	}

	// Gated base excluded when not in the id set.
	if got, _ := s.MessagesToHandle("nut", []int64{genID}, 0); len(got) != 2 {
		t.Fatalf("gated base should not contribute; got %d", len(got))
	}
	_ = sysID
}
