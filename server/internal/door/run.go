package door

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// ErrNotConfigured means the door has no command set.
var ErrNotConfigured = errors.New("door: not configured")

// ErrUnavailable means the door's binary could not be located (not on PATH and
// not an existing file), so the door can't run on this host.
var ErrUnavailable = errors.New("door: binary unavailable")

// Run launches the door and bridges the caller's terminal (rw) to the door
// process's stdin/stdout/stderr. It writes the drop file first, then execs the
// configured command with cmd.Dir set to the work dir. The bridge ends when the
// process exits or rw returns EOF; both copy goroutines are torn down on exit.
// A misconfigured or absent door returns a sentinel error, never a panic/hang.
func (d Door) Run(c Caller, sys System, rw io.ReadWriteCloser) error {
	if strings.TrimSpace(d.Command) == "" {
		return ErrNotConfigured
	}
	argv := strings.Fields(d.Command)
	if len(argv) == 0 {
		return ErrNotConfigured
	}

	// Resolve the binary: it must be on PATH or be an existing file.
	bin := argv[0]
	if !binaryAvailable(bin) {
		return ErrUnavailable
	}

	// Write the drop file (best effort but report a hard failure). A door that
	// can't get its drop file would misbehave, so surface the error.
	if _, err := d.WriteDropFile(c, sys); err != nil {
		return err
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	if dir := strings.TrimSpace(d.WorkDir); dir != "" {
		cmd.Dir = dir
	} else {
		cmd.Dir = "."
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return err
	}

	var wg sync.WaitGroup

	// rw -> process stdin. When rw hits EOF (carrier loss) we close the stdin
	// pipe so the door sees end-of-input; we do NOT block process teardown on
	// this goroutine (the caller may sit idle), so it is not waited on.
	go func() {
		io.Copy(stdin, rw)
		stdin.Close()
	}()

	// process stdout/stderr -> rw. This goroutine ends when the process closes
	// its stdout (i.e. on exit), which is our reliable teardown signal.
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(rw, stdout)
	}()

	werr := cmd.Wait()
	// Process is gone; stdout pipe is closed by Wait, so the stdout copier will
	// observe EOF and return. Drain it before returning.
	wg.Wait()
	// Best-effort: closing stdin unblocks the input copier if it is parked.
	stdin.Close()

	return werr
}

// binaryAvailable reports whether bin can be executed: either it resolves on
// PATH, or it is a path to an existing file.
func binaryAvailable(bin string) bool {
	if bin == "" {
		return false
	}
	if _, err := exec.LookPath(bin); err == nil {
		return true
	}
	if strings.ContainsAny(bin, "/\\") {
		if info, err := os.Stat(bin); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
