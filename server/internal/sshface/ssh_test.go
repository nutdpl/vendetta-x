package sshface

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const banner = "VENDETTA/X READY\n"

// startServer spins up serve() on a random localhost port and returns the
// address plus a slice (guarded) of remote addrs that onSession saw. The
// onSession echoes a fixed banner then echoes back one line of input, proving
// bytes flow both directions.
func startServer(t *testing.T, hostKeyPath string) (addr string, seen *seenAddrs) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	seen = &seenAddrs{}
	onSession := func(ch io.ReadWriteCloser, remoteAddr, _ string) {
		seen.add(remoteAddr)
		// Write a banner immediately.
		_, _ = io.WriteString(ch, banner)
		// Read one line and echo it back uppercased-ish (just echo).
		r := bufio.NewReader(ch)
		line, _ := r.ReadString('\n')
		_, _ = io.WriteString(ch, "ECHO:"+line)
	}

	go func() {
		// serve closes ln on return; ignore the returned error (it'll be a
		// "use of closed network connection" at teardown).
		_ = serve(ln, hostKeyPath, onSession)
	}()

	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String(), seen
}

type seenAddrs struct {
	mu    sync.Mutex
	addrs []string
}

func (s *seenAddrs) add(a string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addrs = append(s.addrs, a)
}

func (s *seenAddrs) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.addrs))
	copy(out, s.addrs)
	return out
}

func dialClient(t *testing.T, addr string) *ssh.Client {
	t.Helper()
	cfg := &ssh.ClientConfig{
		User:            "anyone",
		Auth:            []ssh.AuthMethod{ssh.Password("whatever")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	var (
		client *ssh.Client
		err    error
	)
	// The server goroutine may not have begun accepting yet; retry briefly.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, err = ssh.Dial("tcp", addr, cfg)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("ssh.Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// runInteractive opens a session, requests a pty, starts a shell, sends a line,
// and returns everything read from stdout.
func runInteractive(t *testing.T, client *ssh.Client, input string) string {
	t.Helper()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if err := session.RequestPty("xterm", 24, 80, ssh.TerminalModes{}); err != nil {
		t.Fatalf("RequestPty: %v", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}

	if err := session.Shell(); err != nil {
		t.Fatalf("Shell: %v", err)
	}

	_, _ = io.WriteString(stdin, input)

	// Read all output until the server closes the channel (onSession returns).
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(stdout)
		done <- result{data, err}
	}()

	select {
	case r := <-done:
		return string(r.data)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading session output")
		return ""
	}
}

func TestServe_InteractiveSession(t *testing.T) {
	hostKey := filepath.Join(t.TempDir(), "keys", "ssh_host_key")
	addr, seen := startServer(t, hostKey)

	client := dialClient(t, addr)
	out := runInteractive(t, client, "hello\n")

	if want := banner; len(out) < len(want) || out[:len(want)] != want {
		t.Fatalf("missing banner; got %q", out)
	}
	if wantEcho := "ECHO:hello\n"; !contains(out, wantEcho) {
		t.Fatalf("missing echo %q in %q", wantEcho, out)
	}

	addrs := seen.snapshot()
	if len(addrs) != 1 {
		t.Fatalf("expected 1 onSession invocation, got %d", len(addrs))
	}
	if addrs[0] == "" {
		t.Fatal("onSession got empty remoteAddr")
	}

	// The host key file must have been created.
	if _, err := os.Stat(hostKey); err != nil {
		t.Fatalf("host key not persisted: %v", err)
	}
}

func TestServe_HostKeyReusedAcrossConnections(t *testing.T) {
	hostKey := filepath.Join(t.TempDir(), "ssh_host_key")
	addr, seen := startServer(t, hostKey)

	// First connection generates the key; capture its bytes.
	c1 := dialClient(t, addr)
	_ = runInteractive(t, c1, "one\n")

	keyBytes1, err := os.ReadFile(hostKey)
	if err != nil {
		t.Fatalf("read host key: %v", err)
	}

	// Second connection must reuse the same key (same fingerprint).
	var fp1, fp2 string
	cfg := &ssh.ClientConfig{
		User: "x",
		Auth: []ssh.AuthMethod{ssh.Password("x")},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fp2 = ssh.FingerprintSHA256(key)
			return nil
		},
		Timeout: 5 * time.Second,
	}
	cfg2 := *cfg
	cfg.HostKeyCallback = func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fp1 = ssh.FingerprintSHA256(key)
		return nil
	}

	if c, err := ssh.Dial("tcp", addr, cfg); err == nil {
		c.Close()
	} else {
		t.Fatalf("dial 1: %v", err)
	}
	if c, err := ssh.Dial("tcp", addr, &cfg2); err == nil {
		c.Close()
	} else {
		t.Fatalf("dial 2: %v", err)
	}

	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("fingerprints differ: %q vs %q", fp1, fp2)
	}

	keyBytes2, err := os.ReadFile(hostKey)
	if err != nil {
		t.Fatalf("re-read host key: %v", err)
	}
	if string(keyBytes1) != string(keyBytes2) {
		t.Fatal("host key file changed across connections (regenerated)")
	}

	if got := len(seen.snapshot()); got < 1 {
		t.Fatalf("expected at least 1 onSession call, got %d", got)
	}
}

func TestServe_NonSessionChannelRejected(t *testing.T) {
	hostKey := filepath.Join(t.TempDir(), "ssh_host_key")
	addr, _ := startServer(t, hostKey)
	client := dialClient(t, addr)

	_, _, err := client.OpenChannel("not-a-session", nil)
	if err == nil {
		t.Fatal("expected non-session channel to be rejected")
	}
	oce, ok := err.(*ssh.OpenChannelError)
	if !ok {
		t.Fatalf("expected *ssh.OpenChannelError, got %T: %v", err, err)
	}
	if oce.Reason != ssh.UnknownChannelType {
		t.Fatalf("expected UnknownChannelType, got %v", oce.Reason)
	}

	// A session channel should still work on the same connection afterwards.
	out := runInteractive(t, client, "still\n")
	if !contains(out, banner) {
		t.Fatalf("session after rejection failed; got %q", out)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
