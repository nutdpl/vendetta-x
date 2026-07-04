//go:build linux

package door

import (
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"
)

// Linux ioctls for /dev/ptmx (asm-generic values, correct on amd64/arm64).
const (
	tiocSPTLCK = 0x40045431 // (un)lock the pty slave
	tiocGPTN   = 0x80045430 // get the pty slave number
)

// openPTY allocates a new pseudo-terminal pair by driving /dev/ptmx directly,
// so the door layer needs no third-party pty dependency.
func openPTY() (master, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	var unlock int32 // 0 = unlock
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), tiocSPTLCK, uintptr(unsafe.Pointer(&unlock))); e != 0 {
		m.Close()
		return nil, nil, e
	}
	var n int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), tiocGPTN, uintptr(unsafe.Pointer(&n))); e != 0 {
		m.Close()
		return nil, nil, e
	}
	s, err := os.OpenFile("/dev/pts/"+strconv.Itoa(int(n)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	return m, s, nil
}

// runPTY runs cmd attached to a fresh pseudo-terminal (so the door sees a real
// tty: raw keys, isatty true, the terminal a DOS-door emulator expects) and
// bridges the caller (rw) to the pty master. cmd must not have been started.
// It returns errNoPTY (without starting cmd) if a pty can't be allocated, so the
// caller can fall back to plain pipes.
func runPTY(cmd *exec.Cmd, rw io.ReadWriteCloser) error {
	master, slave, err := openPTY()
	if err != nil {
		return errNoPTY
	}

	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	// New session + make the pty the controlling terminal for the door.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}

	if err := cmd.Start(); err != nil {
		master.Close()
		slave.Close()
		return err
	}
	// The child owns the slave now; the parent only needs the master.
	slave.Close()

	// caller -> door. Not waited on: the caller may sit idle, and closing the
	// master on teardown unblocks this copy.
	go func() { io.Copy(master, rw) }()

	// door -> caller. When the door exits (all slave fds gone) the master
	// read drains the pty buffer and then errors (EIO on Linux), ending the
	// copy with every byte delivered.
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		io.Copy(rw, master)
	}()

	// Drain the output BEFORE closing the master: closing first can discard
	// the door's final burst still queued in the pty buffer (the same
	// lost-last-screen race the pipe bridge had). If the caller hangs up
	// instead, the copy ends on its write error -- either way this returns.
	<-copyDone
	werr := cmd.Wait()
	master.Close() // unblock the input copier
	return werr
}
