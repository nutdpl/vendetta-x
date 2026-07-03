package store

import (
	"testing"
	"time"
)

// post is a test helper: one message into a board, returning its id.
func post(t *testing.T, s *Store, boardID int64, subj string) int64 {
	t.Helper()
	id, err := s.PostMessage(&Message{
		BoardID: boardID, From: "nut", To: "All", Subject: subj, Body: "body",
		Posted: time.Now(),
	})
	if err != nil {
		t.Fatalf("PostMessage(%s): %v", subj, err)
	}
	return id
}

func TestLastReadPointer(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	boards, _ := s.Boards()
	bd := boards[0].ID
	const user = int64(1)

	// Never read: pointer is zero.
	if got, err := s.LastRead(user, bd); err != nil || got != 0 {
		t.Fatalf("LastRead fresh = %d, %v; want 0, nil", got, err)
	}

	m1 := post(t, s, bd, "one")
	m2 := post(t, s, bd, "two")

	if err := s.SetLastRead(user, bd, m2); err != nil {
		t.Fatalf("SetLastRead: %v", err)
	}
	if got, _ := s.LastRead(user, bd); got != m2 {
		t.Fatalf("LastRead = %d, want %d", got, m2)
	}

	// Monotonic: re-reading an older message can't rewind the pointer.
	if err := s.SetLastRead(user, bd, m1); err != nil {
		t.Fatalf("SetLastRead older: %v", err)
	}
	if got, _ := s.LastRead(user, bd); got != m2 {
		t.Fatalf("pointer rewound to %d, want %d", got, m2)
	}
}

func TestUploadQueue(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	areas, _ := s.FileAreas()
	area := areas[0].ID

	id, err := s.AddPendingFile(area, "held.zip", "waiting", "phantom", []byte("payload"))
	if err != nil {
		t.Fatalf("AddPendingFile: %v", err)
	}

	// Invisible while pending: listing, content, digest.
	for _, f := range mustFiles(t, s, area) {
		if f.ID == id {
			t.Fatal("pending file leaked into the area listing")
		}
	}
	if c, _ := s.FileContent(id); c != nil {
		t.Fatal("pending file content served")
	}

	// But the dupe check sees it -- re-uploading the same bytes is refused.
	f, _ := s.FileByID(id)
	if f == nil || f.Approved {
		t.Fatalf("FileByID pending = %+v", f)
	}
	if dupe, _ := s.FileByHash(f.Hash); dupe == nil {
		t.Fatal("FileByHash missed the pending copy")
	}

	// Approval flips every switch.
	if err := s.ApproveFile(id); err != nil {
		t.Fatalf("ApproveFile: %v", err)
	}
	found := false
	for _, f := range mustFiles(t, s, area) {
		found = found || f.ID == id
	}
	if !found {
		t.Fatal("approved file missing from listing")
	}
	if c, _ := s.FileContent(id); string(c) != "payload" {
		t.Fatalf("approved content = %q", c)
	}

	// Rejection path: delete removes the row entirely.
	id2, _ := s.AddPendingFile(area, "bad.zip", "", "phantom", []byte("junk"))
	if err := s.DeleteFile(id2); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if f, _ := s.FileByID(id2); f != nil {
		t.Fatal("rejected file still present")
	}
}

func mustFiles(t *testing.T, s *Store, area int64) []FileEntry {
	t.Helper()
	files, err := s.Files(area)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	return files
}

