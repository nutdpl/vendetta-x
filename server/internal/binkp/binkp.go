// Package binkp is a minimal BinkP 1.0 mailer client (FTS-1026): the wire
// protocol FTN hubs listen on (TCP 24554) for unattended mail exchange.
// It originates a session, authenticates (plain password, or CRAM-MD5 when
// the answering side offers it -- fsxNet and FidoNet hubs running binkd
// always do), pushes our outbound files, collects whatever the hub has on
// hold for us, and hangs up. No server mode, no compression extensions, no
// resume: the simplest correct caller.
package binkp

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Command bytes (FTS-1026).
const (
	mNUL  = 0
	mADR  = 1
	mPWD  = 2
	mFILE = 3
	mOK   = 4
	mERR  = 5
	mBSY  = 6
	mGET  = 7
	mGOT  = 8
	mSKIP = 9
	mEOB  = 10
)

// File is one file moved during a session.
type File struct {
	Name string
	Data []byte
	Time time.Time
}

// Config describes the uplink and who we are.
type Config struct {
	// HostPort is the hub's address; port 24554 is assumed when absent.
	HostPort string
	// OurAddr is our full FTN address string presented in M_ADR.
	OurAddr string
	// Password is the session password (CRAM-MD5 is used when offered).
	Password string
	// SysName / Sysop dress the M_NUL greeting.
	SysName, Sysop string
	// Timeout bounds the whole session (default 90s).
	Timeout time.Duration

	// MaxInbound caps the total bytes accepted from the hub (default 64 MiB).
	MaxInbound int64
}

