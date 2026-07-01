package term

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// clientEnd drives the far side of a net.Pipe: collects what the session
// writes and (optionally) answers its charset probe like a terminal would.
func startSession(t *testing.T) (*Session, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	s := NewRW(server, "test")
	t.Cleanup(func() { s.Close(); client.Close() })
	return s, client
}

// TestDetectCharsetUTF8 answers the CPR probe like a UTF-8 terminal (the
// 3-byte ─ advanced one column: cursor at column 2).
func TestDetectCharsetUTF8(t *testing.T) {
	s, client := startSession(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64)
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := client.Read(buf)
		if !bytes.Contains(buf[:n], []byte("\x1b[6n")) {
			t.Errorf("no CPR request in probe: %q", buf[:n])
			return
		}
		client.Write([]byte("\x1b[1;2R"))
		// swallow the erase sequence
		client.Read(buf)
	}()
	s.DetectCharset(time.Second)
	<-done
	if !s.IsUTF8() {
		t.Fatal("UTF-8 terminal not detected")
	}
}

// TestDetectCharsetCP437 answers like a CP437 terminal (three glyphs: col 4).
func TestDetectCharsetCP437(t *testing.T) {
	s, client := startSession(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64)
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		client.Read(buf)
		client.Write([]byte("\x1b[1;4R"))
		client.Read(buf)
	}()
	s.DetectCharset(time.Second)
	<-done
	if s.IsUTF8() {
		t.Fatal("CP437 terminal misdetected as UTF-8")
	}
}

// TestDetectCharsetNoAnswer: a client that never replies stays CP437.
func TestDetectCharsetNoAnswer(t *testing.T) {
	s, client := startSession(t)
	go io.Copy(io.Discard, client) // read the probe, never answer
	s.DetectCharset(150 * time.Millisecond)
	if s.IsUTF8() {
		t.Fatal("silent client misdetected as UTF-8")
	}
}

// TestOutputTranscodes verifies Print in UTF-8 mode expands CP437 art bytes
// and leaves them alone in native mode.
func TestOutputTranscodes(t *testing.T) {
	s, client := startSession(t)
	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := client.Read(buf)
		got <- append([]byte(nil), buf[:n]...)
	}()
	s.SetUTF8(true)
	s.Print("\x1b[1;35m\xb0\xc4\x04")
	s.Flush()
	out := <-got
	want := []byte("\x1b[1;35m░─♦")
	if !bytes.Equal(out, want) {
		t.Fatalf("transcoded output = %q, want %q", out, want)
	}
}

// TestInputFoldsUTF8 verifies a typed multibyte char folds to its CP437 byte.
func TestInputFoldsUTF8(t *testing.T) {
	s, client := startSession(t)
	go client.Write([]byte("é日")) // U+00E9 -> CP437 0x82; U+65E5 -> '?'
	s.SetUTF8(true)
	if k, ch := s.ReadKey(); k != KeyChar || ch != 0x82 {
		t.Fatalf("é: key=%v ch=0x%02X, want KeyChar,0x82", k, ch)
	}
	if k, ch := s.ReadKey(); k != KeyChar || ch != '?' {
		t.Fatalf("日: key=%v ch=%q, want KeyChar,'?'", k, ch)
	}
}

// TestSetTermType covers the SSH-side heuristic.
func TestSetTermType(t *testing.T) {
	cases := []struct {
		term string
		utf8 bool
	}{
		{"xterm-256color", true}, {"vt220", true}, {"screen", true},
		{"tmux-256color", true}, {"alacritty", true}, {"linux", true},
		{"syncterm", false}, {"ansi-bbs", false}, {"ansi", false},
		{"netrunner", false}, {"", false}, {"weirdo-term", false},
	}
	for _, c := range cases {
		s := &Session{}
		s.SetTermType(c.term)
		if s.IsUTF8() != c.utf8 {
			t.Errorf("SetTermType(%q): utf8=%v, want %v", c.term, s.IsUTF8(), c.utf8)
		}
	}
	// explicit: TERM must be matched case-insensitively
	s := &Session{}
	s.SetTermType(strings.ToUpper("SyncTERM"))
	if s.IsUTF8() {
		t.Error("SYNCTERM (upper) misdetected as UTF-8")
	}
}
