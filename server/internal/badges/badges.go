// Package badges derives a caller's earned distinctions purely from their
// existing counters -- posts, calls, uploads, bytes shared -- with no stored
// state of its own. Thresholds map to scene-flavored titles ("100 posts ->
// CO-CONSPIRATOR") shown on profile cards and the stats screen across every
// face. Because it's a pure function of the user record, a badge appears the
// instant the counter crosses its line and never needs a migration or a
// backfill.
package badges

import "vendetta-x/server/internal/store"

// Badge is one earned distinction. Title is the short name shown on a card;
// Desc says what earned it (for a tooltip / the "how" line).
type Badge struct {
	Title string
	Desc  string
}

// tier is one rung of a ladder: the minimum counter value that earns it, and
// the title/desc awarded. Ladders are listed ascending; the highest rung a
// value clears is the one that shows, so a card carries one badge per category
// instead of the whole ladder.
type tier struct {
	min   int64
	title string
	desc  string
}

var (
	postTiers = []tier{
		{1, "First Words", "posted to the bases"},
		{25, "Regular", "25 posts"},
		{100, "Co-Conspirator", "100 posts"},
		{500, "Loudmouth", "500 posts"},
		{2000, "Board Legend", "2000 posts"},
	}
	callTiers = []tier{
		{1, "New Blood", "first call"},
		{25, "Familiar Face", "25 calls"},
		{100, "Die-Hard", "100 calls"},
		{365, "Nightrider", "365 calls"},
	}
	uploadTiers = []tier{
		{1, "Contributor", "uploaded a file"},
		{10, "Supplier", "10 uploads"},
		{50, "Courier", "50 uploads"},
		{200, "Distro King", "200 uploads"},
	}
	byteTiers = []tier{
		{1 << 20, "Packet Mule", "1 MB shared"},
		{100 << 20, "Data Hoard", "100 MB shared"},
		{1 << 30, "Warez Baron", "1 GB shared"},
	}
)

// highest returns the top tier val clears, or nil when it clears none.
func highest(val int64, tiers []tier) *Badge {
	var got *Badge
	for i := range tiers {
		if val >= tiers[i].min {
			got = &Badge{Title: tiers[i].title, Desc: tiers[i].desc}
		}
	}
	return got
}

// Earned returns every badge u currently holds -- a staff mark for privileged
// accounts, then the highest rung reached on the posts, calls, uploads, and
// bytes-shared ladders. Order is stable (identity first, then activity) so a
// card reads the same every time.
func Earned(u store.User) []Badge {
	var out []Badge
	if u.Privileged() {
		out = append(out, Badge{"Staff", "runs the board"})
	}
	for _, b := range []*Badge{
		highest(int64(u.Posts), postTiers),
		highest(int64(u.Calls), callTiers),
		highest(int64(u.Uploads), uploadTiers),
		highest(u.UlBytes, byteTiers),
	} {
		if b != nil {
			out = append(out, *b)
		}
	}
	return out
}

// Titles returns just the badge titles, for the terminal card's one-line list.
func Titles(u store.User) []string {
	bs := Earned(u)
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		out = append(out, b.Title)
	}
	return out
}
