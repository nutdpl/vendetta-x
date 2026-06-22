package term

import (
	"sync/atomic"
	"time"
)

// Idle-session reaping. A blocked ReadKey holds its goroutine (and any node
// slot) until the caller sends input -- so a caller who connects and then walks
// away, or a slow-drip bot that passed the gate, can pin resources forever.
// The watchdog below closes the connection once no input has arrived for the
// idle window, which unblocks the read (it returns EOF) and ends the session.
// It works over any transport because it closes rwc directly, covering both
// telnet sockets and SSH channels (which don't support read deadlines).

// markActivity stamps the time of the latest read; ReadKey calls it, so every
// keypress (and thus ReadLine/ReadPassword, which loop on ReadKey) refreshes it.
func (s *Session) markActivity() {
	atomic.StoreInt64(&s.lastRead, time.Now().UnixNano())
}

// SetIdleTimeout starts a one-shot watchdog that drops the session after d of
// no input. d <= 0 disables it. Safe to call once per session, before the board
// flow begins.
func (s *Session) SetIdleTimeout(d time.Duration) {
	if d <= 0 {
		return
	}
	s.idleOnce.Do(func() {
		s.markActivity()
		go s.idleWatch(d)
	})
}

func (s *Session) idleWatch(d time.Duration) {
	tick := d / 2
	if tick < time.Second {
		tick = time.Second
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			last := atomic.LoadInt64(&s.lastRead)
			if time.Since(time.Unix(0, last)) >= d {
				// Close the raw connection (not the full Session.Close, to avoid
				// racing the writer's Flush) -- the blocked read returns EOF.
				s.rwc.Close()
				return
			}
		}
	}
}
