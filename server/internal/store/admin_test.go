package store

import (
	"testing"
	"time"
)

func TestBoardCRUD(t *testing.T) {
	s := newTestStore(t)

	id, err := s.AddBoard(&Board{Tag: "t", Name: "Test", Desc: "d", ReadACS: "s10", PostACS: "s20"})
	if err != nil {
		t.Fatalf("AddBoard: %v", err)
	}
	b, err := s.BoardByID(id)
	if err != nil || b == nil {
		t.Fatalf("BoardByID(%d): %v / %v", id, b, err)
	}
	if b.Name != "Test" || b.ReadACS != "s10" || b.PostACS != "s20" {
		t.Fatalf("unexpected board: %+v", b)
	}

	b.Name = "Renamed"
	b.ReadACS = ""
	if err := s.UpdateBoard(b); err != nil {
		t.Fatalf("UpdateBoard: %v", err)
	}
	got, _ := s.BoardByID(id)
	if got.Name != "Renamed" || got.ReadACS != "" {
		t.Fatalf("update not applied: %+v", got)
	}

	// A message on the board should be removed with it (cascade).
	if _, err := s.PostMessage(&Message{BoardID: id, From: "a", To: "b", Subject: "s", Body: "x"}); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := s.DeleteBoard(id); err != nil {
		t.Fatalf("DeleteBoard: %v", err)
	}
	if got, _ := s.BoardByID(id); got != nil {
		t.Fatalf("board still present after delete")
	}
	if msgs, _ := s.Messages(id, 0); len(msgs) != 0 {
		t.Fatalf("messages not cascaded: %d left", len(msgs))
	}
}

func TestFileAreaCRUD(t *testing.T) {
	s := newTestStore(t)

	id, err := s.AddFileArea(&FileArea{Tag: "a", Name: "Area", Desc: "d", ACS: "s30"})
	if err != nil {
		t.Fatalf("AddFileArea: %v", err)
	}
	a, err := s.FileAreaByID(id)
	if err != nil || a == nil || a.ACS != "s30" {
		t.Fatalf("FileAreaByID: %+v / %v", a, err)
	}
	a.Name = "Renamed"
	if err := s.UpdateFileArea(a); err != nil {
		t.Fatalf("UpdateFileArea: %v", err)
	}
	if got, _ := s.FileAreaByID(id); got.Name != "Renamed" {
		t.Fatalf("area update not applied: %+v", got)
	}
	if err := s.DeleteFileArea(id); err != nil {
		t.Fatalf("DeleteFileArea: %v", err)
	}
	if got, _ := s.FileAreaByID(id); got != nil {
		t.Fatalf("area still present after delete")
	}
}

func TestUserUpdateAndDelete(t *testing.T) {
	s := newTestStore(t)

	id, err := s.AddUser(&User{Handle: "neo", Group: "Users", SL: 10, DSL: 10})
	if err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	u, _ := s.UserByID(id)
	u.Group = "Elite"
	u.Flags = "AC"
	u.SL = 200
	u.Tagline = "edited"
	if err := s.UpdateUser(u); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	got, _ := s.UserByID(id)
	if got.Group != "Elite" || got.Flags != "AC" || got.SL != 200 || got.Tagline != "edited" {
		t.Fatalf("user update not applied: %+v", got)
	}
	// Handle is identity and must be untouched by UpdateUser.
	if got.Handle != "neo" {
		t.Fatalf("handle changed unexpectedly: %q", got.Handle)
	}
	if err := s.DeleteUser(id); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if got, _ := s.UserByID(id); got != nil {
		t.Fatalf("user still present after delete")
	}
}

