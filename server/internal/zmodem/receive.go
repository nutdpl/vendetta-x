package zmodem

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// File is one received upload.
type File struct {
	Name string
	Data []byte
}

// ErrTooLarge is returned when the sender's file exceeds the receive cap.
type ErrTooLarge struct{ Name string }

func (e ErrTooLarge) Error() string {
	return fmt.Sprintf("zmodem: upload %q exceeds the size cap", e.Name)
}

// Receive accepts a single file from a ZMODEM sender (SyncTERM, lrzsz sz)
// over rw, which must be 8-bit clean in both directions. Files beyond the
// first are politely skipped; a file larger than max aborts with ErrTooLarge.
// The caller is responsible for any overall deadline on the transport.
func Receive(rw io.ReadWriter, max int64) (*File, error) {
	c := newConn(rw)
	myFlags := header{typ: zrinit, b: [4]byte{0, 0, 0, canfdx | canovio | canfc32}}

	if err := c.writeHexHeader(myFlags); err != nil {
		return nil, err
	}

	var got *File
	var tooLarge *ErrTooLarge
	for retries := 0; ; retries++ {
		if retries > maxRetries*4 {
			return nil, fmt.Errorf("zmodem: session did not complete")
		}
		h, err := c.readHeader()
		if err != nil {
			return nil, err
		}
		switch h.typ {
		case zrqinit:
			if err := c.writeHexHeader(myFlags); err != nil {
				return nil, err
			}

		case zsinit:
			// optional attn-string subpacket; consume and ACK
			if _, _, err := c.readSubpacket(true, 1024); err != nil {
				// try 16-bit CRC framing as a fallback
				return nil, err
			}
			if err := c.writeHexHeader(header{typ: zack}); err != nil {
				return nil, err
			}

		case zfile:
			use32 := c.lastFormatWas32
			info, term, err := c.readSubpacket(use32, 2048)
			if err != nil {
				return nil, err
			}
			if term != zcrcw {
				return nil, fmt.Errorf("zmodem: ZFILE info not ZCRCW-terminated")
			}
			name, declared := parseFileInfo(info)
			if got != nil || tooLarge != nil {
				// one file per session on this board
				if err := c.writeHexHeader(header{typ: zskip}); err != nil {
					return nil, err
				}
				continue
			}
			if declared > 0 && declared > max {
				tooLarge = &ErrTooLarge{Name: name}
				if err := c.writeHexHeader(header{typ: zskip}); err != nil {
					return nil, err
				}
				continue
			}
			data, err := c.receiveData(max, use32)
			if err != nil {
				if _, ok := err.(ErrTooLarge); ok {
					tooLarge = &ErrTooLarge{Name: name}
					// hard-stop the sender; mid-stream there is no polite skip
					c.w.Write([]byte{zdle, zdle, zdle, zdle, zdle})
					return nil, *tooLarge
				}
				return nil, err
			}
			got = &File{Name: name, Data: data}
			if err := c.writeHexHeader(myFlags); err != nil {
				return nil, err
			}

		case zfin:
			if err := c.writeHexHeader(header{typ: zfin}); err != nil {
				return nil, err
			}
			// best-effort: consume the sender's "OO" sign-off
			var oo [2]byte
			io.ReadFull(c.r, oo[:])
			if tooLarge != nil {
				return nil, *tooLarge
			}
			if got == nil {
				return nil, fmt.Errorf("zmodem: sender finished without a file")
			}
			return got, nil

		case zabort, zferr:
			return nil, fmt.Errorf("zmodem: sender aborted")
		}
	}
}

// receiveData collects the ZDATA stream for the current file until ZEOF.
func (c *conn) receiveData(max int64, use32 bool) ([]byte, error) {
	if err := c.writeHexHeader(posHeader(zrpos, 0)); err != nil {
		return nil, err
	}
	var data []byte
	for retries := 0; ; retries++ {
		if retries > maxRetries*4 {
			return nil, fmt.Errorf("zmodem: data stream did not complete")
		}
		h, err := c.readHeader()
		if err != nil {
			return nil, err
		}
		use32 = c.lastFormatWas32
		switch h.typ {
		case zdata:
			if h.pos() != int64(len(data)) {
				// out of sync: ask for our position again
				if err := c.writeHexHeader(posHeader(zrpos, int64(len(data)))); err != nil {
					return nil, err
				}
				continue
			}
			for {
				chunk, term, err := c.readSubpacket(use32, subpacketSize*2)
				if err != nil {
					// CRC/framing error: rewind to our position
					if err := c.writeHexHeader(posHeader(zrpos, int64(len(data)))); err != nil {
						return nil, err
					}
					break
				}
				data = append(data, chunk...)
				if int64(len(data)) > max {
					return nil, ErrTooLarge{}
				}
				if term == zcrcq || term == zcrcw {
					if err := c.writeHexHeader(posHeader(zack, int64(len(data)))); err != nil {
						return nil, err
					}
				}
				if term == zcrce || term == zcrcw {
					break // header follows (ZEOF or another ZDATA)
				}
			}

		case zeof:
			if h.pos() == int64(len(data)) {
				return data, nil
			}
			// short/long: ask to resume from where we are
			if err := c.writeHexHeader(posHeader(zrpos, int64(len(data)))); err != nil {
				return nil, err
			}

		case zfin:
			return nil, fmt.Errorf("zmodem: sender finished mid-file")
		}
	}
}

// parseFileInfo splits a ZFILE info subpacket: "name NUL size mtime ...".
// The name is reduced to its base (no directory components).
func parseFileInfo(info []byte) (name string, size int64) {
	nul := bytes.IndexByte(info, 0)
	if nul < 0 {
		return "upload.bin", 0
	}
	name = path.Base(strings.ReplaceAll(string(info[:nul]), "\\", "/"))
	if name == "" || name == "." || name == "/" {
		name = "upload.bin"
	}
	rest := string(info[nul+1:])
	if end := strings.IndexByte(rest, 0); end >= 0 {
		rest = rest[:end]
	}
	fields := strings.Fields(rest)
	if len(fields) > 0 {
		size, _ = strconv.ParseInt(fields[0], 10, 64)
	}
	return name, size
}
