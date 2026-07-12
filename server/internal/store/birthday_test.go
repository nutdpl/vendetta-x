package store

import "testing"

func TestNormalizeBirthday(t *testing.T) {
	ok := map[string]string{
		"07-12": "07-12",
		"7-12":  "07-12",
		"07/12": "07-12",
		"1/1":   "01-01",
		"02-29": "02-29", // leap day is allowed (month-day only)
		"12-31": "12-31",
		"  ":    "", // blank clears
		"":      "",
	}
	for in, want := range ok {
		got, err := NormalizeBirthday(in)
		if err != nil {
			t.Errorf("NormalizeBirthday(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeBirthday(%q) = %q, want %q", in, got, want)
		}
	}

	for _, bad := range []string{"13-01", "00-10", "07-32", "07-00", "july", "7", "7-12-2020", "aa-bb"} {
		if _, err := NormalizeBirthday(bad); err == nil {
			t.Errorf("NormalizeBirthday(%q) should have errored", bad)
		}
	}
}

func TestSetBirthdayRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	u, _ := s.UserByHandle("nut")

	if err := s.SetBirthday(u.ID, "7/12"); err != nil {
		t.Fatalf("SetBirthday: %v", err)
	}
	got, _ := s.UserByHandle("nut")
	if got.Birthday != "07-12" {
		t.Fatalf("stored birthday = %q, want 07-12", got.Birthday)
	}

	// An invalid date is refused and leaves the stored value untouched.
	if err := s.SetBirthday(u.ID, "99-99"); err == nil {
		t.Fatal("SetBirthday should reject an invalid date")
	}
	got, _ = s.UserByHandle("nut")
	if got.Birthday != "07-12" {
		t.Fatalf("a rejected update changed the stored birthday to %q", got.Birthday)
	}

	// Clearing works.
	if err := s.SetBirthday(u.ID, ""); err != nil {
		t.Fatalf("SetBirthday clear: %v", err)
	}
	got, _ = s.UserByHandle("nut")
	if got.Birthday != "" {
		t.Fatalf("birthday should be cleared, got %q", got.Birthday)
	}
}
