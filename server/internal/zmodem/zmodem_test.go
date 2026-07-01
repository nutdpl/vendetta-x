package zmodem

import (
	"bytes"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// torture returns data covering every byte value (including ZDLE, XON/XOFF,
// IAC, CR-after-@) plus a pseudo-random tail -- the bytes most likely to
// expose escaping bugs.
func torture(n int) []byte {
	data := make([]byte, 0, n)
	for i := 0; i < 256; i++ {
		data = append(data, byte(i))
	}
	data = append(data, '@', '\r', '@', 0x8D, 0x18, 0x18, 0x18, 0x18, 0x18)
	rng := rand.New(rand.NewSource(1994))
	for len(data) < n {
		data = append(data, byte(rng.Intn(256)))
	}
	return data[:n]
}

func TestCRC16Vector(t *testing.T) {
	// CRC-16/XMODEM check value.
	if got := crc16([]byte("123456789")); got != 0x31C3 {
		t.Fatalf("crc16 = %#04x, want 0x31c3", got)
	}
}

// pipeRW joins a Reader and Writer into an io.ReadWriter.
type pipeRW struct {
	io.Reader
	io.Writer
}

// duplex builds a bidirectional link from two OS pipes. Unlike io.Pipe/
// net.Pipe (fully synchronous), OS pipes carry a real kernel buffer, so the
// protocol's opening writes (both ends write first, like on a socket) don't
// deadlock -- the same conditions as the TCP/ssh transports it runs on.
func duplex(t *testing.T) (a, b io.ReadWriter) {
	t.Helper()
	aR, bW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	bR, aW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { aR.Close(); aW.Close(); bR.Close(); bW.Close() })
	return pipeRW{aR, aW}, pipeRW{bR, bW}
}

// TestLoopback runs our sender against our receiver.
func TestLoopback(t *testing.T) {
	data := torture(200_000)
	a, b := duplex(t)

	errc := make(chan error, 1)
	go func() {
		errc <- Send(a, "torture.bin", time.Unix(1_000_000, 0), data)
	}()
	f, err := Receive(b, 1<<20)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("Send: %v", err)
	}
	if f.Name != "torture.bin" {
		t.Errorf("name = %q", f.Name)
	}
	if !bytes.Equal(f.Data, data) {
		t.Fatalf("data mismatch: got %d bytes want %d", len(f.Data), len(data))
	}
}

// TestLoopbackTooLarge verifies the receive cap trips.
func TestLoopbackTooLarge(t *testing.T) {
	data := torture(60_000)
	a, b := duplex(t)
	go Send(a, "big.bin", time.Now(), data) // outcome irrelevant
	_, err := Receive(b, 10_000)
	if _, ok := err.(ErrTooLarge); !ok {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

// lrzsz reference-tool round trips. Skipped when the binaries are absent.

func requireTool(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not installed; skipping reference interop test", name)
	}
	return p
}

// TestSendToRealRZ streams a file from our sender into lrzsz's rz.
func TestSendToRealRZ(t *testing.T) {
	rz := requireTool(t, "rz")
	dir := t.TempDir()
	data := torture(150_000)

	cmd := exec.Command(rz, "--binary", "--quiet", "--overwrite")
	cmd.Dir = dir
	toRZ, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	fromRZ, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	timer := time.AfterFunc(30*time.Second, func() { cmd.Process.Kill() })
	defer timer.Stop()

	if err := Send(pipeRW{fromRZ, toRZ}, "landed.bin", time.Unix(1_700_000_000, 0), data); err != nil {
		t.Fatalf("Send: %v", err)
	}
	toRZ.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("rz exited: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "landed.bin"))
	if err != nil {
		t.Fatalf("rz did not write the file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("file corrupt: got %d bytes want %d", len(got), len(data))
	}
}

// TestReceiveFromRealSZ accepts an upload from lrzsz's sz.
func TestReceiveFromRealSZ(t *testing.T) {
	sz := requireTool(t, "sz")
	dir := t.TempDir()
	data := torture(150_000)
	src := filepath.Join(dir, "upload.bin")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(sz, "--binary", "--quiet", src)
	toSZ, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	fromSZ, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	timer := time.AfterFunc(30*time.Second, func() { cmd.Process.Kill() })
	defer timer.Stop()

	f, err := Receive(pipeRW{fromSZ, toSZ}, 1<<20)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("sz exited: %v", err)
	}
	if f.Name != "upload.bin" {
		t.Errorf("name = %q", f.Name)
	}
	if !bytes.Equal(f.Data, data) {
		t.Fatalf("data corrupt: got %d bytes want %d", len(f.Data), len(data))
	}
}

// TestParseFileInfo covers the odd shapes senders emit.
func TestParseFileInfo(t *testing.T) {
	cases := []struct {
		info string
		name string
		size int64
	}{
		{"file.zip\x00151400 17040242536 100644 0 1 151400\x00", "file.zip", 151400},
		{"noprops.bin\x00", "noprops.bin", 0},
		{"../../etc/passwd\x00123 0\x00", "passwd", 123},
		{"C:\\WAREZ\\GAME.ZIP\x0042\x00", "GAME.ZIP", 42},
	}
	for _, tc := range cases {
		name, size := parseFileInfo([]byte(tc.info))
		if name != tc.name || size != tc.size {
			t.Errorf("parseFileInfo(%q) = %q,%d want %q,%d", tc.info, name, size, tc.name, tc.size)
		}
	}
}
