// Animation and pacing primitives for the session layer: a carrier-safe sleep,
// an interruptible timed key wait, and a line-by-line "reveal" that paints art
// onto the wire the way a real board does over a modem. These turn the static
// RenderScreen into the cinematic connect/login entrance.
package term

import (
	"bytes"
	"errors"
	"net"
	"os"
	"time"

	"vendetta-x/server/internal/render"
)

// sleep waits d, but wakes immediately if the session is torn down (idle
// watchdog / carrier loss) so a paced screen never holds a dead socket open.
func (s *Session) sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-s.done:
	}
}

// Sleep flushes pending output, then pauses for d (interruptible by teardown).
// Flushing first matters: the bytes that set up the pause must hit the wire
// before we block, or the caller sees nothing happen.
func (s *Session) Sleep(d time.Duration) {
	s.Flush()
	s.sleep(d)
}

// isTimeout reports whether err is a read-deadline expiry rather than a real
// end of stream. Both surface as KeyEOF from ReadKey; only the latter means the
// caller hung up.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// WaitKey blocks up to d for a keypress, returning true if one arrived. A
// deadline expiry returns false (keep going); a real hangup also returns false
// but is detected on the next blocking read. Transports without a read deadline
// (SSH channels) can't peek without blocking, so there WaitKey just paces like
// Sleep and reports no key -- the reveal still animates, it just isn't skippable.
func (s *Session) WaitKey(d time.Duration) bool {
	s.Flush()
	if s.dl == nil {
		s.sleep(d)
		return false
	}
	if err := s.dl.SetReadDeadline(time.Now().Add(d)); err != nil {
		s.sleep(d)
		return false
	}
	k, ch := s.ReadKey()
	s.dl.SetReadDeadline(time.Time{})
	if k == KeyEOF {
		return false // either the pause elapsed or the caller hung up
	}
	// A real key: stash it so the screen that follows the animation still sees
	// it. Hammering a hotkey through the intro skips it AND selects the option.
	s.setPending(k, ch)
	return true
}

// setPending stashes a one-key pushback under readMu (so it never races a
// concurrent ReadKey). takePending consumes a stashed key, reporting whether one
// was present.
func (s *Session) setPending(k Kind, ch byte) {
	s.readMu.Lock()
	s.pending = &keyEvent{k: k, ch: ch}
	s.readMu.Unlock()
}

func (s *Session) takePending() bool {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	if s.pending != nil {
		s.pending = nil
		return true
	}
	return false
}

// WaitAnyKey waits up to d for a keypress and consumes it, used to gate a splash
// on "press any key". Unlike WaitKey it does NOT push the key back: dismissing
// the splash spends the key, so it doesn't leak into the screen that follows. A
// key already stashed by a preceding animation (the caller skipped ahead) counts
// as the dismissal. d <= 0 (or a transport without a read deadline) just drains
// any pending key and returns.
func (s *Session) WaitAnyKey(d time.Duration) {
	s.Flush()
	if s.takePending() {
		return
	}
	if s.dl == nil || d <= 0 {
		s.sleep(d)
		return
	}
	if err := s.dl.SetReadDeadline(time.Now().Add(d)); err != nil {
		s.sleep(d)
		return
	}
	s.ReadKey()
	s.dl.SetReadDeadline(time.Time{})
}

// Reveal renders a .pp/.ans screen to a buffer, then streams it to the wire one
// line at a time with lineDelay between lines, so the art paints on top-to-bottom
// like a board drawing over a modem. Any keypress dumps the rest instantly. It
// returns the screen's lightbar markers (so it is a drop-in for RenderScreen)
// and whether the caller skipped. lineDelay <= 0 is an instant draw.
func (s *Session) Reveal(path string, tokens map[string]string, lineDelay time.Duration) ([]render.Marker, bool) {
	var markers []render.Marker
	var buf bytes.Buffer
	ctx := &render.Ctx{Tokens: tokens, OnMarker: func(m render.Marker) { markers = append(markers, m) }}
	if err := render.RenderFile(&buf, path, ctx); err != nil {
		// Missing/broken art: fall back to an instant draw rather than crash.
		return s.RenderScreen(path, tokens), false
	}

	data := buf.Bytes()
	skipped := lineDelay <= 0
	for i := 0; i < len(data); {
		end := len(data)
		if nl := bytes.IndexByte(data[i:], '\n'); nl >= 0 {
			end = i + nl + 1
		}
		s.emitBytes(data[i:end])
		i = end
		if skipped {
			continue // already skipping: blast the remaining lines
		}
		if s.WaitKey(lineDelay) {
			skipped = true
		}
	}
	s.Flush()
	return markers, skipped
}
