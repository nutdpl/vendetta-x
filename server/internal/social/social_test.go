package social

import (
	"strings"
	"testing"
	"time"

	"vendetta-x/server/internal/store"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func sampleUsers() []store.User {
	return []store.User{
		{Handle: "alice", Posts: 10, Calls: 5, FirstCall: d(2020, 1, 1), LastCall: d(2024, 1, 1)},
		{Handle: "bob", Posts: 50, Calls: 1, FirstCall: d(2022, 6, 1), LastCall: d(2026, 6, 1)},
		{Handle: "carol", Posts: 30, Calls: 9, FirstCall: d(2019, 3, 3), LastCall: d(2025, 5, 5)},
		{Handle: "dave", Posts: 30, Calls: 9, FirstCall: d(2023, 12, 31), LastCall: d(2021, 2, 2)},
	}
}

func handles(us []store.User) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = u.Handle
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRankTopPostersDescAndCap(t *testing.T) {
	us := sampleUsers()
	l := Rank(us, 2)
	if len(l.TopPosters) != 2 {
		t.Fatalf("expected cap of 2, got %d", len(l.TopPosters))
	}
	got := handles(l.TopPosters)
	want := []string{"bob", "carol"} // 50, then 30 (carol < dave by handle)
	if !eqStrings(got, want) {
		t.Fatalf("TopPosters = %v, want %v", got, want)
	}
}

func TestRankTopCallersDesc(t *testing.T) {
	l := Rank(sampleUsers(), 10)
	got := handles(l.TopCallers)
	// Calls: carol=9, dave=9 (tie -> carol, dave), alice=5, bob=1.
	want := []string{"carol", "dave", "alice", "bob"}
	if !eqStrings(got, want) {
		t.Fatalf("TopCallers = %v, want %v", got, want)
	}
}

func TestRankTieBreakByHandleDeterministic(t *testing.T) {
	// carol and dave both have Posts=30; tie must break by handle (carol<dave).
	for i := 0; i < 5; i++ {
		l := Rank(sampleUsers(), 10)
		got := handles(l.TopPosters)
		want := []string{"bob", "carol", "dave", "alice"}
		if !eqStrings(got, want) {
			t.Fatalf("iter %d: TopPosters = %v, want %v", i, got, want)
		}
	}
}

func TestRankTieBreakCaseInsensitive(t *testing.T) {
	us := []store.User{
		{Handle: "Zed", Posts: 5},
		{Handle: "apex", Posts: 5},
	}
	l := Rank(us, 10)
	got := handles(l.TopPosters)
	want := []string{"apex", "Zed"} // case-insensitive: apex < zed
	if !eqStrings(got, want) {
		t.Fatalf("case-insensitive tie-break = %v, want %v", got, want)
	}
}

func TestRankNewestUsesFirstCallRecency(t *testing.T) {
	l := Rank(sampleUsers(), 10)
	got := handles(l.NewestUsers)
	// FirstCall most-recent first: dave(2023), bob(2022), alice(2020), carol(2019).
	want := []string{"dave", "bob", "alice", "carol"}
	if !eqStrings(got, want) {
		t.Fatalf("NewestUsers = %v, want %v", got, want)
	}
}

func TestRankDefaultNAndShortInput(t *testing.T) {
	l := Rank(sampleUsers(), 0) // n<=0 -> default; fewer users than default
	if len(l.TopPosters) != 4 {
		t.Fatalf("expected all 4 with default n, got %d", len(l.TopPosters))
	}
}

func TestRankDoesNotMutateInput(t *testing.T) {
	us := sampleUsers()
	before := handles(us)
	_ = Rank(us, 2)
	after := handles(us)
	if !eqStrings(before, after) {
		t.Fatalf("Rank mutated input: before %v, after %v", before, after)
	}
}

func TestLastCallersOrderingAndNoMutation(t *testing.T) {
	us := sampleUsers()
	before := handles(us)
	got := handles(LastCallers(us, 10))
	// LastCall most-recent first: bob(2026), carol(2025), alice(2024), dave(2021).
	want := []string{"bob", "carol", "alice", "dave"}
	if !eqStrings(got, want) {
		t.Fatalf("LastCallers = %v, want %v", got, want)
	}
	if !eqStrings(before, handles(us)) {
		t.Fatalf("LastCallers mutated input")
	}
}

func TestLastCallersCap(t *testing.T) {
	got := LastCallers(sampleUsers(), 1)
	if len(got) != 1 || got[0].Handle != "bob" {
		t.Fatalf("LastCallers cap=1 = %v", handles(got))
	}
}

func TestProfileCardContents(t *testing.T) {
	u := store.User{
		Handle: "phantom", RealName: "Ghost", Group: "Staff",
		Posts: 7, Calls: 3,
		FirstCall: d(2020, 1, 2),
		// LastCall left zero -> should render "never".
	}
	card := ProfileCard(u)
	if !strings.Contains(card, "phantom") {
		t.Fatalf("ProfileCard missing handle")
	}
	if !strings.Contains(card, "\x1b[0m") {
		t.Fatalf("ProfileCard missing reset escape")
	}
	if !strings.Contains(card, "never") {
		t.Fatalf("ProfileCard should show 'never' for zero LastCall")
	}
	if !strings.Contains(card, "2020-01-02") {
		t.Fatalf("ProfileCard missing formatted FirstCall date")
	}
	if !strings.Contains(card, "\r\n") {
		t.Fatalf("ProfileCard should use CRLF line endings")
	}
}

func TestProfileCardShowsBadges(t *testing.T) {
	// 120 posts earns the Co-Conspirator badge, which the card should list.
	active := store.User{Handle: "nut", Posts: 120, FirstCall: d(2021, 6, 1)}
	if card := ProfileCard(active); !strings.Contains(card, "Co-Conspirator") {
		t.Fatalf("ProfileCard should list earned badges, got:\n%s", card)
	}
	// A brand-new account earns nothing, so the badges row is omitted entirely.
	fresh := store.User{Handle: "noob"}
	if card := ProfileCard(fresh); strings.Contains(card, "badges") {
		t.Fatalf("ProfileCard should omit the badges row for a user with none")
	}
}

func TestLeaderBoardHeadingsNonEmpty(t *testing.T) {
	board := LeaderBoard(Rank(sampleUsers(), 3))
	if board == "" {
		t.Fatalf("LeaderBoard is empty")
	}
	for _, h := range []string{"Top Posters", "Top Callers", "Newest Users"} {
		if !strings.Contains(board, h) {
			t.Fatalf("LeaderBoard missing heading %q", h)
		}
	}
	if !strings.Contains(board, "\x1b[0m") {
		t.Fatalf("LeaderBoard missing reset escape")
	}
}
