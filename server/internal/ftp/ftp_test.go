package ftp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer is a tiny in-process FTP server speaking just enough real
// protocol (over real TCP sockets) to exercise the client: USER/PASS gate,
// TYPE, PASV with a fresh data listener per transfer, RETR from a file map,
// STOR into it, DELE, QUIT. It intentionally uses multiline replies for the
// greeting to exercise that path too.
type fakeServer struct {
	ln    net.Listener
	user  string
	pass  string
	mu    sync.Mutex
	files map[string][]byte
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeServer{ln: ln, user: "vendx", pass: "secret", files: map[string][]byte{}}
	go s.serve()
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *fakeServer) put(name string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[name] = data
}

func (s *fakeServer) get(name string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.files[name]
	return b, ok
}

func (s *fakeServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.session(conn)
	}
}

func (s *fakeServer) session(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	reply := func(text string) { fmt.Fprintf(conn, "%s\r\n", text) }

	// Multiline greeting on purpose.
	reply("220-fakehub FTP ready")
	reply("220-carrying DOVE-Net since 1994")
	reply("220 ok")

	var dataLn net.Listener
	loggedIn := false
	gotUser := ""
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(strings.TrimRight(line, "\r\n"), " ")
		switch strings.ToUpper(verb) {
		case "USER":
			gotUser = arg
			reply("331 password required")
		case "PASS":
			if gotUser == s.user && arg == s.pass {
				loggedIn = true
				reply("230 welcome")
			} else {
				reply("530 bad credentials")
			}
		case "TYPE":
			reply("200 type set")
		case "PASV":
			if dataLn != nil {
				dataLn.Close()
			}
			var err error
			dataLn, err = net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				reply("425 cannot listen")
				continue
			}
			port := dataLn.Addr().(*net.TCPAddr).Port
			// Advertise a bogus host on purpose: real hubs behind NAT do
			// this, and the client must fall back to the control host.
			reply(fmt.Sprintf("227 Entering Passive Mode (10,0,0,99,%d,%d)", port>>8, port&0xFF))
		case "RETR":
			if !loggedIn {
				reply("530 not logged in")
				continue
			}
			body, ok := s.get(arg)
			if !ok {
				reply("550 no such file")
				continue
			}
			reply("150 opening data connection")
			dc, err := dataLn.Accept()
			if err != nil {
				reply("426 data failed")
				continue
			}
			dc.Write(body)
			dc.Close()
			reply("226 transfer complete")
		case "STOR":
			if !loggedIn {
				reply("530 not logged in")
				continue
			}
			reply("150 ready for data")
			dc, err := dataLn.Accept()
			if err != nil {
				reply("426 data failed")
				continue
			}
			body, _ := io.ReadAll(dc)
			dc.Close()
			s.put(arg, body)
			reply("226 stored")
		case "DELE":
			s.mu.Lock()
			_, existed := s.files[arg]
			delete(s.files, arg)
			s.mu.Unlock()
			if existed {
				reply("250 deleted")
			} else {
				reply("550 no such file")
			}
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not implemented")
		}
	}
}

func dialAndLogin(t *testing.T, s *fakeServer) *Client {
	t.Helper()
	c, err := Dial(s.ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { c.Quit() })
	if err := c.Login("vendx", "secret"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	return c
}

func TestRetrStorDelete(t *testing.T) {
	s := newFakeServer(t)
	packet := bytes.Repeat([]byte("QWKDATA!"), 1000)
	s.put("VENDX.QWK", packet)

	c := dialAndLogin(t, s)

	got, err := c.Retr("VENDX.QWK")
	if err != nil {
		t.Fatalf("Retr: %v", err)
	}
	if !bytes.Equal(got, packet) {
		t.Fatalf("Retr returned %d bytes, want %d (content mismatch)", len(got), len(packet))
	}

	// Missing file: nil, nil -- not an error.
	got, err = c.Retr("NOPE.QWK")
	if err != nil || got != nil {
		t.Fatalf("Retr(missing) = %v bytes, err %v; want nil, nil", got, err)
	}

	rep := []byte("this is a rep packet")
	if err := c.Stor("VENDX.REP", rep); err != nil {
		t.Fatalf("Stor: %v", err)
	}
	stored, ok := s.get("VENDX.REP")
	if !ok || !bytes.Equal(stored, rep) {
		t.Fatalf("server stored %q, want %q", stored, rep)
	}

	if err := c.Delete("VENDX.QWK"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.get("VENDX.QWK"); ok {
		t.Fatal("file still present after Delete")
	}
	// Deleting a missing file is fine.
	if err := c.Delete("VENDX.QWK"); err != nil {
		t.Fatalf("Delete(missing): %v", err)
	}
}

func TestBadLogin(t *testing.T) {
	s := newFakeServer(t)
	c, err := Dial(s.ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Quit()
	if err := c.Login("vendx", "wrong"); err == nil {
		t.Fatal("bad password accepted")
	}
}

func TestParsePasv(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"Entering Passive Mode (192,168,1,2,7,208)", "192.168.1.2:2000", true},
		{"Entering Passive Mode 192,168,1,2,7,208", "192.168.1.2:2000", true},
		{"=127,0,0,1,234,24", "127.0.0.1:59928", true},
		{"Entering Passive Mode (192,168,1,2,7,208).", "192.168.1.2:2000", true},
		{"no numbers here", "", false},
		{"1,2,3", "", false},
	}
	for _, c := range cases {
		got, err := parsePasv(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("parsePasv(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("parsePasv(%q) should fail, got %q", c.in, got)
		}
	}
}
