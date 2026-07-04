package main

import (
	"fmt"

	"vendetta-x/server/internal/store"
)

// The ratio economy: callers must contribute uploads to keep downloading.
// "Ratio is law" -- but a law the sysop can switch off (it's off by default)
// and that never traps a brand-new caller (there's a free byte allowance).
// The numeric policy lives in the store (store/ratio.go) so the web face
// enforces it identically; this file is just the telnet/ssh formatting.

func (b *board) ratioEnabled() bool                    { return b.st.RatioEnabled() }
func (b *board) ratioExempt(user *store.User) bool     { return b.st.RatioExempt(user) }
func (b *board) downloadAllowance(u *store.User) int64 { return b.st.DownloadAllowance(u) }

// ratioBlocksDownload reports whether the ratio would block downloading a file
// of the given size, and a caller-facing reason line if so.
func (b *board) ratioBlocksDownload(user *store.User, size int64) (bool, string) {
	if !b.st.RatioBlocks(user, size) {
		return false, ""
	}
	return true, fmt.Sprintf(
		"Ratio is law. You have %s of download credit left; that file is %s. Upload something.",
		sizeStr(b.st.DownloadAllowance(user)), sizeStr(size))
}

// ratioLine renders the caller's ratio standing for the stats screen.
func (b *board) ratioLine(user *store.User) string {
	up, down := user.UlBytes, user.DlBytes
	var ratio string
	switch {
	case up == 0 && down == 0:
		ratio = "--"
	case up == 0:
		ratio = "\xec" // infinity glyph in CP437: all take, no give
	default:
		ratio = fmt.Sprintf("%.2f:1", float64(down)/float64(up))
	}
	s := fmt.Sprintf("%d up (%s) / %d down (%s) \xb7 ratio %s",
		user.Uploads, sizeStr(up), user.Downloads, sizeStr(down), ratio)
	if b.ratioEnabled() && !b.ratioExempt(user) {
		s += fmt.Sprintf(" \xb7 %s credit left", sizeStr(b.downloadAllowance(user)))
	}
	return s
}
