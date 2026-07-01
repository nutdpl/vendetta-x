package term

import (
	"bufio"
	"io"
	"time"
)

// Transfer hands out an 8-bit-clean io.ReadWriter for a binary file transfer
// (ZMODEM), then returns a cleanup func the caller MUST invoke when done.
//
// Over SSH the channel is already transparent, so this is a thin passthrough
// on the session's buffered reader (no buffered input lost) and raw writer.
// Over telnet the byte 0xFF is the IAC command escape: it must be doubled on
// the way out and collapsed on the way back, and a real telnet command that
// arrives mid-transfer (a stray client keepalive) must be swallowed, not fed
// to the protocol. telnetCodec does exactly that.
//
// The idle watchdog still guards the session during the transfer (a stalled
// transfer eventually trips it and closes rwc, unblocking the read).
func (s *Session) Transfer() (io.ReadWriter, func()) {
	s.bw.Flush()
	if !s.telnet {
		return &passthrough{s: s}, func() {}
	}
	return &telnetCodec{s: s, br: s.br, w: rawSession{s: s}}, func() {}
}

// passthrough is the SSH transfer path: 8-bit clean already, so it just marks
// activity (keeping the idle watchdog from reaping a live transfer) and reads
// from the session's buffered reader / writes to the raw connection.
type passthrough struct{ s *Session }

func (p *passthrough) Read(b []byte) (int, error) {
	p.s.markActivity()
	return p.s.br.Read(b)
}
func (p *passthrough) Write(b []byte) (int, error) { return rawSession{s: p.s}.Write(b) }

// telnetCodec is an 8-bit-clean ReadWriter over a telnet byte stream: it
// doubles outbound IAC (0xFF) and un-doubles inbound IAC, dropping any real
// telnet command sequence that shows up during the transfer.
type telnetCodec struct {
	s  *Session
	br *bufio.Reader
	w  io.Writer
}

const iac = 0xFF

func (t *telnetCodec) ReadByte() (byte, error) {
	t.s.markActivity()
	for {
		b, err := t.br.ReadByte()
		if err != nil {
			return 0, err
		}
		if b != iac {
			return b, nil
		}
		// IAC: peek the next byte to tell doubled-IAC (literal 0xFF) from a
		// telnet command.
		n, err := t.br.ReadByte()
		if err != nil {
			return 0, err
		}
		switch {
		case n == iac:
			return iac, nil // doubled IAC -> one literal 0xFF
		case n == 250: // SB ... IAC SE -- consume through the SE
			for {
				x, err := t.br.ReadByte()
				if err != nil {
					return 0, err
				}
				if x == iac {
					if y, err := t.br.ReadByte(); err != nil {
						return 0, err
					} else if y == 240 {
						break
					}
				}
			}
		case n >= 251 && n <= 254: // WILL/WONT/DO/DONT + option byte
			if _, err := t.br.ReadByte(); err != nil {
				return 0, err
			}
		}
		// command swallowed; loop for the next real data byte
	}
}

func (t *telnetCodec) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Block for one byte, then opportunistically drain what's buffered so a
	// 1KB ZMODEM subpacket doesn't cost 1024 syscalls.
	b, err := t.ReadByte()
	if err != nil {
		return 0, err
	}
	p[0] = b
	n := 1
	for n < len(p) && t.br.Buffered() > 0 {
		b, err := t.ReadByte()
		if err != nil {
			return n, err
		}
		p[n] = b
		n++
	}
	return n, nil
}

func (t *telnetCodec) Write(p []byte) (int, error) {
	// Double every IAC. Write in runs to keep syscalls down.
	start := 0
	for i, b := range p {
		if b != iac {
			continue
		}
		if i > start {
			if _, err := t.w.Write(p[start:i]); err != nil {
				return start, err
			}
		}
		if _, err := t.w.Write([]byte{iac, iac}); err != nil {
			return i, err
		}
		start = i + 1
	}
	if start < len(p) {
		if _, err := t.w.Write(p[start:]); err != nil {
			return start, err
		}
	}
	return len(p), nil
}

// TransferDeadline is a convenience for callers that want a wall-clock bound
// on a transfer independent of the idle watchdog: it sets a read deadline (if
// the transport supports one) and returns a func to clear it. Over SSH, which
// has no deadline, it is a no-op and the idle watchdog remains the backstop.
func (s *Session) TransferDeadline(d time.Duration) func() {
	if s.dl == nil {
		return func() {}
	}
	s.dl.SetReadDeadline(time.Now().Add(d))
	return func() { s.dl.SetReadDeadline(time.Time{}) }
}
