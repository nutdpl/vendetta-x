package web

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// A minimal RFC 6455 WebSocket server, stdlib-only -- just enough to carry the
// board's byte stream to a browser terminal. It exposes the accepted socket as
// an io.ReadWriteCloser so the existing board entry can drive it exactly like a
// telnet socket or an ssh channel. No extensions, no permessage-deflate; text
// and binary frames both deliver bytes, control frames (ping/close) are
// handled, and server->client frames are unmasked per spec.

// wsGUID is the RFC 6455 magic value appended to the client key.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsConn wraps a hijacked TCP connection as a framed io.ReadWriteCloser.
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader

	mu     sync.Mutex // serializes frame writes (board write + pong/close)
	closed bool

	// leftover data-frame payload not yet consumed by Read.
	buf []byte
}

// acceptWebSocket completes the handshake on an HTTP request and returns the
// upgraded connection. It writes the 101 response itself; on error nothing has
// been written that the caller must clean up beyond returning.
func acceptWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!headerContainsToken(r.Header.Get("Connection"), "upgrade") {
		return nil, errors.New("not a websocket upgrade")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("connection does not support hijacking")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack: %w", err)
	}

	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := brw.WriteString(resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write handshake: %w", err)
	}
	if err := brw.Flush(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("flush handshake: %w", err)
	}
	return &wsConn{conn: conn, br: brw.Reader}, nil
}

// headerContainsToken reports whether a comma-separated header value carries
// token (case-insensitive) -- Connection: keep-alive, Upgrade.
func headerContainsToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// opcodes
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// Read returns bytes from incoming data frames, transparently answering pings
// and surfacing a client close as io.EOF. It blocks until data is available.
func (c *wsConn) Read(p []byte) (int, error) {
	for len(c.buf) == 0 {
		op, payload, err := c.readFrame()
		if err != nil {
			return 0, err
		}
		switch op {
		case opText, opBinary, opContinuation:
			c.buf = payload
		case opPing:
			if err := c.writeFrame(opPong, payload); err != nil {
				return 0, err
			}
		case opPong:
			// ignore
		case opClose:
			c.sendClose()
			return 0, io.EOF
		}
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

// readFrame reads one whole frame, unmasking the payload. Fragmented data
// frames are reassembled into a single payload before returning.
func (c *wsConn) readFrame() (byte, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
		return 0, nil, err
	}
	fin := hdr[0]&0x80 != 0
	op := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	length := int(hdr[1] & 0x7f)

	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, nil, err
		}
		n := binary.BigEndian.Uint64(ext[:])
		if n > 1<<20 { // a 1 MiB frame from a keystroke stream is hostile
			return 0, nil, errors.New("ws: frame too large")
		}
		length = int(n)
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i&3]
		}
	}

	// Reassemble a fragmented data frame (rare for keystrokes, but honor it).
	if !fin && (op == opText || op == opBinary || op == opContinuation) {
		nextOp, rest, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}
		_ = nextOp
		payload = append(payload, rest...)
	}
	return op, payload, nil
}

// Write sends p as a single unmasked binary frame.
func (c *wsConn) Write(p []byte) (int, error) {
	if err := c.writeFrame(opBinary, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// writeFrame writes one unmasked frame with the given opcode and payload.
func (c *wsConn) writeFrame(op byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return io.ErrClosedPipe
	}
	var hdr []byte
	b0 := byte(0x80 | op) // FIN + opcode
	n := len(payload)
	switch {
	case n < 126:
		hdr = []byte{b0, byte(n)}
	case n < 1<<16:
		hdr = []byte{b0, 126, byte(n >> 8), byte(n)}
	default:
		hdr = make([]byte, 10)
		hdr[0] = b0
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(n))
	}
	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// sendClose sends a close frame (best effort).
func (c *wsConn) sendClose() {
	_ = c.writeFrame(opClose, []byte{0x03, 0xe8}) // 1000 normal
}

// Close sends a close frame and shuts the underlying connection.
func (c *wsConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	// best-effort close frame, then tear down the socket.
	_ = c.writeFrameUnlocked(opClose, []byte{0x03, 0xe8})
	return c.conn.Close()
}

// writeFrameUnlocked writes a frame without taking the mutex (Close already
// marked closed; we still want the close frame out before the socket dies).
func (c *wsConn) writeFrameUnlocked(op byte, payload []byte) error {
	hdr := []byte{byte(0x80 | op), byte(len(payload))}
	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := c.conn.Write(payload)
		return err
	}
	return nil
}
