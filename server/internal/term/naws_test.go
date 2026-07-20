package term

import "testing"

func TestWinSizeDefaultsAndClamp(t *testing.T) {
	s, _ := pair(t)
	if s.Cols() != 80 || s.Rows() != 24 {
		t.Fatalf("defaults = %dx%d, want 80x24", s.Cols(), s.Rows())
	}
	s.SetWinSize(132, 43)
	if s.Cols() != 132 || s.Rows() != 43 {
		t.Fatalf("after set = %dx%d, want 132x43", s.Cols(), s.Rows())
	}
	// Zero / absurd values are ignored (never shrink to nothing).
	s.SetWinSize(0, 0)
	s.SetWinSize(99999, -1)
	if s.Cols() != 132 || s.Rows() != 43 {
		t.Fatalf("bogus sizes changed the window to %dx%d", s.Cols(), s.Rows())
	}
}

func TestNAWSSubnegotiationParsed(t *testing.T) {
	s, cli := pair(t)
	// IAC SB NAWS width(0,132) height(0,43) IAC SE, then a plain 'x'.
	go func() {
		cli.Write([]byte{255, 250, 31, 0, 132, 0, 43, 255, 240, 'x'})
	}()
	k, ch := s.ReadKey()
	if k != KeyChar || ch != 'x' {
		t.Fatalf("ReadKey after NAWS = (%v,%q), want the 'x' that followed", k, ch)
	}
	if s.Cols() != 132 || s.Rows() != 43 {
		t.Fatalf("NAWS not applied: %dx%d, want 132x43", s.Cols(), s.Rows())
	}
}

func TestNAWSWithEscapedByte(t *testing.T) {
	s, cli := pair(t)
	// A width of 255 must arrive as IAC IAC (255 255) inside the SB. Height 40.
	go func() {
		cli.Write([]byte{255, 250, 31, 255, 255, 255, 255, 0, 40, 255, 240, 'y'})
	}()
	k, ch := s.ReadKey()
	if k != KeyChar || ch != 'y' {
		t.Fatalf("ReadKey = (%v,%q), want 'y'", k, ch)
	}
	// width = 0xFFFF = 65535 is clamped out (>1000), height 40 applies.
	if s.Rows() != 40 {
		t.Fatalf("rows = %d, want 40", s.Rows())
	}
}
