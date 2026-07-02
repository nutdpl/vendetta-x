// Package ftp is a minimal FTP client -- just enough of RFC 959 for a QWK-net
// node to exchange packets with its hub (log in, fetch a file, store a file).
// Passive mode only, binary type only, standard library only. It is NOT a
// general-purpose client: no directory listings, no TLS, no active mode.
//
// Vendetta/X speaks FTP because that is what real QWK network hubs
// (Synchronet boards carrying DOVE-Net and friends) actually serve for
// unattended packet exchange.
package ftp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Client is one logged-in FTP control connection.
type Client struct {
	conn    net.Conn
	r       *bufio.Reader
	timeout time.Duration
}

// maxFetchBytes caps a RETR download, guarding against a hostile or broken
// hub streaming forever. QWK packets are far below this.
const maxFetchBytes = 64 << 20 // 64 MiB

// Dial connects to addr (host:port), waits for the server greeting, and
// returns a Client ready for Login. timeout bounds every subsequent network
// operation (dial, reads, writes) individually.
func Dial(addr string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("ftp: dial %s: %w", addr, err)
	}
	c := &Client{conn: conn, r: bufio.NewReader(conn), timeout: timeout}
	if code, msg, err := c.readReply(); err != nil {
		conn.Close()
		return nil, err
	} else if code != 220 {
		conn.Close()
		return nil, fmt.Errorf("ftp: greeting %d %s", code, msg)
	}
	return c, nil
}

// Login authenticates and switches to binary (image) type.
func (c *Client) Login(user, pass string) error {
	code, msg, err := c.cmd("USER " + user)
	if err != nil {
		return err
	}
	if code == 331 || code == 332 {
		if code, msg, err = c.cmd("PASS " + pass); err != nil {
			return err
		}
	}
	if code != 230 && code != 202 {
		return fmt.Errorf("ftp: login refused: %d %s", code, msg)
	}
	if code, msg, err = c.cmd("TYPE I"); err != nil {
		return err
	} else if code != 200 {
		return fmt.Errorf("ftp: TYPE I refused: %d %s", code, msg)
	}
	return nil
}

// Retr downloads the named file. A "file not found" style refusal (550)
// returns (nil, nil) -- for a QWK node, no packet waiting is a normal outcome,
// not an error.
func (c *Client) Retr(name string) ([]byte, error) {
	data, err := c.pasv()
	if err != nil {
		return nil, err
	}
	code, msg, err := c.cmd("RETR " + name)
	if err != nil {
		data.Close()
		return nil, err
	}
	if code == 550 {
		data.Close()
		return nil, nil // nothing waiting
	}
	if code != 125 && code != 150 {
		data.Close()
		return nil, fmt.Errorf("ftp: RETR %s: %d %s", name, code, msg)
	}

	data.SetReadDeadline(time.Now().Add(c.timeout))
	body, rerr := io.ReadAll(io.LimitReader(data, maxFetchBytes+1))
	data.Close()
	if rerr != nil {
		return nil, fmt.Errorf("ftp: RETR %s read: %w", name, rerr)
	}
	if len(body) > maxFetchBytes {
		return nil, fmt.Errorf("ftp: RETR %s: file too large", name)
	}

	if code, msg, err = c.readReply(); err != nil {
		return nil, err
	} else if code != 226 && code != 250 {
		return nil, fmt.Errorf("ftp: RETR %s close: %d %s", name, code, msg)
	}
	return body, nil
}

// Stor uploads content as the named file.
func (c *Client) Stor(name string, content []byte) error {
	data, err := c.pasv()
	if err != nil {
		return err
	}
	code, msg, err := c.cmd("STOR " + name)
	if err != nil {
		data.Close()
		return err
	}
	if code != 125 && code != 150 {
		data.Close()
		return fmt.Errorf("ftp: STOR %s: %d %s", name, code, msg)
	}

	data.SetWriteDeadline(time.Now().Add(c.timeout))
	_, werr := data.Write(content)
	data.Close() // EOF tells the server the transfer is complete
	if werr != nil {
		return fmt.Errorf("ftp: STOR %s write: %w", name, werr)
	}

	if code, msg, err = c.readReply(); err != nil {
		return err
	} else if code != 226 && code != 250 {
		return fmt.Errorf("ftp: STOR %s close: %d %s", name, code, msg)
	}
	return nil
}

