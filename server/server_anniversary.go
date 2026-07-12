package main

import "time"

// isBirthdayToday reports whether the canonical "MM-DD" birthday falls on now.
// An empty birthday never matches.
func isBirthdayToday(birthday string, now time.Time) bool {
	return birthday != "" && birthday == now.Format("01-02")
}

// anniversaryYears returns how many full years today marks since firstCall, but
// only when today is that same month-day (otherwise 0). A Feb-29 first call
// only rings on Feb 29. A zero or same-year first call yields 0.
func anniversaryYears(firstCall, now time.Time) int {
	if firstCall.IsZero() || firstCall.Format("01-02") != now.Format("01-02") {
		return 0
	}
	if years := now.Year() - firstCall.Year(); years > 0 {
		return years
	}
	return 0
}
