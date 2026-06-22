//go:build !linux

package door

import (
	"io"
	"os/exec"
)

// runPTY is Linux-only; elsewhere there is no /dev/ptmx path, so the door always
// falls back to the plain-pipe bridge.
func runPTY(cmd *exec.Cmd, rw io.ReadWriteCloser) error { return errNoPTY }
