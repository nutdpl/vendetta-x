package store

// Ratio-economy policy lives here so every face (telnet/ssh via main, and the
// web handlers) computes download eligibility identically. The numbers are
// policy; formatting the caller-facing messaging is left to each face.

// RatioEnabled reports whether download-ratio enforcement is on. Off by
// default: a fresh board doesn't gate downloads until a sysop opts in.
func (s *Store) RatioEnabled() bool {
	return s.SettingBool("files.ratio.enabled", false)
}

// RatioBytes is the download-to-upload byte ratio (downloads allowed per
// uploaded byte). Default 3:1; never returns < 1.
func (s *Store) RatioBytes() int {
	if n := s.SettingInt("files.ratio.bytes", 3); n > 0 {
		return n
	}
	return 3
}

// RatioFreeBytes is the download credit every caller gets regardless of
// uploads, so newcomers and small grabs are never blocked. Default 5 MiB.
func (s *Store) RatioFreeBytes() int64 {
	return int64(s.SettingInt("files.ratio.free_mb", 5)) << 20
}

// RatioExempt reports whether a user bypasses ratio entirely. Privileged
// accounts (sysops) always are.
func (s *Store) RatioExempt(u *User) bool {
	return u.Privileged()
}

// DownloadAllowance returns how many more bytes u may download right now:
// (uploaded * ratio) + free - already-downloaded. Can be negative if the user
// is already in the hole (only possible after a policy change).
func (s *Store) DownloadAllowance(u *User) int64 {
	return u.UlBytes*int64(s.RatioBytes()) + s.RatioFreeBytes() - u.DlBytes
}

// RatioBlocks reports whether downloading a file of the given size is
// currently disallowed for u under the ratio policy.
func (s *Store) RatioBlocks(u *User, size int64) bool {
	if !s.RatioEnabled() || s.RatioExempt(u) {
		return false
	}
	return size > s.DownloadAllowance(u)
}