func TestSettingsAndFeatures(t *testing.T) {
	s := newTestStore(t)

	// Unset keys fall back to the provided defaults.
	if v := s.Setting("board.name", "Default"); v != "Default" {
		t.Fatalf("Setting default: %q", v)
	}
	if n := s.SettingInt("newuser.sl", 10); n != 10 {
		t.Fatalf("SettingInt default: %d", n)
	}
	if !s.FeatureEnabled("voting") {
		t.Fatalf("features should default on")
	}

	if err := s.SetSetting("board.name", "Vendetta/X"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	// Upsert overwrites in place.
	if err := s.SetSetting("board.name", "ACME"); err != nil {
		t.Fatalf("SetSetting upsert: %v", err)
	}
	if v := s.Setting("board.name", "x"); v != "ACME" {
		t.Fatalf("Setting after upsert: %q", v)
	}

	if err := s.SetSetting("feature.voting", "0"); err != nil {
		t.Fatalf("SetSetting feature: %v", err)
	}
	if s.FeatureEnabled("voting") {
		t.Fatalf("voting should be disabled after setting 0")
	}
	if err := s.SetSetting("newuser.sl", "25"); err != nil {
		t.Fatalf("SetSetting int: %v", err)
	}
	if n := s.SettingInt("newuser.sl", 10); n != 25 {
		t.Fatalf("SettingInt after set: %d", n)
	}

	all, err := s.Settings()
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if all["board.name"] != "ACME" || all["feature.voting"] != "0" {
		t.Fatalf("Settings map wrong: %+v", all)
	}
}

func TestPurgeOldMessages(t *testing.T) {
	s := newTestStore(t)
	boardID, err := s.AddBoard(&Board{Tag: "t", Name: "Test"})
	if err != nil {
		t.Fatalf("AddBoard: %v", err)
	}

	now := time.Now()
	old, _ := s.PostMessage(&Message{BoardID: boardID, Subject: "old", Posted: now.Add(-48 * time.Hour)})
	recent, _ := s.PostMessage(&Message{BoardID: boardID, Subject: "recent", Posted: now.Add(-1 * time.Hour)})

	n, err := s.PurgeOldMessages(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("PurgeOldMessages: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged %d messages, want 1", n)
	}

	msgs, _ := s.Messages(boardID, 0)
	if len(msgs) != 1 || msgs[0].ID != recent {
		t.Fatalf("wrong messages survived purge: %+v", msgs)
	}
	_ = old
}

func TestLocalMessagesAfterAndHasMessage(t *testing.T) {
	s := newTestStore(t)
	boardID, err := s.AddBoard(&Board{Tag: "net", Name: "Networked"})
	if err != nil {
		t.Fatalf("AddBoard: %v", err)
	}

	when := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	local1, _ := s.PostMessage(&Message{BoardID: boardID, From: "nut", Subject: "local one", Posted: when})
	if _, err := s.PostMessage(&Message{
		BoardID: boardID, From: "remote guy", Subject: "imported", Posted: when, Origin: "DOVENET",
	}); err != nil {
		t.Fatalf("PostMessage imported: %v", err)
	}
	local2, _ := s.PostMessage(&Message{BoardID: boardID, From: "razor", Subject: "local two", Posted: when.Add(time.Minute)})

	// Everything local, oldest first, imported message excluded.
	got, err := s.LocalMessagesAfter(boardID, 0)
	if err != nil {
		t.Fatalf("LocalMessagesAfter: %v", err)
	}
	if len(got) != 2 || got[0].ID != local1 || got[1].ID != local2 {
		t.Fatalf("export feed = %+v, want local ids %d,%d", got, local1, local2)
	}

	// High-water mark excludes already-exported rows.
	got, _ = s.LocalMessagesAfter(boardID, local1)
	if len(got) != 1 || got[0].ID != local2 {
		t.Fatalf("after mark = %+v, want just %d", got, local2)
	}

	// Origin survives the round trip.
	all, _ := s.Messages(boardID, 0)
	var foundOrigin bool
	for _, m := range all {
		if m.Origin == "DOVENET" {
			foundOrigin = true
		}
	}
	if !foundOrigin {
		t.Fatal("imported message lost its origin")
	}

	// Dedup check: exact (board, from, subject, posted) matches; others don't.
	if ok, _ := s.HasMessage(boardID, "remote guy", "imported", when); !ok {
		t.Fatal("HasMessage missed an existing message")
	}
	if ok, _ := s.HasMessage(boardID, "remote guy", "imported", when.Add(time.Second)); ok {
		t.Fatal("HasMessage matched a different posted time")
	}
	if ok, _ := s.HasMessage(boardID, "someone else", "imported", when); ok {
		t.Fatal("HasMessage matched a different author")
	}
}

func TestTrimOneliners(t *testing.T) {
	s := newTestStore(t)
	base := time.Now()
	for i := 0; i < 5; i++ {
		if err := s.AddOneliner(&Oneliner{
			Author: "x", Text: "line",
			Posted: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("AddOneliner %d: %v", i, err)
		}
	}

	n, err := s.TrimOneliners(2)
	if err != nil {
		t.Fatalf("TrimOneliners: %v", err)
	}
	if n != 3 {
		t.Fatalf("trimmed %d, want 3", n)
	}
	left, _ := s.Oneliners(10)
	if len(left) != 2 {
		t.Fatalf("left with %d oneliners, want 2", len(left))
	}

	// keep <= 0 is a no-op.
	if n, err := s.TrimOneliners(0); err != nil || n != 0 {
		t.Fatalf("TrimOneliners(0) = %d, %v, want 0, nil", n, err)
	}
	left, _ = s.Oneliners(10)
	if len(left) != 2 {
		t.Fatalf("TrimOneliners(0) trimmed something: %d left", len(left))
	}
}
