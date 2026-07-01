// Package term is the telnet terminal session layer: it renders the board's
// real .pp art through the render package, runs lightbar menus over the |{...}
// markers, and reads keys (decoding telnet IAC and arrow escapes). This is what
// turns a raw socket into the elite ANSI board.
package term

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"vendetta-x/server/internal/render"
)

type Kind int

const (
	KeyChar Kind = iota // ch holds the byte
	KeyEnter
	KeyEsc
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEOF
)

const (
	lbSel  = "\x1b[1;37;41m" // selected: bright white on red
	lbNorm = "\x1b[0;37;40m" // normal: grey on black
)

// deadliner is the optional read-deadline capability some transports provide
// (net.Conn does; an SSH channel does not).
type deadliner interface {
	SetReadDeadline(t time.Time) error
}

// errNoDeadline is returned by SetReadDeadline when the transport can't do it.
var errNoDeadline = errors.New("term: transport has no read deadline")

// Session wraps a connection (telnet socket or SSH channel) with buffered I/O.
// It is transport-agnostic: the telnet codec works the same over either.
type Session struct {
	rwc    io.ReadWriteCloser
	dl     deadliner // nil if the transport has no read deadline
	remote string
	br     *bufio.Reader
	bw     *bufio.Writer

	// idle-session reaping (see idle.go): lastRead is the most recent input
	// time (UnixNano, atomic); the watchdog closes rwc when it goes stale.
	lastRead  int64
	idleOnce  sync.Once
	done      chan struct{}
	closeOnce sync.Once

	// lastReadErr holds the error that caused the most recent KeyEOF, so a
	// timed read (WaitKey) can tell a benign read deadline from a real EOF.
	lastReadErr error

	// pending is a one-key pushback: a key consumed while skipping an animation
	// (see anim.go) is stashed here so the next ReadKey delivers it. That makes a
	// hotkey hit mid-intro both skip the paint AND land on the menu.
	pending *keyEvent

	// readMu serializes input decoding. The teleconference runs a second
	// goroutine that reads keys (so it can wait on a chat message OR a keypress);
	// on transports without a read deadline (SSH) that reader can still be blocked
	// in ReadKey when the caller leaves chat and the main loop resumes reading.
	// readMu guarantees the two never touch the buffered reader / pending state
	// concurrently, so leaving chat costs at most one key, never a data race.
	readMu sync.Mutex

	// telnet is true for the telnet transport, false for SSH. It only matters
	// for binary file transfer (Transfer): a telnet byte stream must escape
	// 0xFF as IAC IAC, while an SSH channel is already 8-bit clean.
	telnet bool
}

type keyEvent struct {
	k  Kind
	ch byte
}

// New builds a session over a telnet socket.
func New(conn net.Conn) *Session {
	return &Session{
		rwc:    conn,
		dl:     conn,
		remote: conn.RemoteAddr().String(),
		br:     bufio.NewReaderSize(conn, 1024),
		bw:     bufio.NewWriterSize(conn, 4096),
		done:   make(chan struct{}),
		telnet: true,
	}
}

// NewRW builds a session over any read/write/closer (e.g. an SSH channel),
// with remote as the caller's address for logging/presence. If the transport
// supports SetReadDeadline it is used; otherwise deadline-based teardown is
// skipped gracefully.
func NewRW(rwc io.ReadWriteCloser, remote string) *Session {
	s := &Session{
		rwc:    rwc,
		remote: remote,
		br:     bufio.NewReaderSize(rwc, 1024),
		bw:     bufio.NewWriterSize(rwc, 4096),
		done:   make(chan struct{}),
	}
	if d, ok := rwc.(deadliner); ok {
		s.dl = d
	}
	return s
}

// RemoteAddr is the caller's address string.
func (s *Session) RemoteAddr() string { return s.remote }

func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		if s.done != nil {
			close(s.done) // stop the idle watchdog
		}
	})
	s.Flush()
	return s.rwc.Close()
}
func (s *Session) Flush()                            { s.bw.Flush() }
func (s *Session) Print(str string)                  { s.bw.WriteString(str) }
func (s *Session) Printf(f string, a ...interface{}) { fmt.Fprintf(s.bw, f, a...) }
func (s *Session) Write(b []byte)                    { s.bw.Write(b) }

