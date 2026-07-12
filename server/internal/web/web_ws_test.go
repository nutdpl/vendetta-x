package web

import (
	"bufio"
	"encoding/base64"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vendetta-x/server/internal/store"
)

// clientTextFrame builds a masked client->server text frame (payload < 126).
func clientTextFrame(payload string) []byte {
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	out := []byte{0x81, byte(0x80 | len(payload))}
	out = append(out, mask[:]...)
	for i := 0; i < len(payload); i++ {
		out = append(out, payload[i]^mask[i&3])
	}
	return out
}

// readServerFrame reads one unmasked server frame's opcode and payload.
func readServerFrame(t *testing.T, r *bufio.Reader) (byte, []byte) {
	t.Helper()
	h, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read frame b0: %v", err)
	}
	l, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read frame b1: %v", err)
	}
	if l&0x80 != 0 {
		t.Fatal("server frame must not be masked")
	}
	n := int(l & 0x7f)
	if n == 126 {
		var ext [2]byte
		io.ReadFull(r, ext[:])
		n = int(ext[0])<<8 | int(ext[1])
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatalf("read frame payload: %v", err)
	}
	return h & 0x0f, payload
}

func TestWSCodecRoundTrip(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	ws := &wsConn{conn: server, br: bufio.NewReader(server)}

	// Client sends a masked text frame; wsConn.Read must return it unmasked.
	go func() { client.Write(clientTextFrame("hello")) }()
	buf := make([]byte, 32)
	n, err := ws.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := string(buf[:n]); got != "hello" {
		t.Fatalf("Read = %q, want hello", got)
	}

	// wsConn.Write must produce a well-formed unmasked binary frame.
	cr := bufio.NewReader(client)
	go func() { ws.Write([]byte("world")) }()
	op, payload := readServerFrame(t, cr)
	if op != opBinary {
		t.Fatalf("opcode = %d, want binary(%d)", op, opBinary)
	}
	if string(payload) != "world" {
		t.Fatalf("payload = %q, want world", payload)
	}
}

func TestWSPingAnsweredWithPong(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	ws := &wsConn{conn: server, br: bufio.NewReader(server)}

	// A masked ping (opcode 0x9), empty payload.
	ping := []byte{0x89, 0x80, 0x11, 0x22, 0x33, 0x44}
	cr := bufio.NewReader(client)
	go func() { client.Write(ping) }()
	// Read blocks until a data frame; the ping is answered internally. Read the
	// pong off the client side to confirm.
	done := make(chan struct{})
	go func() { buf := make([]byte, 8); ws.Read(buf); close(done) }()
	op, _ := readServerFrame(t, cr)
	if op != opPong {
		t.Fatalf("expected a pong (%d) in reply to ping, got op %d", opPong, op)
	}
	server.Close()
	<-done
}

// TestWSUpgradeAndStream drives the whole path over a real listener: a manual
// WebSocket handshake, then a board runner that echoes input back uppercased.
func TestWSUpgradeAndStream(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	st.Seed()

	// A stand-in board runner: greet, then echo each byte uppercased.
	runner := func(conn io.ReadWriteCloser, remote string) {
		defer conn.Close()
		conn.Write([]byte("READY"))
		buf := make([]byte, 64)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				conn.Write([]byte(strings.ToUpper(string(buf[:n]))))
			}
			if err != nil {
				return
			}
		}
	}
	h := New(st, func() []string { return nil }, Config{RunTerminal: runner})
	srv := httptest.NewServer(h)
	defer srv.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	req := "GET /ws-term HTTP/1.1\r\n" +
		"Host: " + strings.TrimPrefix(srv.URL, "http://") + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	r := bufio.NewReader(conn)
	statusLine, _ := r.ReadString('\n')
	if !strings.Contains(statusLine, "101") {
		t.Fatalf("handshake status = %q, want 101", strings.TrimSpace(statusLine))
	}
	// Drain the rest of the response headers.
	for {
		line, _ := r.ReadString('\n')
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// First server frame: the greeting.
	op, payload := readServerFrame(t, r)
	if op != opBinary || string(payload) != "READY" {
		t.Fatalf("greeting frame: op=%d payload=%q, want binary READY", op, payload)
	}

	// Send a masked frame; expect it echoed back uppercased.
	if _, err := conn.Write(clientTextFrame("go")); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	_, echo := readServerFrame(t, r)
	if string(echo) != "GO" {
		t.Fatalf("echo = %q, want GO", echo)
	}
}
