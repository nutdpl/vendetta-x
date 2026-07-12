package badges

import (
	"strings"
	"testing"

	"vendetta-x/server/internal/store"
)

func titleSet(u store.User) map[string]bool {
	m := map[string]bool{}
	for _, t := range Titles(u) {
		m[t] = true
	}
	return m
}

func TestEarnedLaddersPickHighestRung(t *testing.T) {
	// A busy regular: 120 posts -> Co-Conspirator (not Regular), 30 calls ->
	// Familiar Face, 12 uploads -> Supplier, 150 MB -> Data Hoard.
	u := store.User{Posts: 120, Calls: 30, Uploads: 12, UlBytes: 150 << 20}
	set := titleSet(u)
	for _, want := range []string{"Co-Conspirator", "Familiar Face", "Supplier", "Data Hoard"} {
		if !set[want] {
			t.Errorf("expected badge %q, got %v", want, Titles(u))
		}
	}
	// The lower rungs it passed through must NOT also show.
	for _, notWant := range []string{"Regular", "New Blood", "Contributor", "Packet Mule"} {
		if set[notWant] {
			t.Errorf("did not expect superseded badge %q in %v", notWant, Titles(u))
		}
	}
}

func TestEarnedEmptyForBlankUser(t *testing.T) {
	if got := Earned(store.User{}); len(got) != 0 {
		t.Fatalf("a brand-new account should hold no badges, got %v", got)
	}
}

func TestStaffBadgeForPrivileged(t *testing.T) {
	// SL >= 100 is privileged.
	if !titleSet(store.User{SL: 255})["Staff"] {
		t.Error("an SL 255 account should carry the Staff badge")
	}
	// The "A" flag is privileged too, case-insensitively.
	if !titleSet(store.User{Flags: "a"})["Staff"] {
		t.Error("an a-flagged account should carry the Staff badge")
	}
	// An ordinary user does not.
	if titleSet(store.User{SL: 10, Posts: 5})["Staff"] {
		t.Error("an ordinary user must not carry the Staff badge")
	}
}

func TestEarnedDescriptionsAccompanyTitles(t *testing.T) {
	bs := Earned(store.User{Posts: 1})
	if len(bs) != 1 || bs[0].Title != "First Words" || strings.TrimSpace(bs[0].Desc) == "" {
		t.Fatalf("first post should earn 'First Words' with a description, got %+v", bs)
	}
}
