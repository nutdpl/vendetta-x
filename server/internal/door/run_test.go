package door

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeRW stands in for a caller's terminal: it supplies optional input and
// captures everything the door writes back.
type fakeRW struct {
	in  io.Reader
	mu  sync.Mutex
	out bytes.Buffer
}

func (f *fakeRW) Read(p []byte) (int, error) { return f.in.Read(p) }
func (f *fakeRW) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.Write(p)
}
func (f *fakeRW) Close() error { return nil }
func (f *fakeRW) got() string  { f.mu.Lock(); defer f.mu.Unlock(); return f.out.String() }

// ttyProbe writes a tiny shell script that reports whether its stdin/stdout are
// a tty, and returns a Door whose command runs it. The script path has no spaces
// (t.TempDir), so the naive argv split handles "sh <path>" correctly.
func ttyProbe(t *testing.T) (Door, Caller, System) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "probe.sh")
	body := "#!/bin/sh\nif [ -t 0 ] && [ -t 1 ]; then printf 'TTY_OK\\n'; else printf 'NO_TTY\\n'; fi\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	d := Door{Name: "probe", WorkDir: dir, DropType: "DOOR.SYS", Enabled: true, Command: "sh " + script}
	c := Caller{Node: 1, Handle: "tester", SL: 10, MinutesLeft: 5, Emulation: 1}
	return d, c, System{Name: "Test BBS", Sysop: "nut"}
}

// TestRunGivesDoorATTY is the core fix: Run must hand the door a real terminal
// (so DOS-door emulators like dosemu2 and tty-expecting native doors work),
// not a plain pipe. On Linux this exercises the pty path.
func TestRunGivesDoorATTY(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	d, c, sys := ttyProbe(t)
	rw := &fakeRW{in: strings.NewReader("")}
	if err := d.Run(c, sys, rw); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(rw.got(), "TTY_OK") {
		t.Fatalf("door was not given a tty; output = %q", rw.got())
	}
}

// TestRunPipesFallback verifies the plain-pipe bridge still runs the door (it
// just isn't a tty) -- the fallback for platforms/hosts without a pty.
func TestRunPipesFallback(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	d, _, _ := ttyProbe(t)
	argv := strings.Fields(d.Command)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = d.WorkDir
	rw := &fakeRW{in: strings.NewReader("")}
	if err := runPipes(cmd, rw); err != nil {
		t.Fatalf("runPipes: %v", err)
	}
	if !strings.Contains(rw.got(), "NO_TTY") {
		t.Fatalf("pipe bridge unexpectedly looked like a tty; output = %q", rw.got())
	}
}

// TestRunUnavailable: a command whose binary doesn't exist returns ErrUnavailable
// (never a hang/panic), and an empty command returns ErrNotConfigured.
func TestRunSentinels(t *testing.T) {
	rw := &fakeRW{in: strings.NewReader("")}
	d := Door{Command: "", WorkDir: t.TempDir()}
	if err := d.Run(Caller{}, System{}, rw); err != ErrNotConfigured {
		t.Fatalf("empty command: got %v, want ErrNotConfigured", err)
	}
	d = Door{Command: "definitely-not-a-real-binary-xyz123", WorkDir: t.TempDir()}
	if err := d.Run(Caller{}, System{}, rw); err != ErrUnavailable {
		t.Fatalf("missing binary: got %v, want ErrUnavailable", err)
	}
}
