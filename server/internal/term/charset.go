package term

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"vendetta-x/server/internal/charset"
)

// Charset handling: the board's native tongue is CP437 -- the art, the box
// drawing, the shade blocks are all raw high-bit bytes that SyncTERM-family
// clients render natively. A stock modern terminal (macOS Terminal, GNOME
// Terminal, a Linux box's ssh) expects UTF-8 and shows mojibake instead.
// The session can therefore run in UTF-8 mode: output transcodes CP437 bytes
// to their Unicode glyphs, and typed UTF-8 folds back to CP437 (or '?') so
// stored text stays single-byte and renders for every kind of caller.
//
// Mode is decided per session: the telnet face probes the terminal
// (DetectCharset), the SSH face maps the TERM string the client sent in its
// pty request (SetTermType). Raw passthrough paths -- doors and ZMODEM
// transfers -- are deliberately NOT transcoded: binary must see wire bytes.

// SetUTF8 switches output transcoding / input folding on or off.
func (s *Session) SetUTF8(on bool) { s.utf8 = on }

// IsUTF8 reports whether the session is transcoding for a UTF-8 terminal.
func (s *Session) IsUTF8() bool { return s.utf8 }

// CharsetName names the session's wire charset for display ("utf-8"/"cp437").
func (s *Session) CharsetName() string {
	if s.utf8 {
		return "utf-8"
	}
	return "cp437"
}

// ---- output ----------------------------------------------------------------

// emitBytes writes b to the buffered writer, transcoding CP437->UTF-8 when
// the session is in UTF-8 mode. ANSI escapes are pure ASCII and pass through
// a per-byte transcode untouched.
func (s *Session) emitBytes(b []byte) {
	if !s.utf8 {
		s.bw.Write(b)
		return
	}
	s.bw.Write(charset.EncodeToUTF8(b))
}

func (s *Session) emitByte(b byte) {
	if !s.utf8 {
		s.bw.WriteByte(b)
		return
	}
	s.bw.Write(charset.EncodeToUTF8([]byte{b}))
}

// emitWriter adapts the emit path to io.Writer for fmt.Fprintf.
type emitWriter struct{ s *Session }

func (w emitWriter) Write(p []byte) (int, error) {
	w.s.emitBytes(p)
	return len(p), nil
}

// ---- input -----------------------------------------------------------------

// foldUTF8Input is called by ReadKey when a UTF-8 session reads a lead byte
// (>= 0x80): it consumes the sequence's continuation bytes (only those
// already buffered -- a terminal sends the whole sequence in one burst, and
// blocking for bytes that may never come would hang the key loop) and folds
// the rune to its CP437 byte, or '?' when it has no CP437 glyph.
func (s *Session) foldUTF8Input(lead byte) byte {
	var need int
	switch {
	case lead&0xE0 == 0xC0:
		need = 1
	case lead&0xF0 == 0xE0:
		need = 2
	case lead&0xF8 == 0xF0:
		need = 3
	default:
		return '?' // stray continuation or invalid lead
	}
	if s.br.Buffered() < need {
		return '?'
	}
	seq := make([]byte, 1, 4)
	seq[0] = lead
	for i := 0; i < need; i++ {
		nb, err := s.br.ReadByte()
		if err != nil {
			return '?'
		}
		seq = append(seq, nb)
	}
	r, _ := utf8.DecodeRune(seq)
	if r == utf8.RuneError {
		return '?'
	}
	b, _ := charset.RuneToCP437(r)
	return b
}

// ---- detection -------------------------------------------------------------

// DetectCharset probes whether the terminal renders UTF-8 or CP437 and sets
// the session mode. It prints a 3-byte UTF-8 sequence (U+2500 ─) at column 1
// and asks for a cursor position report (ESC[6n): a UTF-8 terminal shows one
// glyph (cursor lands on column 2), a CP437 terminal shows three (column 4).
// The probe line is then erased.
//
// The read is strictly bounded: it needs the transport's read deadline, so it
// only runs on the telnet face (SSH channels have no deadline -- the SSH face
// decides by TERM instead, see SetTermType). A client that never answers CPR
// (netcat, dumb scanners) times out and stays CP437, the board's native mode.
func (s *Session) DetectCharset(wait time.Duration) {
	if s.dl == nil {
		return
	}
	s.bw.Write([]byte("\r\xe2\x94\x80\x1b[6n"))
	s.bw.Flush()

	col, ok := s.readCPR(wait)
	// erase the probe (up to 3 glyphs on a CP437 terminal) and rehome
	s.bw.Write([]byte("\r   \r"))
	s.bw.Flush()
	if ok && col == 2 {
		s.utf8 = true
	}
}

// readCPR reads a cursor position report (ESC [ row ; col R) off the wire,
// returning the column. Bounded by the deadline; junk before the report is
// skipped, and everything consumed is confined to what's arrived.
func (s *Session) readCPR(wait time.Duration) (col int, ok bool) {
	if err := s.dl.SetReadDeadline(time.Now().Add(wait)); err != nil {
		return 0, false
	}
	defer s.dl.SetReadDeadline(time.Time{})

	var cur strings.Builder
	inSeq := false
	for i := 0; i < 64; i++ { // hard cap: a report is ~8 bytes; 64 allows junk
		b, err := s.br.ReadByte()
		if err != nil {
			return 0, false // deadline hit or connection gone
		}
		switch {
		case b == 0x1B:
			inSeq = true
			cur.Reset()
		case inSeq && b == 'R':
			var row int
			if n, _ := fmt.Sscanf(cur.String(), "[%d;%d", &row, &col); n == 2 {
				return col, true
			}
			inSeq = false
		case inSeq:
			cur.WriteByte(b)
		}
	}
	return 0, false
}

// ---- SSH TERM heuristic ------------------------------------------------------

// SetTermType decides the charset from an SSH client's TERM string. Retro BBS
// clients identify themselves plainly (SyncTERM sends "syncterm", NetRunner
// "ansi-bbs"-family values); everything a modern OS ships (xterm-*, vt220,
// screen, tmux, alacritty, kitty, ...) is a UTF-8 terminal. Unknown or empty
// stays CP437 -- the board's native mode, and what a scene caller expects.
func (s *Session) SetTermType(term string) {
	t := strings.ToLower(strings.TrimSpace(term))
	switch {
	case t == "":
		return
	case strings.Contains(t, "syncterm"),
		strings.Contains(t, "ansi"), // ansi, ansi-bbs, pcansi
		strings.Contains(t, "netrunner"),
		strings.Contains(t, "magiterm"),
		strings.Contains(t, "cterm"),
		strings.Contains(t, "dos"):
		s.utf8 = false
	case strings.HasPrefix(t, "xterm"),
		strings.HasPrefix(t, "vt"),
		strings.HasPrefix(t, "screen"),
		strings.HasPrefix(t, "tmux"),
		strings.HasPrefix(t, "rxvt"),
		strings.HasPrefix(t, "linux"),
		strings.HasPrefix(t, "alacritty"),
		strings.HasPrefix(t, "kitty"),
		strings.HasPrefix(t, "foot"),
		strings.HasPrefix(t, "wezterm"),
		strings.HasPrefix(t, "st-"), t == "st":
		s.utf8 = true
	}
}
