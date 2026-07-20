package store

import "testing"

func TestTwitList(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTwit(1, "phantom"); err != nil {
		t.Fatalf("AddTwit: %v", err)
	}
	if err := s.AddTwit(1, "phantom"); err != nil { // idempotent
		t.Fatalf("AddTwit (dupe): %v", err)
	}
	if err := s.AddTwit(1, "Lamer"); err != nil {
		t.Fatalf("AddTwit: %v", err)
	}
	// A different user's list is separate.
	s.AddTwit(2, "nut")

	list, _ := s.Twits(1)
	if len(list) != 2 {
		t.Fatalf("Twits(1) = %v, want 2 entries", list)
	}
	// Sorted case-insensitively: Lamer, phantom.
	if list[0] != "Lamer" || list[1] != "phantom" {
		t.Errorf("Twits order = %v, want [Lamer phantom]", list)
	}
	set, _ := s.TwitSet(1)
	if !set["phantom"] || !set["lamer"] {
		t.Errorf("TwitSet = %v, want lower-cased phantom+lamer", set)
	}

	// Remove is case-insensitive.
	if err := s.RemoveTwit(1, "PHANTOM"); err != nil {
		t.Fatalf("RemoveTwit: %v", err)
	}
	if list, _ := s.Twits(1); len(list) != 1 || list[0] != "Lamer" {
		t.Fatalf("after remove, Twits(1) = %v, want [Lamer]", list)
	}
	// Empty handle refused.
	if err := s.AddTwit(1, "   "); err == nil {
		t.Error("AddTwit should refuse a blank handle")
	}
}
