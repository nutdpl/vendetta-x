package zmodem

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// ErrSkipped is returned by Send when the receiver declines the file (ZSKIP).
var ErrSkipped = errors.New("zmodem: receiver skipped the file")

const maxRetries = 8

const zchallenge = 14

// Send transmits one named file to a ZMODEM receiver (SyncTERM, NetRunner,
// lrzsz rz) over rw, which must be 8-bit clean in both directions. It blocks
// until the session completes; the caller is responsible for any overall
// deadline on the underlying transport.
func Send(rw io.ReadWriter, name string, mtime time.Time, data []byte) error {
	c := newConn(rw)

	// Announce: the classic "rz\r" nudge (starts a receiver on a raw shell,
	// harmless noise to an auto-starting terminal) then ZRQINIT.
	if _, err := c.w.Write([]byte("rz\r")); err != nil {
		return err
	}
	if err := c.writeHexHeader(header{typ: zrqinit}); err != nil {
		return err
	}

	// Wait for the receiver's ZRINIT and capture its capabilities.
	var use32 bool
	for retries := 0; ; retries++ {
		if retries > maxRetries {
			return fmt.Errorf("zmodem: no ZRINIT from receiver")
		}
		h, err := c.readHeader()
		if err != nil {
			return err
		}
		switch h.typ {
		case zrinit:
			use32 = h.b[3]&canfc32 != 0
			c.escapeAll = h.b[3]&escctl != 0
		case znak, zrqinit:
			if err := c.writeHexHeader(header{typ: zrqinit}); err != nil {
				return err
			}
			continue
		case zchallenge:
			if err := c.writeHexHeader(header{typ: zack, b: h.b}); err != nil {
				return err
			}
			continue
		case zfin:
			return fmt.Errorf("zmodem: receiver finished before transfer")
		default:
			continue
		}
		break
	}

	// ZFILE: binary header (ZF0 = binary conversion) + the file info
	// subpacket ("name NUL size mtime-octal"), ZCRCW terminated.
	info := append([]byte(name), 0)
	info = append(info, []byte(fmt.Sprintf("%d %o", len(data), mtime.Unix()))...)
	info = append(info, 0)
	fileHdr := header{typ: zfile, b: [4]byte{0, 0, 0, 1}} // ZF0=ZCBIN

	sendFile := func() error {
		if err := c.writeBinHeader(fileHdr, use32); err != nil {
			return err
		}
		return c.writeSubpacket(info, zcrcw, use32)
	}
	if err := sendFile(); err != nil {
		return err
	}

	// Wait for the receiver's ZRPOS (start position).
	var pos int64
	for retries := 0; ; retries++ {
		if retries > maxRetries {
			return fmt.Errorf("zmodem: no ZRPOS after ZFILE")
		}
		h, err := c.readHeader()
		if err != nil {
			return err
		}
		switch h.typ {
		case zrpos:
			pos = h.pos()
		case zskip:
			c.writeHexHeader(header{typ: zfin})
			return ErrSkipped
		case zrinit, znak:
			// stale ZRINIT or a garbled ZFILE: offer the file again
			if err := sendFile(); err != nil {
				return err
			}
			continue
		case zferr, zabort:
			return fmt.Errorf("zmodem: receiver aborted")
		default:
			continue
		}
		break
	}

	// Data: ZDATA from pos, streaming ZCRCG subpackets, final ZCRCE, then
	// ZEOF. The receiver may answer ZRPOS to rewind (CRC error on its side).
	for attempt := 0; ; attempt++ {
		if attempt > maxRetries {
			return fmt.Errorf("zmodem: too many rewinds")
		}
		if pos < 0 || pos > int64(len(data)) {
			return fmt.Errorf("zmodem: receiver requested bad position %d", pos)
		}
		if err := c.writeBinHeader(posHeader(zdata, pos), use32); err != nil {
			return err
		}
		for off := pos; ; {
			end := off + subpacketSize
			if end >= int64(len(data)) {
				if err := c.writeSubpacket(data[off:], zcrce, use32); err != nil {
					return err
				}
				break
			}
			if err := c.writeSubpacket(data[off:end], zcrcg, use32); err != nil {
				return err
			}
			off = end
		}
		if err := c.writeHexHeader(posHeader(zeof, int64(len(data)))); err != nil {
			return err
		}

		// After ZEOF: ZRINIT = file landed; ZRPOS = rewind and resend.
	eofWait:
		for retries := 0; ; retries++ {
			if retries > maxRetries {
				return fmt.Errorf("zmodem: no response to ZEOF")
			}
			h, err := c.readHeader()
			if err != nil {
				return err
			}
			switch h.typ {
			case zrinit:
				// done: close the session
				if err := c.writeHexHeader(header{typ: zfin}); err != nil {
					return err
				}
				for i := 0; i <= maxRetries; i++ {
					fh, err := c.readHeader()
					if err != nil {
						return err
					}
					if fh.typ == zfin {
						_, err = c.w.Write([]byte("OO"))
						return err
					}
				}
				return fmt.Errorf("zmodem: no ZFIN handshake")
			case zrpos:
				pos = h.pos()
				break eofWait
			case znak:
				if err := c.writeHexHeader(posHeader(zeof, int64(len(data)))); err != nil {
					return err
				}
			case zack:
				continue
			case zskip:
				return ErrSkipped
			case zferr, zabort:
				return fmt.Errorf("zmodem: receiver aborted")
			}
		}
	}
}