func TestAutomessage(t *testing.T) {
	s := newTestStore(t)
	if a, txt, _ := s.Automessage(); a != "" || txt != "" {
		t.Fatalf("fresh automessage should be empty, got %q/%q", a, txt)
	}
	if err := s.SetAutomessage("phantom", "vendetta lives \x1b[31m"); err != nil {
		t.Fatalf("SetAutomessage: %v", err)
	}
	author, text, at := s.Automessage()
	if author != "phantom" || text != "vendetta lives [31m" || at.IsZero() {
		t.Fatalf("Automessage = %q/%q/%v (control bytes must be stripped)", author, text, at)
	}
	if err := s.SetAutomessage("", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, txt, _ := s.Automessage(); txt != "" {
		t.Fatalf("cleared automessage still reads %q", txt)
	}
}

func TestDigestCounts(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	cutoff := time.Now().Add(-1 * time.Minute)

	areas, _ := s.FileAreas()
	if _, err := s.AddFile(areas[0].ID, "fresh.zip", "new stuff", "nut", []byte("x")); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	counts, err := s.FileCountsAfter(cutoff)
	if err != nil || counts[areas[0].ID] < 1 {
		t.Fatalf("FileCountsAfter = %v, %v; want area %d >= 1", counts, err, areas[0].ID)
	}
	if counts2, _ := s.FileCountsAfter(time.Now().Add(time.Hour)); len(counts2) != 0 {
		t.Fatalf("future cutoff should count nothing, got %v", counts2)
	}

	if _, err := s.AddUser(&User{Handle: "newblood", FirstCall: time.Now()}); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if n, err := s.NewUsersSince(cutoff); err != nil || n < 1 {
		t.Fatalf("NewUsersSince = %d, %v; want >= 1", n, err)
	}
}

func TestReplyToRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	boards, _ := s.Boards()
	bd := boards[0].ID

	root := post(t, s, bd, "root")
	reply, err := s.PostMessage(&Message{
		BoardID: bd, From: "phantom", To: "nut", Subject: "Re: root",
		Body: "nut> quoted\n\nagreed", Posted: time.Now(), ReplyTo: root,
	})
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}

	m, err := s.MessageByID(reply)
	if err != nil || m == nil {
		t.Fatalf("MessageByID: %v, %v", m, err)
	}
	if m.ReplyTo != root {
		t.Fatalf("ReplyTo = %d, want %d", m.ReplyTo, root)
	}
	if r, _ := s.MessageByID(root); r == nil || r.ReplyTo != 0 {
		t.Fatalf("root ReplyTo should be 0, got %+v", r)
	}
	if missing, err := s.MessageByID(999999); err != nil || missing != nil {
		t.Fatalf("missing message should be nil,nil; got %v, %v", missing, err)
	}
}

func TestUnreadCountsAndMessagesAfter(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	boards, _ := s.Boards()
	b1, b2 := boards[0].ID, boards[1].ID
	const user = int64(1)

	// Read everything that exists so far (the seed content), then post fresh.
	for _, bd := range boards {
		msgs, _ := s.Messages(bd.ID, 1)
		if len(msgs) > 0 {
			if err := s.SetLastRead(user, bd.ID, msgs[0].ID); err != nil {
				t.Fatalf("SetLastRead seed: %v", err)
			}
		}
	}
	if counts, _ := s.UnreadCounts(user); len(counts) != 0 {
		t.Fatalf("expected no unread after catching up, got %v", counts)
	}

	first := post(t, s, b1, "fresh-1")
	post(t, s, b1, "fresh-2")
	post(t, s, b2, "fresh-3")

	counts, err := s.UnreadCounts(user)
	if err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	if counts[b1] != 2 || counts[b2] != 1 {
		t.Fatalf("counts = %v, want board1:2 board2:1", counts)
	}

	// MessagesAfter feeds the new-scan: oldest first, only above the pointer.
	ptr, _ := s.LastRead(user, b1)
	fresh, err := s.MessagesAfter(b1, ptr)
	if err != nil {
		t.Fatalf("MessagesAfter: %v", err)
	}
	if len(fresh) != 2 || fresh[0].ID != first || fresh[0].Subject != "fresh-1" {
		t.Fatalf("MessagesAfter = %d msgs, first %q; want 2 msgs starting at fresh-1",
			len(fresh), fresh[0].Subject)
	}

	// A user with no pointers sees everything as new.
	all, _ := s.UnreadCounts(int64(99))
	if all[b1] < 2 || all[b2] < 1 {
		t.Fatalf("fresh user counts = %v, want at least board1:2 board2:1", all)
	}
}
