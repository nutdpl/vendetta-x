// Package zmodem is a from-scratch ZMODEM implementation for the board's
// in-terminal file transfers: Send streams a file to a caller's receiver
// (SyncTERM, NetRunner, lrzsz rz), Receive accepts an upload from a caller's
// sender. Pure Go, no external protocol binaries -- the board stays one
// static binary.
//
// The wire format follows Chuck Forsberg's ZMODEM spec as implemented by
// lrzsz (the de facto reference): hex headers for handshake frames, binary
// headers (CRC-16 or CRC-32, chosen by the receiver's CANFC32 flag) for
// data, ZDLE escaping, and streaming ZCRCG subpackets. The transport must be
// 8-bit clean -- over raw TCP telnet that means the caller wraps the
// connection in an IAC codec first (see term.Transfer).
package zmodem

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// frame types
const (
	zrqinit = 0  // request receive init (sender -> receiver)
	zrinit  = 1  // receive init (receiver -> sender)
	zsinit  = 2  // send init (optional attn string)
	zack    = 3  // ACK to ZCRCQ/ZCRCW or ZSINIT
	zfile   = 4  // file name + info follows
	zskip   = 5  // receiver: skip this file
	znak    = 6  // last header/subpacket was garbled
	zabort  = 7  // receiver requests abort
	zfin    = 8  // session finish
	zrpos   = 9  // receiver: resume from byte position
	zdata   = 10 // data subpackets follow, from position
	zeof    = 11 // end of file, position = file length
	zferr   = 12 // fatal receiver error
)

// control bytes
const (
	zpad  = '*'  // frame lead-in pad
	zdle  = 0x18 // CAN: the escape byte
	zbin  = 'A'  // binary header, CRC-16
	zhex  = 'B'  // hex header, CRC-16
	zbin3 = 'C'  // binary header, CRC-32 ("ZBIN32")
)

// subpacket terminators (sent as ZDLE + byte)
const (
	zcrce = 'h' // end of frame; header follows
	zcrcg = 'i' // more data follows (streaming)
	zcrcq = 'j' // more data follows, send ZACK
	zcrcw = 'k' // end of frame, send ZACK (sender waits)
)

// ZDLE-escape specials
const (
	zrub0 = 'l' // escaped 0x7F
	zrub1 = 'm' // escaped 0xFF
)

// ZRINIT ZF0 capability flags
const (
	canfdx  = 0x01 // full duplex
	canovio = 0x02 // can overlap disk I/O
	canfc32 = 0x20 // can use 32-bit CRC
	escctl  = 0x40 // wants all control chars escaped
)

const subpacketSize = 1024

// ErrCancelled is returned when the far end aborts the session (CAN burst).
var ErrCancelled = errors.New("zmodem: session cancelled by remote")

// header is one ZMODEM frame header: a type and four position/flag bytes.
// Position frames store a little-endian offset in b[0..3]; flag frames use
// b[3] as the primary flags byte (lrzsz's ZF0).
type header struct {
	typ byte
	b   [4]byte
}

func posHeader(typ byte, pos int64) header {
	return header{typ: typ, b: [4]byte{byte(pos), byte(pos >> 8), byte(pos >> 16), byte(pos >> 24)}}
}

func (h header) pos() int64 {
	return int64(h.b[0]) | int64(h.b[1])<<8 | int64(h.b[2])<<16 | int64(h.b[3])<<24
}

// ---- CRC ------------------------------------------------------------------

// crc16 is plain CRC-16/XMODEM (poly 0x1021, init 0, MSB first). lrzsz's
// source LOOKS like it augments the message with two zero bytes, but that's
// an artifact of its table macro being the "augmented" formulation -- the
// zero-feeds there convert it to exactly this standard value. Check value:
// crc16("123456789") == 0x31C3.
func crc16Update(crc uint16, b byte) uint16 {
	crc ^= uint16(b) << 8
	for i := 0; i < 8; i++ {
		if crc&0x8000 != 0 {
			crc = crc<<1 ^ 0x1021
		} else {
			crc <<= 1
		}
	}
	return crc
}

func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc = crc16Update(crc, b)
	}
	return crc
}

// ---- low-level escaped I/O --------------------------------------------------

// conn wraps the transport with a one-byte reader and the session's escaping
// state (whether the receiver asked for full control-character escaping).
type conn struct {
	r         io.Reader
	w         io.Writer
	escapeAll bool
	rbuf      [1]byte
	lastSent  byte
	// lastFormatWas32 remembers the CRC width of the most recent binary
	// header read: the data subpackets that follow a ZFILE/ZDATA frame use
	// the same width as their header.
	lastFormatWas32 bool
}

func newConn(rw io.ReadWriter) *conn { return &conn{r: rw, w: rw} }

func (c *conn) readByte() (byte, error) {
	if br, ok := c.r.(io.ByteReader); ok {
		return br.ReadByte()
	}
	_, err := io.ReadFull(c.r, c.rbuf[:])
	return c.rbuf[0], err
}

