// Package throttle is a tiny in-memory failure limiter keyed by a string
// (typically a client IP), used to slow down login brute-forcing on every face.
// It records failures in a sliding window and reports when a key is over the
// limit; a successful login resets the key.
package throttle

import (
	"sync"
	"time"
)

// Throttle limits repeated failures per key within a sliding time window.
type Throttle struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	fails  map[string][]time.Time
}

// New returns a Throttle that blocks a key once it has max failures within
// window.
func New(max int, window time.Duration) *Throttle {
	return &Throttle{max: max, window: window, fails: map[string][]time.Time{}}
}

// Blocked reports whether key has reached the failure limit within the window.
func (t *Throttle) Blocked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.recent(key)) >= t.max
}

// Fail records a failed attempt for key.
func (t *Throttle) Fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fails[key] = append(t.recent(key), time.Now())
}

// Reset clears a key's failures (call on a successful login).
func (t *Throttle) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, key)
}

// recent returns key's failures still inside the window, pruning older ones and
// dropping the key entirely when it falls empty (so the map can't grow without
// bound). The caller must hold the lock.
func (t *Throttle) recent(key string) []time.Time {
	cutoff := time.Now().Add(-t.window)
	in := t.fails[key]
	out := in[:0]
	for _, ts := range in {
		if ts.After(cutoff) {
			out = append(out, ts)
		}
	}
	if len(out) == 0 {
		delete(t.fails, key)
		return nil
	}
	t.fails[key] = out
	return out
}
