package binkp

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeHub is just enough binkd to prove the client: handshake (optionally
// CRAM-MD5), receive the caller's files, send its own, EOB both ways.
type fakeHub struct {
	ln       net.Listener
	password string
	cram     bool
	sendFile *File

	gotFiles []File
	gotPwd   string
	err      error
	done     chan struct{}
}

func startHub(t *testing.T, password string, cram bool, send *File) *fakeHub {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	h := &fakeHub{ln: ln, password: password, cram: cram, sendFile: send, done: make(chan struct{})}
	go h.serve()
	t.Cleanup(func() { ln.Close() })
	return h
}

func (h *fakeHub) serve() {
	defer close(h.done)
	conn, err := h.ln.Accept()
	if err != nil {
		h.err = err
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	wcmd := func(c byte, arg string) {
		payload := append([]byte{c}, arg...)
		var hd [2]byte
		binary.BigEndian.PutUint16(hd[:], uint16(len(payload))|0x8000)
		conn.Write(hd[:])
		conn.Write(payload)
	}
	wdata := func(b []byte) {
		var hd [2]byte
		binary.BigEndian.PutUint16(hd[:], uint16(len(b)))
		conn.Write(hd[:])
		conn.Write(b)
	}
	read := func() (byte, []byte, bool) {
		var hd [2]byte
		if _, err := io.ReadFull(conn, hd[:]); err != nil {
			return 0, nil, false
		}
		v := binary.BigEndian.Uint16(hd[:])
		buf := make([]byte, v&0x7FFF)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return 0, nil, false
		}
		if v&0x8000 != 0 {
			return buf[0], buf[1:], true
		}
		return 0xFF, buf, true // 0xFF marks data
	}

	challenge := []byte("0123456789abcdef")
	wcmd(mNUL, "SYS fake hub")
	if h.cram {
		wcmd(mNUL, "OPT CRAM-MD5-"+hex.EncodeToString(challenge))
	}
	wcmd(mADR, "21:1/100")

	// Read caller frames until password arrives.
	for {
		c, payload, ok := read()
		if !ok {
			h.err = io.ErrUnexpectedEOF
			return
		}
		if c == mPWD {
			h.gotPwd = string(payload)
			break
		}
	}
	if h.cram {
		mac := hmac.New(md5.New, []byte(h.password))
		mac.Write(challenge)
		want := "CRAM-MD5-" + hex.EncodeToString(mac.Sum(nil))
		if h.gotPwd != want {
			wcmd(mERR, "bad password")
			return
		}
	} else if h.gotPwd != h.password {
		wcmd(mERR, "bad password")
		return
	}
	wcmd(mOK, "secure")

	// Receive caller files until caller EOB.
	var cur *File
	var curSize int64
recv:
	for {
		c, payload, ok := read()
		if !ok {
			h.err = io.ErrUnexpectedEOF
			return
		}
		switch c {
		case mFILE:
			f := strings.Fields(string(payload))
			size, _ := strconv.ParseInt(f[1], 10, 64)
			cur = &File{Name: f[0]}
			curSize = size
			if size == 0 {
				h.gotFiles = append(h.gotFiles, *cur)
				cur = nil
			}
		case 0xFF: // data
			if cur != nil {
				cur.Data = append(cur.Data, payload...)
				if int64(len(cur.Data)) >= curSize {
					wcmd(mGOT, cur.Name)
					h.gotFiles = append(h.gotFiles, *cur)
					cur = nil
				}
			}
		case mEOB:
			break recv
		}
	}

	// Send our own file, then EOB, then wait for the caller's M_GOT.
	if h.sendFile != nil {
		wcmd(mFILE, h.sendFile.Name+" "+strconv.Itoa(len(h.sendFile.Data))+" "+
			strconv.FormatInt(time.Now().Unix(), 10)+" 0")
		wdata(h.sendFile.Data)
	}
	wcmd(mEOB, "")
	for h.sendFile != nil {
		c, _, ok := read()
		if !ok {
			return // caller may hang up right after M_GOT; fine
		}
		if c == mGOT {
			return
		}
	}
}

func TestSessionPlainPassword(t *testing.T) {
	inboundPkt := &File{Name: "00000001.pkt", Data: []byte("HUB PACKET BYTES")}
	h := startHub(t, "s3cret", false, inboundPkt)

	out := []File{{Name: "0abc.pkt", Data: []byte("OUR PACKET"), Time: time.Now()}}
	in, err := Session(Config{
		HostPort: h.ln.Addr().String(), OurAddr: "21:1/999",
		Password: "s3cret", SysName: "Vendetta/X", Sysop: "nut",
		Timeout: 10 * time.Second,
	}, out)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	<-h.done
	if h.err != nil {
		t.Fatalf("hub: %v", h.err)
	}
	if len(h.gotFiles) != 1 || string(h.gotFiles[0].Data) != "OUR PACKET" {
		t.Fatalf("hub received %+v", h.gotFiles)
	}
	if len(in) != 1 || in[0].Name != "00000001.pkt" || string(in[0].Data) != "HUB PACKET BYTES" {
		t.Fatalf("client received %+v", in)
	}
}

func TestSessionCRAM(t *testing.T) {
	h := startHub(t, "s3cret", true, nil)
	_, err := Session(Config{
		HostPort: h.ln.Addr().String(), OurAddr: "21:1/999",
		Password: "s3cret", Timeout: 10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("Session (CRAM): %v", err)
	}
	<-h.done
	if !strings.HasPrefix(h.gotPwd, "CRAM-MD5-") {
		t.Fatalf("client sent plain password %q despite CRAM offer", h.gotPwd)
	}
}

func TestSessionBadPassword(t *testing.T) {
	h := startHub(t, "right", true, nil)
	_, err := Session(Config{
		HostPort: h.ln.Addr().String(), OurAddr: "21:1/999",
		Password: "wrong", Timeout: 10 * time.Second,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("expected auth failure, got %v", err)
	}
}