// Delete removes the named remote file. Some hubs auto-delete a fetched QWK;
// for the rest, deleting after a successful import prevents re-downloading.
// A 550 (no such file) is not an error.
func (c *Client) Delete(name string) error {
	code, msg, err := c.cmd("DELE " + name)
	if err != nil {
		return err
	}
	if code != 250 && code != 550 {
		return fmt.Errorf("ftp: DELE %s: %d %s", name, code, msg)
	}
	return nil
}

// Quit says goodbye and closes the control connection.
func (c *Client) Quit() error {
	c.cmd("QUIT") // best effort; the close is what matters
	return c.conn.Close()
}

// pasv issues PASV, parses the host,port tuple, and dials the data connection.
func (c *Client) pasv() (net.Conn, error) {
	code, msg, err := c.cmd("PASV")
	if err != nil {
		return nil, err
	}
	if code != 227 {
		return nil, fmt.Errorf("ftp: PASV refused: %d %s", code, msg)
	}
	addr, err := parsePasv(msg)
	if err != nil {
		return nil, err
	}
	// Some servers advertise an internal IP in PASV; trusting the control
	// connection's host instead is the standard client workaround.
	if host, _, err := net.SplitHostPort(c.conn.RemoteAddr().String()); err == nil {
		if _, port, err2 := net.SplitHostPort(addr); err2 == nil {
			addr = net.JoinHostPort(host, port)
		}
	}
	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("ftp: data dial %s: %w", addr, err)
	}
	return conn, nil
}

// parsePasv extracts host:port from a 227 reply's "(h1,h2,h3,h4,p1,p2)"
// tuple. Parentheses are customary but not guaranteed, so it scans for the
// first run of six comma-separated integers.
func parsePasv(msg string) (string, error) {
	start := strings.IndexAny(msg, "0123456789")
	if start < 0 {
		return "", errors.New("ftp: unparseable PASV reply: " + msg)
	}
	fields := strings.FieldsFunc(msg[start:], func(r rune) bool { return r == ',' })
	if len(fields) < 6 {
		return "", errors.New("ftp: short PASV tuple: " + msg)
	}
	nums := make([]int, 6)
	for i := 0; i < 6; i++ {
		f := strings.TrimSpace(fields[i])
		// The final field may carry trailing junk like ")." -- trim non-digits.
		f = strings.TrimRightFunc(f, func(r rune) bool { return r < '0' || r > '9' })
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 || n > 255 {
			return "", errors.New("ftp: bad PASV tuple: " + msg)
		}
		nums[i] = n
	}
	host := fmt.Sprintf("%d.%d.%d.%d", nums[0], nums[1], nums[2], nums[3])
	port := nums[4]<<8 | nums[5]
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// cmd sends one command line and reads the reply.
func (c *Client) cmd(line string) (int, string, error) {
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write([]byte(line + "\r\n")); err != nil {
		return 0, "", fmt.Errorf("ftp: send %q: %w", strings.Fields(line)[0], err)
	}
	return c.readReply()
}

// readReply reads one (possibly multiline) FTP reply: "123 text", or
// "123-text ... 123 text" spanning several lines.
func (c *Client) readReply() (int, string, error) {
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	line, err := c.r.ReadString('\n')
	if err != nil {
		return 0, "", fmt.Errorf("ftp: read reply: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) < 3 {
		return 0, "", errors.New("ftp: short reply: " + line)
	}
	code, err := strconv.Atoi(line[:3])
	if err != nil {
		return 0, "", errors.New("ftp: malformed reply: " + line)
	}

	msg := strings.TrimSpace(line[3:])
	if len(line) > 3 && line[3] == '-' {
		// Multiline: read until "code " terminator line.
		term := line[:3] + " "
		for {
			c.conn.SetReadDeadline(time.Now().Add(c.timeout))
			next, err := c.r.ReadString('\n')
			if err != nil {
				return 0, "", fmt.Errorf("ftp: read multiline reply: %w", err)
			}
			next = strings.TrimRight(next, "\r\n")
			if strings.HasPrefix(next, term) {
				msg = strings.TrimSpace(next[4:])
				break
			}
		}
	}
	return code, msg, nil
}
