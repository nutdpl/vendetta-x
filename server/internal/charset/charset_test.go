package charset

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func TestKnownGlyphs(t *testing.T) {
	cases := []struct {
		b byte
		r rune
	}{
		{0xB0, '░'}, {0xB1, '▒'}, {0xB2, '▓'}, {0xDB, '█'},
		{0xC4, '─'}, {0xCD, '═'}, {0xB3, '│'}, {0xBA, '║'},
		{0xDF, '▀'}, {0xDC, '▄'}, {0x04, '♦'}, {0xFA, '·'},
		{0xF9, '∙'}, {0xFE, '■'}, {'A', 'A'}, {0x1B, 0x1B},
	}
	for _, c := range cases {
		if got := Rune(c.b); got != c.r {
			t.Errorf("Rune(0x%02X) = %q, want %q", c.b, got, c.r)
		}
	}
}

func TestEncodePassesANSIThrough(t *testing.T) {
	in := []byte("\x1b[1;35mplain ascii\x1b[0m")
	if got := EncodeToUTF8(in); !bytes.Equal(got, in) {
		t.Fatalf("ASCII/ANSI changed: %q -> %q", in, got)
	}
}

func TestEncodeArtBytes(t *testing.T) {
	got := EncodeToUTF8([]byte{0xB0, 'x', 0xC4})
	want := []byte("░x─")
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeToUTF8 = % x, want % x", got, want)
	}
	if !utf8.Valid(got) {
		t.Fatal("output is not valid UTF-8")
	}
}

func TestReverseMap(t *testing.T) {
	// every CP437 byte must round-trip through its rune
	for i := 0; i < 256; i++ {
		b, ok := RuneToCP437(Rune(byte(i)))
		if !ok || (b != byte(i) && Rune(b) != Rune(byte(i))) {
			t.Errorf("round-trip 0x%02X -> %q -> 0x%02X (ok=%v)", i, Rune(byte(i)), b, ok)
		}
	}
	// unmappable rune folds to '?'
	if b, ok := RuneToCP437('日'); ok || b != '?' {
		t.Errorf("unmappable = 0x%02X ok=%v, want '?',false", b, ok)
	}
}
