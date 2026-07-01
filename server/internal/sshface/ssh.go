// Package sshface serves the Vendetta/X board over SSH -- the third way in,
// alongside telnet and the web. It performs the SSH handshake and, for each
// interactive (pty + shell) session, hands the channel to a callback that runs
// the exact same board flow as telnet (via term.NewRW + board.runBoard).
//
// This file is the CONTRACT. Serve is a stub until implemented.
package sshface

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"

	"golang.org/x/crypto/ssh"
)

// recoverConn isolates a per-connection/per-channel goroutine: a panic in the
// SSH library (malformed handshake, channel, or request from a hostile client)
// is logged and contained, never taking down the listener or the other faces.
func recoverConn(who, remoteAddr string) {
	if r := recover(); r != nil {
		log.Printf("sshface: %s panic (%s): %v\n%s", who, remoteAddr, r, debug.Stack())
	}
}

// Serve listens for SSH connections on addr and, for every authenticated
// interactive session, calls onSession with the session channel (an
// io.ReadWriteCloser carrying the terminal stream) and the caller's remote
// address. It blocks; callers run it in a goroutine.
//
// hostKeyPath is where the server's persistent host key lives; it is generated
// on first run if absent (so the host fingerprint is stable across restarts).
//
// Authentication here is permissive (the board runs its own bcrypt login at the
// matrix screen, just like telnet) -- SSH auth only establishes the transport.
func Serve(addr, hostKeyPath string, onSession func(ch io.ReadWriteCloser, remoteAddr, term string)) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return serve(ln, hostKeyPath, onSession)
}

// ServeListener is Serve over an already-open listener, so the caller owns the
// listener's lifetime (and can Close it to stop serving for graceful shutdown).
// It closes ln when it returns.
func ServeListener(ln net.Listener, hostKeyPath string, onSession func(ch io.ReadWriteCloser, remoteAddr, term string)) error {
	return serve(ln, hostKeyPath, onSession)
}

// serve is the testable core of Serve: it owns an already-open listener so a
// test can pass a net.Listen("tcp", "127.0.0.1:0") and read ln.Addr(). It
// closes ln when it returns. Exported Serve simply wraps it.
func serve(ln net.Listener, hostKeyPath string, onSession func(ch io.ReadWriteCloser, remoteAddr, term string)) error {
	defer ln.Close()

	signer, err := loadOrCreateHostKey(hostKeyPath)
	if err != nil {
		return err
	}

	config := &ssh.ServerConfig{
		// Permissive auth: the board runs its own bcrypt login. SSH auth here
		// only establishes the encrypted transport. Accept any credentials via
		// both password and keyboard-interactive so OpenSSH and PuTTY both get
		// in. (A config with zero auth methods is rejected by x/crypto/ssh.)
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
		KeyboardInteractiveCallback: func(ssh.ConnMetadata, ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(signer)

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed or fatal accept error: stop serving.
			return err
		}
		go handleConn(conn, config, onSession)
	}
}

// handleConn performs the SSH handshake for a single TCP connection and then
// services its channels. It never panics on a malformed handshake or an early
// disconnect -- it logs and returns, leaving other connections untouched.
func handleConn(conn net.Conn, config *ssh.ServerConfig, onSession func(ch io.ReadWriteCloser, remoteAddr, term string)) {
	remoteAddr := conn.RemoteAddr().String()
	defer recoverConn("handleConn", remoteAddr)
	defer conn.Close() // belt-and-suspenders: ensure the fd is reclaimed on any exit

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		// Handshake failed (port scanner, incompatible client, mid-negotiation
		// disconnect). Nothing to clean up beyond the conn, which NewServerConn
		// already closed.
		log.Printf("sshface: handshake from %s failed: %v", remoteAddr, err)
		return
	}
	defer sshConn.Close()

	// Out-of-band global requests are not used by the board; discard them.
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		go handleChannel(newChannel, remoteAddr, onSession)
	}
}

// handleChannel accepts a session channel and services its requests. When a
// "shell" (or "exec") request arrives the interactive session is starting, so
// it invokes onSession with the channel. Each channel runs in its own
// goroutine so one slow caller never blocks others.
func handleChannel(newChannel ssh.NewChannel, remoteAddr string, onSession func(ch io.ReadWriteCloser, remoteAddr, term string)) {
	defer recoverConn("handleChannel", remoteAddr)

	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Printf("sshface: accept channel from %s failed: %v", remoteAddr, err)
		return
	}

	started := false
	term := ""
	for req := range requests {
		switch req.Type {
		case "pty-req", "shell", "exec":
			// Reply true to these so the client proceeds to allocate a terminal
			// and start the program. The shell/exec request is the cue that the
			// interactive session is beginning. The pty request also names the
			// client's TERM (RFC 4254 6.2: string TERM, then dimensions) -- the
			// board uses it to pick the session charset (SyncTERM = CP437,
			// xterm-family = UTF-8).
			if req.Type == "pty-req" {
				term = parsePtyTerm(req.Payload)
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			if (req.Type == "shell" || req.Type == "exec") && !started {
				started = true
				// Run the board, then tear the channel down cleanly. We break
				// out of the request loop afterward: once the board session is
				// over there is nothing more to service.
				runSession(channel, remoteAddr, term, onSession)
				return
			}
		case "env", "window-change":
			// Accepted and ignored; the board reads the stream directly.
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}

	// The request channel closed before a shell/exec ever arrived (client gave
	// up). Make sure the channel is closed.
	if !started {
		_ = channel.Close()
	}
}

// runSession hands the raw channel to onSession, then closes it and reports a
// clean exit status. The channel carries a pristine terminal stream -- no
// telnet IAC bytes are written here (that is the telnet face's job).
func runSession(channel ssh.Channel, remoteAddr, term string, onSession func(ch io.ReadWriteCloser, remoteAddr, term string)) {
	defer channel.Close()

	if onSession != nil {
		onSession(channel, remoteAddr, term)
	}

	// Best-effort exit status 0 so well-behaved clients see a clean shell exit.
	// Errors here are non-fatal (the peer may already be gone).
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
}

// parsePtyTerm extracts the TERM string from a pty-req payload (RFC 4254
// 6.2): uint32 length + TERM, followed by dimensions we don't need. Returns
// "" for a short or malformed payload.
func parsePtyTerm(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	n := int(payload[0])<<24 | int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if n < 0 || n > 64 || 4+n > len(payload) {
		return ""
	}
	return string(payload[4 : 4+n])
}

// loadOrCreateHostKey loads the PEM-encoded RSA host key at path, or, if the
// file does not exist, generates a fresh RSA-2048 key, persists it (0600, with
// parent dirs created as needed), and returns a signer. Reusing the persisted
// key keeps the host fingerprint stable across restarts.
func loadOrCreateHostKey(path string) (ssh.Signer, error) {
	if data, err := os.ReadFile(path); err == nil {
		signer, perr := ssh.ParsePrivateKey(data)
		if perr != nil {
			return nil, perr
		}
		return signer, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	// Generate a new key.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}

	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, err
	}
	return signer, nil
}
