package term

import (
	"bufio"
	"bytes"
	"io"
	"testing"
)

func bufReader(b []byte) *bufio.Reader { return bufio.NewReader(bytes.NewReader(b)) }

// TestTelnetCodecWriteEscapesIAC verifies outbound 0xFF is doubled and every
// other byte passes through untouched.
func TestTelnetCodecWriteEscapesIAC(t *testing.T) {
	var out bytes.Buffer
	c := &telnetCodec{w: &out}
	in := []byte{0x00, 0xFF, 'a', 0xFF, 0xFF, 0x18, 0xFF}
	n, err := c.Write(in)
	if err != nil || n != len(in) {
		t.Fatalf("Write = %d,%v want %d,nil", n, err, len(in))
	}
	want := []byte{0x00, 0xFF, 0xFF, 'a', 0xFF, 0xFF, 0xFF, 0xFF, 0x18, 0xFF, 0xFF}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("escaped = % x\n    want = % x", out.Bytes(), want)
	}
}

// TestTelnetCodecReadUnescapes checks that doubled IAC collapses to one 0xFF
// and a real telnet command sequence mid-stream is swallowed, not delivered.
func TestTelnetCodecReadUnescapes(t *testing.T) {
	// data: 'h', literal 0xFF (as IAC IAC), IAC WILL ECHO (command, dropped),
	// 'i', IAC SB ... IAC SE (subnegotiation, dropped), 'j'
	wire := []byte{
		'h',
		0xFF, 0xFF, // -> 0xFF
		0xFF, 251, 1, // IAC WILL ECHO -> dropped
		'i',
		0xFF, 250, 1, 2, 3, 0xFF, 240, // IAC SB ... IAC SE -> dropped
		'j',
	}
	c := &telnetCodec{s: &Session{}, br: bufReader(wire), w: io.Discard}
	got, err := io.ReadAll(c)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := []byte{'h', 0xFF, 'i', 'j'}
	if !bytes.Equal(got, want) {
		t.Fatalf("decoded = % x, want % x", got, want)
	}
}

// TestTelnetCodecRoundTrip pushes every byte value through Write then Read and
// expects the original back -- the escaping must be transparent end to end.
func TestTelnetCodecRoundTrip(t *testing.T) {
	orig := make([]byte, 0, 512)
	for i := 0; i < 256; i++ {
		orig = append(orig, byte(i))
	}
	orig = append(orig, orig...) // 0xFF appears adjacent to itself

	var wire bytes.Buffer
	enc := &telnetCodec{w: &wire}
	if _, err := enc.Write(orig); err != nil {
		t.Fatal(err)
	}
	dec := &telnetCodec{s: &Session{}, br: bufReader(wire.Bytes()), w: io.Discard}
	got, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, orig) {
		t.Fatalf("round-trip mismatch: %d bytes back, want %d", len(got), len(orig))
	}
}
