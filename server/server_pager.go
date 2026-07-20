package main

import (
	"strings"

	"vendetta-x/server/internal/term"
)

// Per-reader header/footer margins subtracted from the real terminal height to
// get the "more" pager's body budget. The board honors the client's reported
// window size (NAWS on telnet, pty-req on ssh) and falls back to the 80x24
// floor, so a taller terminal simply pages fewer times.
const (
	gfileHeaderRows = 4 // g-files: title/category/author + rule
	msgFrameRows    = 9 // messages: the framed msgread header + the nav footer
)

// bodyRows returns how many body lines to show before the "more" prompt on the
// caller's actual terminal, never fewer than a handful.
func bodyRows(s *term.Session, margin int) int {
	if r := s.Rows() - margin; r >= 4 {
		return r
	}
	return 4
}

// pageLines prints body lines indented in the reader's dim-white idiom,
// pausing with a classic "-- more --" prompt every `rows` lines so long text
// never scrolls off a short terminal. At the prompt: Space / N pages forward,
// Enter advances one line, C shows the rest continuously, Q / Esc stops.
// Returns false when the caller quit early (the rest was suppressed), true when
// all lines were shown.
func (b *board) pageLines(s *term.Session, lines []string, rows int) bool {
	if rows < 1 {
		rows = 1
	}
	shown := 0
	continuous := false
	for _, ln := range lines {
		s.Printf("  \x1b[0;37m%s\x1b[0m\r\n", ln)
		shown++
		if continuous || shown < rows {
			continue
		}
		s.Print("\x1b[1;30m  -- more -- \x1b[0;37m[\x1b[1;37mSpace\x1b[0;37m] page \xfa " +
			"[\x1b[1;37mEnter\x1b[0;37m] line \xfa [\x1b[1;37mC\x1b[0;37m]ont \xfa [\x1b[1;37mQ\x1b[0;37m]uit\x1b[0m")
		s.Flush()
		k, ch := s.ReadKey()
		s.Print("\r\x1b[K") // wipe the prompt line before the next row
		switch {
		case k == term.KeyEsc, k == term.KeyEOF, k == term.KeyChar && lc(ch) == 'q':
			return false
		case k == term.KeyChar && lc(ch) == 'c':
			continuous = true
		case k == term.KeyEnter, k == term.KeyDown:
			shown = rows - 1 // reveal a single line, then prompt again
		default:
			shown = 0 // a fresh page
		}
	}
	return true
}

// pageText is pageLines over a raw body split on newlines -- the common case
// for stored documents and message bodies.
func (b *board) pageText(s *term.Session, body string, rows int) bool {
	return b.pageLines(s, strings.Split(body, "\n"), rows)
}
