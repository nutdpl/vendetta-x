package term

// Terminal window size (NAWS on telnet, pty-req/window-change on ssh). The
// board's guaranteed floor is 80x24, so an unknown or zero dimension reports
// the floor; a client that tells us it's bigger gets the real number. Stored
// atomically because ssh window-change can arrive on a different goroutine than
// the session read loop.

// Cols returns the terminal width in columns (default 80).
func (s *Session) Cols() int {
	if c := s.cols.Load(); c > 0 {
		return int(c)
	}
	return 80
}

// Rows returns the terminal height in rows (default 24).
func (s *Session) Rows() int {
	if r := s.rows.Load(); r > 0 {
		return int(r)
	}
	return 24
}

// SetWinSize records a reported terminal size. A zero or absurd dimension is
// ignored (kept at whatever was known before, or the floor), so a bogus report
// can never shrink a screen to nothing.
func (s *Session) SetWinSize(cols, rows int) {
	if cols > 0 && cols <= 1000 {
		s.cols.Store(int32(cols))
	}
	if rows > 0 && rows <= 1000 {
		s.rows.Store(int32(rows))
	}
}