// Session dials the hub, authenticates, sends outbound, and returns whatever
// the hub delivered.
func Session(cfg Config, outbound []File) (inbound []File, err error) {
	hostPort := cfg.HostPort
	if !strings.Contains(hostPort, ":") {
		hostPort += ":24554"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	maxIn := cfg.MaxInbound
	if maxIn == 0 {
		maxIn = 64 << 20
	}

	conn, err := net.DialTimeout("tcp", hostPort, 20*time.Second)
	if err != nil {
		return nil, fmt.Errorf("binkp: dial %s: %w", hostPort, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	s := &session{conn: conn, maxIn: maxIn}

	// Our side of the handshake.
	s.cmd(mNUL, "SYS "+cfg.SysName)
	s.cmd(mNUL, "ZYZ "+cfg.Sysop)
	s.cmd(mNUL, "VER Vendetta/X binkp/1.0")
	s.cmd(mADR, cfg.OurAddr)
	if s.err != nil {
		return nil, s.err
	}

	// Their side: read until M_ADR, noting a CRAM-MD5 challenge if offered.
	var challenge []byte
	for {
		cmdByte, payload, isCmd, e := s.read()
		if e != nil {
			return nil, e
		}
		if !isCmd {
			continue // stray data before handshake completes: ignore
		}
		if cmdByte == mNUL {
			if opt, ok := strings.CutPrefix(payload, "OPT "); ok {
				for _, o := range strings.Fields(opt) {
					if h, ok := strings.CutPrefix(o, "CRAM-MD5-"); ok {
						if c, decErr := hex.DecodeString(h); decErr == nil {
							challenge = c
						}
					}
				}
			}
			continue
		}
		if cmdByte == mADR {
			break
		}
		if cmdByte == mERR || cmdByte == mBSY {
			return nil, fmt.Errorf("binkp: hub refused: %s", payload)
		}
	}

	// Authenticate.
	if challenge != nil {
		mac := hmac.New(md5.New, []byte(cfg.Password))
		mac.Write(challenge)
		s.cmd(mPWD, "CRAM-MD5-"+hex.EncodeToString(mac.Sum(nil)))
	} else {
		s.cmd(mPWD, cfg.Password)
	}
	if s.err != nil {
		return nil, s.err
	}
	for {
		cmdByte, payload, isCmd, e := s.read()
		if e != nil {
			return nil, e
		}
		if !isCmd {
			continue
		}
		if cmdByte == mOK {
			break
		}
		if cmdByte == mNUL {
			continue
		}
		if cmdByte == mERR || cmdByte == mBSY {
			return nil, fmt.Errorf("binkp: auth failed: %s", payload)
		}
	}

	// Push our files, then EOB.
	for _, f := range outbound {
		s.cmd(mFILE, fmt.Sprintf("%s %d %d 0", f.Name, len(f.Data), f.Time.Unix()))
		for off := 0; off < len(f.Data); off += 4096 {
			end := off + 4096
			if end > len(f.Data) {
				end = len(f.Data)
			}
			s.data(f.Data[off:end])
		}
		if len(f.Data) == 0 {
			s.data(nil)
		}
	}
	s.cmd(mEOB, "")
	if s.err != nil {
		return nil, s.err
	}

	// Receive their files until their EOB; acknowledge each with M_GOT.
	var cur *File
	var curSize int64
	var total int64
	for {
		cmdByte, payload, isCmd, e := s.read()
		if e != nil {
			return nil, e
		}
		if !isCmd {
			if cur != nil {
				total += int64(len(payload))
				if total > maxIn {
					return nil, fmt.Errorf("binkp: inbound exceeds %d bytes", maxIn)
				}
				cur.Data = append(cur.Data, payload...)
				if int64(len(cur.Data)) >= curSize {
					s.cmd(mGOT, fmt.Sprintf("%s %d %d", cur.Name, curSize, cur.Time.Unix()))
					inbound = append(inbound, *cur)
					cur = nil
				}
			}
			continue
		}
		switch cmdByte {
		case mFILE:
			name, size, mtime, perr := parseFileArgs(payload)
			if perr != nil {
				return nil, perr
			}
			cur = &File{Name: name, Time: mtime}
			curSize = size
			if size == 0 {
				s.cmd(mGOT, fmt.Sprintf("%s 0 %d", name, mtime.Unix()))
				inbound = append(inbound, *cur)
				cur = nil
			}
		case mEOB:
			return inbound, s.err
		case mGOT, mSKIP, mGET, mNUL:
			// acks for our files / chatter: nothing to do
		case mERR:
			return inbound, fmt.Errorf("binkp: hub error: %s", payload)
		}
	}
}

// parseFileArgs reads "filename size unixtime [offset]".
func parseFileArgs(s string) (string, int64, time.Time, error) {
	f := strings.Fields(s)
	if len(f) < 3 {
		return "", 0, time.Time{}, fmt.Errorf("binkp: bad M_FILE %q", s)
	}
	size, err1 := strconv.ParseInt(f[1], 10, 64)
	unix, err2 := strconv.ParseInt(f[2], 10, 64)
	if err1 != nil || err2 != nil || size < 0 {
		return "", 0, time.Time{}, fmt.Errorf("binkp: bad M_FILE %q", s)
	}
	// Strip any path a rude mailer sends.
	name := f[0]
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return name, size, time.Unix(unix, 0), nil
}

// session wraps the framed wire. Writes latch the first error (sticky), so
// call sites can stream and check once.
type session struct {
	conn  net.Conn
	err   error
	maxIn int64
}

// cmd sends a command frame.
func (s *session) cmd(c byte, arg string) {
	if s.err != nil {
		return
	}
	payload := append([]byte{c}, arg...)
	s.frame(payload, true)
}

// data sends a data frame.
func (s *session) data(b []byte) {
	if s.err != nil {
		return
	}
	s.frame(b, false)
}

func (s *session) frame(payload []byte, command bool) {
	if len(payload) > 0x7FFF {
		s.err = fmt.Errorf("binkp: frame too large")
		return
	}
	hdr := uint16(len(payload))
	if command {
		hdr |= 0x8000
	}
	var h [2]byte
	binary.BigEndian.PutUint16(h[:], hdr)
	if _, err := s.conn.Write(h[:]); err != nil {
		s.err = fmt.Errorf("binkp: write: %w", err)
		return
	}
	if _, err := s.conn.Write(payload); err != nil {
		s.err = fmt.Errorf("binkp: write: %w", err)
	}
}

// read returns the next frame: (command byte, payload-after-command, true)
// for command frames, (0, raw payload, false) for data frames.
func (s *session) read() (byte, string, bool, error) {
	var h [2]byte
	if _, err := io.ReadFull(s.conn, h[:]); err != nil {
		return 0, "", false, fmt.Errorf("binkp: read: %w", err)
	}
	hdr := binary.BigEndian.Uint16(h[:])
	n := int(hdr & 0x7FFF)
	buf := make([]byte, n)
	if _, err := io.ReadFull(s.conn, buf); err != nil {
		return 0, "", false, fmt.Errorf("binkp: read: %w", err)
	}
	if hdr&0x8000 != 0 {
		if n == 0 {
			return 0, "", false, fmt.Errorf("binkp: empty command frame")
		}
		return buf[0], string(buf[1:]), true, nil
	}
	return 0, string(buf), false, nil
}
