package store

import "testing"

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
