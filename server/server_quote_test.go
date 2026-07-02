package main

import (
	"strings"
	"testing"
)

func TestQuoteLinesBasic(t *testing.T) {
	got := quoteLines("nut", "first line\n\nsecond line", 72)
	want := []string{"nut> first line", "nut>", "nut> second line", ""}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestQuoteLinesWraps(t *testing.T) {
	long := strings.Repeat("word ", 40) // ~200 chars, must wrap
	got := quoteLines("phantom", long, 72)
	if len(got) < 3 {
		t.Fatalf("expected wrapped lines, got %q", got)
	}
	for _, ln := range got[:len(got)-1] {
		if !strings.HasPrefix(ln, "phantom>") {
			t.Errorf("line %q lacks quote prefix", ln)
		}
		if len(ln) > 72 {
			t.Errorf("line %q exceeds width (%d)", ln, len(ln))
		}
	}
}

func TestQuoteLinesCapsLongPosts(t *testing.T) {
	long := strings.TrimRight(strings.Repeat("line\n", 60), "\n")
	got := quoteLines("nut", long, 72)
	if len(got) != quoteCap+2 { // capped lines + elision marker + blank
		t.Fatalf("got %d lines, want %d", len(got), quoteCap+2)
	}
	if got[quoteCap] != "nut> [...]" {
		t.Errorf("elision marker = %q", got[quoteCap])
	}
	if got[len(got)-1] != "" {
		t.Errorf("expected trailing blank line, got %q", got[len(got)-1])
	}
}

func TestQuoteLinesEmptyBody(t *testing.T) {
	if got := quoteLines("nut", "   \n\n  ", 72); got != nil {
		t.Fatalf("blank body should quote nothing, got %q", got)
	}
}