// needsEscape reports whether b must be ZDLE-escaped on the wire.
func (c *conn) needsEscape(b byte) bool {
	switch b {
	case zdle, 0x10, 0x90, 0x11, 0x91, 0x13, 0x93:
		return true
	case 0x0D, 0x8D:
		// escape CR after '@' (guards against Telenet-era escape sequences;
		// lrzsz always applies this rule)
		return c.lastSent == '@' || c.lastSent == 0xC0
	}
	if c.escapeAll && b&0x60 == 0 {
		return true
	}
	return false
}

// sendEscaped writes one byte, ZDLE-escaping as needed.
func (c *conn) sendEscaped(buf *bytes.Buffer, b byte) {
	if c.needsEscape(b) {
		buf.WriteByte(zdle)
		buf.WriteByte(b ^ 0x40)
	} else {
		buf.WriteByte(b)
	}
	c.lastSent = b
}

// readEscaped returns the next data byte, decoding ZDLE escapes. A subpacket
// terminator (ZCRCE/G/Q/W) is returned as (term, true, nil).
func (c *conn) readEscaped() (b byte, isTerm bool, err error) {
	v, err := c.readByte()
	if err != nil {
		return 0, false, err
	}
	if v != zdle {
		return v, false, nil
	}
	// after ZDLE
	v, err = c.readByte()
	if err != nil {
		return 0, false, err
	}
	switch v {
	case zcrce, zcrcg, zcrcq, zcrcw:
		return v, true, nil
	case zrub0:
		return 0x7F, false, nil
	case zrub1:
		return 0xFF, false, nil
	case zdle:
		// CAN CAN ... an abort burst
		n := 2
		for n < 5 {
			v, err = c.readByte()
			if err != nil || v != zdle {
				break
			}
			n++
		}
		if n >= 5 {
			return 0, false, ErrCancelled
		}
		return 0, false, fmt.Errorf("zmodem: unexpected CAN sequence")
	}
	if v&0x60 == 0x40 {
		return v ^ 0x40, false, nil
	}
	return 0, false, fmt.Errorf("zmodem: bad ZDLE escape 0x%02x", v)
}

// ---- header I/O -------------------------------------------------------------

var hexDigits = "0123456789abcdef"

// writeHexHeader emits a hex-format header (always CRC-16).
func (c *conn) writeHexHeader(h header) error {
	var buf bytes.Buffer
	buf.WriteByte(zpad)
	buf.WriteByte(zpad)
	buf.WriteByte(zdle)
	buf.WriteByte(zhex)
	payload := []byte{h.typ, h.b[0], h.b[1], h.b[2], h.b[3]}
	crc := crc16(payload)
	payload = append(payload, byte(crc>>8), byte(crc))
	for _, b := range payload {
		buf.WriteByte(hexDigits[b>>4])
		buf.WriteByte(hexDigits[b&0x0F])
	}
	buf.WriteByte('\r')
	buf.WriteByte('\n')
	if h.typ != zack && h.typ != zfin {
		buf.WriteByte(0x11) // XON: releases any XOFF-choked sender
	}
	_, err := c.w.Write(buf.Bytes())
	return err
}

// writeBinHeader emits a binary header; crc32 selects ZBIN32 vs ZBIN.
func (c *conn) writeBinHeader(h header, use32 bool) error {
	var buf bytes.Buffer
	buf.WriteByte(zpad)
	buf.WriteByte(zdle)
	payload := []byte{h.typ, h.b[0], h.b[1], h.b[2], h.b[3]}
	if use32 {
		buf.WriteByte(zbin3)
		c.lastSent = zbin3
		for _, b := range payload {
			c.sendEscaped(&buf, b)
		}
		crc := crc32.ChecksumIEEE(payload)
		for i := 0; i < 4; i++ {
			c.sendEscaped(&buf, byte(crc>>(8*i)))
		}
	} else {
		buf.WriteByte(zbin)
		c.lastSent = zbin
		for _, b := range payload {
			c.sendEscaped(&buf, b)
		}
		crc := crc16(payload)
		c.sendEscaped(&buf, byte(crc>>8))
		c.sendEscaped(&buf, byte(crc))
	}
	_, err := c.w.Write(buf.Bytes())
	return err
}

// readHeader scans for and decodes the next frame header of any format,
// tolerating line noise between frames. maxJunk bounds the scan.
func (c *conn) readHeader() (header, error) {
	const maxJunk = 4096
	junk := 0
	cans := 0
	for {
		b, err := c.readByte()
		if err != nil {
			return header{}, err
		}
		if b == zdle {
			cans++
			if cans >= 5 {
				return header{}, ErrCancelled
			}
		} else {
			cans = 0
		}
		if b != zpad {
			junk++
			if junk > maxJunk {
				return header{}, fmt.Errorf("zmodem: no frame found (junk overflow)")
			}
			continue
		}
		// one or more ZPADs, then ZDLE, then a format byte
		for b == zpad {
			b, err = c.readByte()
			if err != nil {
				return header{}, err
			}
		}
		if b != zdle {
			junk++
			continue
		}
		b, err = c.readByte()
		if err != nil {
			return header{}, err
		}
		switch b {
		case zhex:
			return c.readHexBody()
		case zbin:
			c.lastFormatWas32 = false
			return c.readBinBody(false)
		case zbin3:
			c.lastFormatWas32 = true
			return c.readBinBody(true)
		default:
			junk++
			continue
		}
	}
}

