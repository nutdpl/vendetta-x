package sanitize

import "testing"

func TestLineStripsControls(t *testing.T) {
	cases := []struct{ in, want string }{
		{"phiber", "phiber"},
		{"\x1b[2J\x1b[31mowned", "[2J[31mowned"}, // ESC bytes gone, text remains
		{"a\tb\rc\nd", "abcd"},                   // tab/cr/lf stripped on a line
		{"clean", "clean"},
		{"\x07bell\x7f", "bell"},
		{"box\xdb\xc4", "box\xdb\xc4"}, // CP437 high bytes preserved
	}
	for _, c := range cases {
		if got := Line(c.in); got != c.want {
			t.Errorf("Line(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// No ESC byte may survive.
	if got := Line("x\x1by"); got != "xy" {
		t.Errorf("ESC not stripped: %q", got)
	}
}

func TestTextKeepsNewlines(t *testing.T) {
	in := "line one\r\nline two\rline three\x1b[31m\ttabbed"
	want := "line one\nline two\nline three[31mtabbed"
	if got := Text(in); got != want {
		t.Errorf("Text(%q) = %q, want %q", in, got, want)
	}
	if got := Text("a\x1bb"); got != "ab" {
		t.Errorf("ESC not stripped in Text: %q", got)
	}
}
