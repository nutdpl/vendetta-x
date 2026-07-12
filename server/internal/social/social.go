// Package social renders the board's social layer: top-N leaderboards, the
// last-callers list, and per-user profile cards. It computes rankings from the
// user base and renders them as ready-to-write ANSI blocks (raw SGR escapes,
// CRLF line endings) so a telnet session can emit them directly.
package social

import (
	"fmt"
	"sort"
	"strings"

	"vendetta-x/server/internal/badges"
	"vendetta-x/server/internal/store"
)

// defaultN is used when a caller passes n <= 0.
const defaultN = 10

// Leaders holds the computed top-N rankings over the user base.
type Leaders struct {
	TopPosters  []store.User // by Posts desc
	TopCallers  []store.User // by Calls desc
	NewestUsers []store.User // by FirstCall desc
}

// lessHandle compares two handles case-insensitively, falling back to the
// raw handle to keep the order total (and thus deterministic).
func lessHandle(a, b store.User) bool {
	la, lb := strings.ToLower(a.Handle), strings.ToLower(b.Handle)
	if la != lb {
		return la < lb
	}
	return a.Handle < b.Handle
}

// sortedCopy returns a copy of users sorted by the supplied "primary" less
// function, with ties broken case-insensitively by handle, capped at n. The
// caller's slice is never mutated.
func sortedCopy(users []store.User, n int, primaryLess func(a, b store.User) bool) []store.User {
	if n <= 0 {
		n = defaultN
	}
	cp := make([]store.User, len(users))
	copy(cp, users)
	sort.SliceStable(cp, func(i, j int) bool {
		if primaryLess(cp[i], cp[j]) {
			return true
		}
		if primaryLess(cp[j], cp[i]) {
			return false
		}
		return lessHandle(cp[i], cp[j])
	})
	if len(cp) > n {
		cp = cp[:n]
	}
	return cp
}

// Rank computes the leaderboards from the full user list; each list is capped
// at n entries. Ties break by handle (case-insensitive) for stable output.
func Rank(users []store.User, n int) Leaders {
	return Leaders{
		TopPosters:  sortedCopy(users, n, func(a, b store.User) bool { return a.Posts > b.Posts }),
		TopCallers:  sortedCopy(users, n, func(a, b store.User) bool { return a.Calls > b.Calls }),
		NewestUsers: sortedCopy(users, n, func(a, b store.User) bool { return a.FirstCall.After(b.FirstCall) }),
	}
}

// LastCallers returns users sorted by most-recent LastCall, capped at n.
func LastCallers(users []store.User, n int) []store.User {
	return sortedCopy(users, n, func(a, b store.User) bool { return a.LastCall.After(b.LastCall) })
}

// ---- rendering -------------------------------------------------------------

const (
	sgrReset   = "\x1b[0m"
	sgrBold    = "\x1b[1m"
	sgrCyan    = "\x1b[1;36m"
	sgrMagenta = "\x1b[1;35m"
	sgrYellow  = "\x1b[1;33m"
	sgrGreen   = "\x1b[1;32m"
	sgrGray    = "\x1b[0;37m"
	crlf       = "\r\n"
)

// labelLine renders a "label: value" row, colored, padding the label.
func labelLine(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "%s%-10s%s %s%s%s", sgrCyan, label, sgrReset, sgrGray, value, sgrReset)
	b.WriteString(crlf)
}

// ProfileCard renders one user's profile as an ANSI block (raw SGR, CRLF),
// suitable for writing straight to a telnet session.
func ProfileCard(u store.User) string {
	d := func(zero bool, val string) string {
		if zero {
			return "never"
		}
		return val
	}

	var b strings.Builder

	const width = 76
	rule := strings.Repeat("-", width-2)

	// Header.
	b.WriteString(sgrMagenta + "." + rule + "." + sgrReset + crlf)
	fmt.Fprintf(&b, "%s  %s%s%s", sgrMagenta, sgrYellow, u.Handle, sgrReset)
	if u.RealName != "" {
		fmt.Fprintf(&b, " %s(%s)%s", sgrGray, u.RealName, sgrReset)
	}
	b.WriteString(crlf)
	b.WriteString(sgrMagenta + "`" + rule + "'" + sgrReset + crlf)

	// Body rows.
	labelLine(&b, "group", orDash(u.Group))
	labelLine(&b, "location", orDash(u.Location))
	labelLine(&b, "tagline", orDash(u.Tagline))
	labelLine(&b, "sl/dsl", fmt.Sprintf("%d / %d", u.SL, u.DSL))
	labelLine(&b, "posts", fmt.Sprintf("%d", u.Posts))
	labelLine(&b, "calls", fmt.Sprintf("%d", u.Calls))
	labelLine(&b, "first", d(u.FirstCall.IsZero(), u.FirstCall.Format("2006-01-02")))
	labelLine(&b, "last", d(u.LastCall.IsZero(), u.LastCall.Format("2006-01-02")))
	if titles := badges.Titles(u); len(titles) > 0 {
		labelLine(&b, "badges", strings.Join(titles, ", "))
	}

	b.WriteString(sgrMagenta + "`" + rule + "'" + sgrReset + crlf)
	b.WriteString(sgrReset)
	return b.String()
}

// orDash returns s, or "-" if s is empty.
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// renderList renders one heading + ranked rows of (handle, value).
func renderList(b *strings.Builder, heading string, users []store.User, value func(store.User) string) {
	fmt.Fprintf(b, "%s%s%s%s", sgrGreen, heading, sgrReset, crlf)
	if len(users) == 0 {
		fmt.Fprintf(b, "%s   (none)%s%s", sgrGray, sgrReset, crlf)
		return
	}
	const dotWidth = 60
	for i, u := range users {
		val := value(u)
		// Plain layout width: "NN. handle " + dots + " value".
		used := len(fmt.Sprintf("%2d. %s ", i+1, u.Handle)) + 1 + len(val)
		pad := dotWidth - used
		if pad < 1 {
			pad = 1
		}
		fmt.Fprintf(b, "%s%2d.%s %s%s%s %s%s%s %s%s%s%s",
			sgrYellow, i+1, sgrReset,
			sgrBold, u.Handle, sgrReset,
			sgrGray, strings.Repeat(".", pad), sgrReset,
			sgrCyan, val, sgrReset, crlf)
	}
}

// LeaderBoard renders the rankings as an ANSI block (raw SGR, CRLF).
func LeaderBoard(l Leaders) string {
	var b strings.Builder
	const width = 76
	rule := strings.Repeat("=", width-2)

	b.WriteString(sgrMagenta + "[" + rule + "]" + sgrReset + crlf)
	fmt.Fprintf(&b, "%s   V E N D E T T A / X   --   L E A D E R   B O A R D%s%s", sgrYellow, sgrReset, crlf)
	b.WriteString(sgrMagenta + "[" + rule + "]" + sgrReset + crlf)
	b.WriteString(crlf)

	renderList(&b, "Top Posters", l.TopPosters, func(u store.User) string {
		return fmt.Sprintf("%d", u.Posts)
	})
	b.WriteString(crlf)
	renderList(&b, "Top Callers", l.TopCallers, func(u store.User) string {
		return fmt.Sprintf("%d", u.Calls)
	})
	b.WriteString(crlf)
	renderList(&b, "Newest Users", l.NewestUsers, func(u store.User) string {
		if u.FirstCall.IsZero() {
			return "never"
		}
		return u.FirstCall.Format("2006-01-02")
	})

	b.WriteString(sgrMagenta + "[" + rule + "]" + sgrReset + crlf)
	b.WriteString(sgrReset)
	return b.String()
}