func (c *conn) readHexBody() (header, error) {
	raw := make([]byte, 7) // type + 4 + crc16
	for i := range raw {
		hi, err := c.readHexNibble()
		if err != nil {
			return header{}, err
		}
		lo, err := c.readHexNibble()
		if err != nil {
			return header{}, err
		}
		raw[i] = hi<<4 | lo
	}
	if crc16(raw[:5]) != uint16(raw[5])<<8|uint16(raw[6]) {
		return header{}, fmt.Errorf("zmodem: hex header CRC mismatch")
	}
	return header{typ: raw[0], b: [4]byte{raw[1], raw[2], raw[3], raw[4]}}, nil
}

func (c *conn) readHexNibble() (byte, error) {
	b, err := c.readByte()
	if err != nil {
		return 0, err
	}
	switch {
	case b >= '0' && b <= '9':
		return b - '0', nil
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, nil
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, nil
	}
	return 0, fmt.Errorf("zmodem: bad hex digit 0x%02x", b)
}

func (c *conn) readBinBody(use32 bool) (header, error) {
	n := 7 // type + 4 + crc16
	if use32 {
		n = 9
	}
	raw := make([]byte, 0, n)
	for len(raw) < n {
		b, isTerm, err := c.readEscaped()
		if err != nil {
			return header{}, err
		}
		if isTerm {
			return header{}, fmt.Errorf("zmodem: unexpected terminator in header")
		}
		raw = append(raw, b)
	}
	if use32 {
		want := uint32(raw[5]) | uint32(raw[6])<<8 | uint32(raw[7])<<16 | uint32(raw[8])<<24
		if crc32.ChecksumIEEE(raw[:5]) != want {
			return header{}, fmt.Errorf("zmodem: bin32 header CRC mismatch")
		}
	} else {
		if crc16(raw[:5]) != uint16(raw[5])<<8|uint16(raw[6]) {
			return header{}, fmt.Errorf("zmodem: bin header CRC mismatch")
		}
	}
	return header{typ: raw[0], b: [4]byte{raw[1], raw[2], raw[3], raw[4]}}, nil
}

// ---- data subpackets --------------------------------------------------------

// writeSubpacket emits one ZDLE-escaped data subpacket ending in term.
func (c *conn) writeSubpacket(data []byte, term byte, use32 bool) error {
	var buf bytes.Buffer
	for _, b := range data {
		c.sendEscaped(&buf, b)
	}
	buf.WriteByte(zdle)
	buf.WriteByte(term)
	c.lastSent = term
	if use32 {
		crc := crc32.NewIEEE()
		crc.Write(data)
		crc.Write([]byte{term})
		sum := crc.Sum32()
		for i := 0; i < 4; i++ {
			c.sendEscaped(&buf, byte(sum>>(8*i)))
		}
	} else {
		var crc uint16
		for _, b := range data {
			crc = crc16Update(crc, b)
		}
		crc = crc16Update(crc, term)
		c.sendEscaped(&buf, byte(crc>>8))
		c.sendEscaped(&buf, byte(crc))
	}
	_, err := c.w.Write(buf.Bytes())
	return err
}

// readSubpacket collects one data subpacket, returning its payload and
// terminator. max bounds the payload size.
func (c *conn) readSubpacket(use32 bool, max int) ([]byte, byte, error) {
	var data []byte
	for {
		b, isTerm, err := c.readEscaped()
		if err != nil {
			return nil, 0, err
		}
		if !isTerm {
			if len(data) >= max {
				return nil, 0, fmt.Errorf("zmodem: subpacket exceeds %d bytes", max)
			}
			data = append(data, b)
			continue
		}
		term := b
		// CRC bytes follow, ZDLE-escaped like data
		n := 2
		if use32 {
			n = 4
		}
		crcRaw := make([]byte, 0, 4)
		for len(crcRaw) < n {
			cb, ct, err := c.readEscaped()
			if err != nil {
				return nil, 0, err
			}
			if ct {
				return nil, 0, fmt.Errorf("zmodem: terminator inside CRC")
			}
			crcRaw = append(crcRaw, cb)
		}
		if use32 {
			crc := crc32.NewIEEE()
			crc.Write(data)
			crc.Write([]byte{term})
			want := uint32(crcRaw[0]) | uint32(crcRaw[1])<<8 | uint32(crcRaw[2])<<16 | uint32(crcRaw[3])<<24
			if crc.Sum32() != want {
				return nil, 0, fmt.Errorf("zmodem: data CRC32 mismatch")
			}
		} else {
			var crc uint16
			for _, db := range data {
				crc = crc16Update(crc, db)
			}
			crc = crc16Update(crc, term)
			if crc != uint16(crcRaw[0])<<8|uint16(crcRaw[1]) {
				return nil, 0, fmt.Errorf("zmodem: data CRC16 mismatch")
			}
		}
		return data, term, nil
	}
}
