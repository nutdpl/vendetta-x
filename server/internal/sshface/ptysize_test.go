package sshface

import (
	"encoding/binary"
	"testing"
)

func ptyReqPayload(term string, cols, rows uint32) []byte {
	p := make([]byte, 0, 4+len(term)+16)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(term)))
	p = append(p, n[:]...)
	p = append(p, term...)
	var dim [4]byte
	binary.BigEndian.PutUint32(dim[:], cols)
	p = append(p, dim[:]...)
	binary.BigEndian.PutUint32(dim[:], rows)
	p = append(p, dim[:]...)
	binary.BigEndian.PutUint32(dim[:], 0) // width px
	p = append(p, dim[:]...)
	binary.BigEndian.PutUint32(dim[:], 0) // height px
	p = append(p, dim[:]...)
	return p
}

func TestParsePtySize(t *testing.T) {
	got1, got2 := parsePtySize(ptyReqPayload("xterm-256color", 120, 40))
	if got1 != 120 || got2 != 40 {
		t.Fatalf("parsePtySize = %dx%d, want 120x40", got1, got2)
	}
	// A short payload yields zero (which the session floors to 80x24).
	if c, r := parsePtySize([]byte{0, 0, 0}); c != 0 || r != 0 {
		t.Fatalf("short payload = %dx%d, want 0x0", c, r)
	}
	// TERM parsing still works alongside the size.
	if term := parsePtyTerm(ptyReqPayload("vt320", 80, 25)); term != "vt320" {
		t.Fatalf("parsePtyTerm = %q, want vt320", term)
	}
}
