package main

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

func TestIsBirthdayToday(t *testing.T) {
	now := day(2026, time.July, 12)
	if !isBirthdayToday("07-12", now) {
		t.Error("07-12 should match July 12")
	}
	if isBirthdayToday("07-13", now) {
		t.Error("07-13 should not match July 12")
	}
	if isBirthdayToday("", now) {
		t.Error("an empty birthday should never match")
	}
}

func TestAnniversaryYears(t *testing.T) {
	first := day(2020, time.July, 12)
	cases := []struct {
		now  time.Time
		want int
	}{
		{day(2026, time.July, 12), 6}, // same day, six years on
		{day(2021, time.July, 12), 1}, // first anniversary
		{day(2020, time.July, 12), 0}, // the day itself: not yet a year
		{day(2026, time.July, 11), 0}, // a day early: no ring
		{day(2026, time.August, 12), 0},
	}
	for _, c := range cases {
		if got := anniversaryYears(first, c.now); got != c.want {
			t.Errorf("anniversaryYears(%s) = %d, want %d", c.now.Format("2006-01-02"), got, c.want)
		}
	}
	// A zero first-call never rings.
	if anniversaryYears(time.Time{}, day(2026, time.July, 12)) != 0 {
		t.Error("a zero first call should yield 0")
	}
}