// Negotiate puts the client into character-at-a-time mode: server WILL echo +
// suppress-go-ahead. Good enough for SyncTERM/xterm/PuTTY.
func (s *Session) Negotiate() {
	s.Write([]byte{255, 251, 1, 255, 251, 3}) // IAC WILL ECHO, IAC WILL SGA
	s.Flush()
}

// ReadKey returns one decoded key, stripping telnet IAC and decoding arrows.
func (s *Session) ReadKey() (Kind, byte) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.markActivity()
	if s.pending != nil {
		ev := *s.pending
		s.pending = nil
		return ev.k, ev.ch
	}
	s.lastReadErr = nil
	for {
		b, err := s.br.ReadByte()
		if err != nil {
			s.lastReadErr = err
			return KeyEOF, 0
		}
		switch b {
		case 255: // IAC -- consume the telnet command
			cmd, err := s.br.ReadByte()
			if err != nil {
				return KeyEOF, 0
			}
			switch {
			case cmd == 250: // SB ... IAC SE
				for {
					x, err := s.br.ReadByte()
					if err != nil {
						return KeyEOF, 0
					}
					if x == 255 {
						if y, _ := s.br.ReadByte(); y == 240 {
							break
						}
					}
				}
			case cmd >= 251 && cmd <= 254: // WILL/WONT/DO/DONT + option byte
				s.br.ReadByte()
			}
			continue
		case '\r':
			// Consume a trailing LF/NUL only if it is ALREADY buffered. A real
			// terminal sends CR+LF (or CR+NUL) as one burst, so the second byte
			// is buffered alongside the CR; a bare CR has nothing buffered. We
			// must not Peek-block waiting for a byte that may never come.
			if s.br.Buffered() > 0 {
				if nb, _ := s.br.Peek(1); len(nb) == 1 && (nb[0] == '\n' || nb[0] == 0) {
					s.br.ReadByte()
				}
			}
			return KeyEnter, '\r'
		case '\n':
			return KeyEnter, '\n'
		case 27: // ESC -- maybe an arrow (CSI/SS3). Only when the rest of the
			// sequence is already buffered; a lone ESC keypress is KeyEsc and
			// must not block waiting for more bytes.
			if s.br.Buffered() > 0 {
				if nb, _ := s.br.Peek(1); len(nb) == 1 && (nb[0] == '[' || nb[0] == 'O') {
					s.br.ReadByte()
					f, err := s.br.ReadByte()
					if err != nil {
						return KeyEOF, 0
					}
					switch f {
					case 'A':
						return KeyUp, 0
					case 'B':
						return KeyDown, 0
					case 'C':
						return KeyRight, 0
					case 'D':
						return KeyLeft, 0
					}
					continue
				}
			}
			return KeyEsc, 27
		default:
			return KeyChar, b
		}
	}
}

// ReadLine reads an echoed line (printable + backspace), up to max chars.
func (s *Session) ReadLine(max int) string {
	var buf []byte
	for {
		k, ch := s.ReadKey()
		switch k {
		case KeyEOF:
			return string(buf)
		case KeyEnter:
			s.Print("\r\n")
			s.Flush()
			return string(buf)
		case KeyChar:
			if ch == 8 || ch == 127 { // backspace
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
					s.Print("\b \b")
					s.Flush()
				}
				continue
			}
			if ch >= 32 && len(buf) < max {
				buf = append(buf, ch)
				s.bw.WriteByte(ch)
				s.Flush()
			}
		}
	}
}

// ReadPassword reads a line like ReadLine but echoes '*' instead of the typed
// characters, up to max chars.
func (s *Session) ReadPassword(max int) string {
	var buf []byte
	for {
		k, ch := s.ReadKey()
		switch k {
		case KeyEOF:
			return string(buf)
		case KeyEnter:
			s.Print("\r\n")
			s.Flush()
			return string(buf)
		case KeyChar:
			if ch == 8 || ch == 127 { // backspace
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
					s.Print("\b \b")
					s.Flush()
				}
				continue
			}
			if ch >= 32 && len(buf) < max {
				buf = append(buf, ch)
				s.Print("*")
				s.Flush()
			}
		}
	}
}

