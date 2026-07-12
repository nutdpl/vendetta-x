package store

import "testing"

func TestSetPrefsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	u, _ := s.UserByHandle("nut")
	if u.Expert || u.Clock12 {
		t.Fatalf("defaults should be off, got expert=%v clock12=%v", u.Expert, u.Clock12)
	}

	if err := s.SetPrefs(u.ID, true, true); err != nil {
		t.Fatalf("SetPrefs: %v", err)
	}
	got, _ := s.UserByHandle("nut")
	if !got.Expert || !got.Clock12 {
		t.Fatalf("after set: expert=%v clock12=%v, want both true", got.Expert, got.Clock12)
	}

	if err := s.SetPrefs(u.ID, false, true); err != nil {
		t.Fatalf("SetPrefs: %v", err)
	}
	got, _ = s.UserByHandle("nut")
	if got.Expert || !got.Clock12 {
		t.Fatalf("after toggle: expert=%v clock12=%v, want false/true", got.Expert, got.Clock12)
	}
}
