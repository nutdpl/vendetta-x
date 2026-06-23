package term

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vendetta-x/server/internal/render"
)

// pair builds a Session over an in-memory net.Pipe and returns the session plus
// the client end of the wire. net.Pipe is unbuffered and supports read
// deadlines, so it exercises the same paths a real telnet socket does.
func pair(t *testing.T) (*Session, net.Conn) {
	t.Helper()
	srv, cli := net.Pipe()
	t.Cleanup(func() { srv.Close(); cli.Close() })
	return New(srv), cli
}

// drainAll reads everything the client sees until the pipe closes, sending the
// bytes back over the returned channel. Needed because net.Pipe writes block
// until read.
func drainAll(cli net.Conn) <-chan string {
	out := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := cli.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		out <- sb.String()
	}()
	return out
}

func TestRevealStreamsEverythingAndReturnsMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "screen.pp")
	// Three plain lines plus a lightbar marker. |CL is the clear at the top.
	art := "|CLline one\nline two\n|{5,3,A,Apple}\n"
	if err := os.WriteFile(path, []byte(art), 0o644); err != nil {
		t.Fatal(err)
	}

	s, cli := pair(t)
	out := drainAll(cli)

	var markers []render.Marker
	done := make(chan struct{})
	go func() {
		// lineDelay 0 == instant draw, so this never blocks on a read deadline.
		markers, _ = s.Reveal(path, nil, 0)
		s.Close()
		close(done)
	}()
	<-done
	got := <-out

	for _, want := range []string{"line one", "line two", "Apple"} {
		if !strings.Contains(got, want) {
			t.Errorf("reveal output missing %q; got:\n%q", want, got)
		}
	}
	if len(markers) != 1 || markers[0].Key != 'A' || markers[0].Row != 5 || markers[0].Col != 3 {
		t.Fatalf("expected one marker A at 5,3; got %+v", markers)
	}
}

func TestWaitKeyTimesOutWithoutFalseHangup(t *testing.T) {
	s, _ := pair(t)
	start := time.Now()
	if s.WaitKey(40 * time.Millisecond) {
		t.Fatal("WaitKey reported a key with no input")
	}
	if d := time.Since(start); d < 30*time.Millisecond {
		t.Fatalf("WaitKey returned too early (%v); deadline not honored", d)
	}
	// A timeout must NOT be remembered as a key for the next read.
	if s.pending != nil {
		t.Fatal("timeout left a pending key")
	}
}

func TestWaitKeyPushesKeyBackForNextRead(t *testing.T) {
	s, cli := pair(t)
	go func() { cli.Write([]byte("x")) }()

	if !s.WaitKey(2 * time.Second) {
		t.Fatal("WaitKey did not see the keypress")
	}
	// The skip key must survive to the menu that follows the animation.
	k, ch := s.ReadKey()
	if k != KeyChar || ch != 'x' {
		t.Fatalf("pushed-back key = (%v,%q); want (KeyChar,'x')", k, ch)
	}
}
