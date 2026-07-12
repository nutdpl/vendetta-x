package main

import (
	"strings"
	"testing"

	"vendetta-x/server/internal/term"
)

// TestExpertLogonSkipsTourAndClock proves expert mode skips the quick-logon
// prompt and the 12-hour clock preference renders the caller's last-call time.
func TestExpertLogonSkipsTourAndClock(t *testing.T) {
	b := newTestBoard(t)
	// Expert + 12h clock, with a fixed last-call time (2026-07-01 15:30 local).
	b.st.DB().Exec(`UPDATE users SET expert=1, clock12=1, last_call=strftime('%s','2026-07-01 15:30:00'), calls=5 WHERE handle='nut'`)
	user, _ := b.st.UserByHandle("nut")

	se := runOnSession(t, b, func(s *term.Session) {
		b.logon(s, map[string]string{}, user)
	})
	se.drain()
	se.waitDone()

	out := string(se.out)
	if strings.Contains(out, "Quick logon?") {
		t.Error("expert mode should skip the quick-logon prompt")
	}
	if !strings.Contains(out, "pm") {
		t.Errorf("12-hour clock should render an am/pm time; output:\n%s", out)
	}
}
