// Package sanitize strips terminal control bytes from user-supplied text so a
// caller can't smuggle ANSI escape sequences into fields that are later echoed
// onto other callers' terminals (handles, taglines, oneliners, subjects, ...).
// A tagline of "\x1b[2J\x1b[31mowned" must not clear or recolor someone else's
// screen when it scrolls past in the user list.
//
// The board is byte-oriented CP437, so these work at the byte level: C0 control
// bytes (0x00-0x1F) and DEL (0x7F) are dropped, while printable ASCII and the
// high range (0x80-0xFF, the CP437 box-drawing/glyph bytes) are preserved.
package sanitize

import "strings"

// Line cleans a single-line field: every control byte goes, including ESC, CR,
// LF and TAB.
func Line(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 0x20 && c != 0x7f {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Text cleans a multi-line field (message/mail bodies): newlines are kept (CRLF
// and lone CR are normalised to LF first), every other control byte is dropped.
func Text(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || (c >= 0x20 && c != 0x7f) {
			b.WriteByte(c)
		}
	}
	return b.String()
}