// SetReadDeadline sets the underlying connection's read deadline. A zero time
// clears it. Used to interrupt a goroutine blocked in ReadKey (e.g. tearing
// down the teleconference input reader).
func (s *Session) SetReadDeadline(t time.Time) error {
	if s.dl == nil {
		return errNoDeadline
	}
	return s.dl.SetReadDeadline(t)
}

// RenderScreen renders a .pp/.ans file to the session, returning any |{...}
// lightbar markers found (in art order).
func (s *Session) RenderScreen(path string, tokens map[string]string) []render.Marker {
	var markers []render.Marker
	ctx := &render.Ctx{Tokens: tokens, OnMarker: func(m render.Marker) { markers = append(markers, m) }}
	render.RenderFile(s.bw, path, ctx)
	s.Flush()
	return markers
}

// Lightbar runs the moving bar over opts (already drawn positions). Returns the
// chosen option's hotkey, or ok=false on carrier loss.
func (s *Session) Lightbar(opts []render.Marker, start int) (byte, bool) {
	if len(opts) == 0 {
		return 0, false
	}
	sel := start
	if sel < 0 || sel >= len(opts) {
		sel = 0
	}
	s.Print("\x1b[?25l") // hide cursor
	for i := range opts {
		s.drawOpt(opts[i], i == sel)
	}
	s.Flush()
	for {
		k, ch := s.ReadKey()
		switch k {
		case KeyEOF:
			s.Print("\x1b[?25h")
			s.Flush()
			return 0, false
		case KeyEnter:
			s.Print("\x1b[?25h")
			s.Flush()
			return opts[sel].Key, true
		case KeyUp, KeyDown, KeyLeft, KeyRight:
			if ni := neighbor(opts, sel, k); ni >= 0 && ni != sel {
				s.drawOpt(opts[sel], false)
				sel = ni
				s.drawOpt(opts[sel], true)
				s.Flush()
			}
		case KeyChar:
			lc := lower(ch)
			for i := range opts {
				if lower(opts[i].Key) == lc {
					if i != sel {
						s.drawOpt(opts[sel], false)
						s.drawOpt(opts[i], true)
					}
					s.Print("\x1b[?25h")
					s.Flush()
					return opts[i].Key, true
				}
			}
		}
	}
}

// Pause prints a prompt and waits for any key (the classic "[ press any key ]").
func (s *Session) Pause() {
	s.Print("\r\n\x1b[1;30m  [ press any key to continue ]\x1b[0m")
	s.Flush()
	s.ReadKey()
}

// Notice shows a one-line message and pauses. Used for "not wired yet" and
// soft error paths.
func (s *Session) Notice(msg string) {
	s.Print("\r\n\x1b[1;33m  " + msg + "\x1b[0m\r\n")
	s.Pause()
}

func (s *Session) drawOpt(m render.Marker, sel bool) {
	style := lbNorm
	if sel {
		style = lbSel
	}
	fmt.Fprintf(s.bw, "\x1b[%d;%dH%s%s\x1b[0m", m.Row, m.Col, style, m.Label)
}

// neighbor finds the option nearest opts[cur] in direction dir (spatial grid
// nav: ports lb_neighbor from core/lbmenu.c). -1 if none that way.
func neighbor(o []render.Marker, cur int, dir Kind) int {
	best, bp, bs := -1, 0, 0
	cr, cc := o[cur].Row, o[cur].Col
	for i := range o {
		if i == cur {
			continue
		}
		rd, cd := o[i].Row-cr, o[i].Col-cc
		var ok bool
		switch dir {
		case KeyUp:
			ok = rd < 0
		case KeyDown:
			ok = rd > 0
		case KeyLeft:
			ok = cd < 0
		case KeyRight:
			ok = cd > 0
		}
		if !ok {
			continue
		}
		var prim, sec int
		if dir == KeyUp || dir == KeyDown {
			prim, sec = abs(rd), abs(cd)
		} else {
			prim, sec = abs(cd), abs(rd)
		}
		if best < 0 || prim < bp || (prim == bp && sec < bs) {
			best, bp, bs = i, prim, sec
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}
