package main

import (
	"errors"
	"strings"
	"time"

	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
	"vendetta-x/server/internal/zmodem"
)

// maxUpload caps a single ZMODEM upload. Generous for scene releases but a
// hard ceiling against a caller filling the SQLite blob store.
const maxUpload = 32 << 20 // 32 MiB

// transferWindow bounds a whole transfer independent of the idle watchdog.
const transferWindow = 20 * time.Minute

// downloadFile sends one file to the caller over ZMODEM. It bumps the file's
// download counter and credits the user's download bytes on success. Returns
// whether the transfer completed.
func (b *board) downloadFile(s *term.Session, user *store.User, f *store.FileEntry) bool {
	if blocked, reason := b.ratioBlocksDownload(user, f.Size); blocked {
		s.Notice(reason)
		return false
	}
	content, err := b.st.FileContent(f.ID)
	if err != nil || content == nil {
		s.Notice("That file's bytes are missing.")
		return false
	}

	s.Printf("\r\n\x1b[0;37m  Sending \x1b[1;37m%s\x1b[0;37m (\x1b[1;37m%s\x1b[0;37m) over ZMODEM -- "+
		"start your receiver.\x1b[0m\r\n", f.Filename, sizeStr(f.Size))
	s.Print("\x1b[1;30m  (SyncTERM/NetRunner auto-start; on a raw client run 'rz')\x1b[0m\r\n")
	s.Flush()

	rw, done := s.Transfer()
	defer done()
	clear := s.TransferDeadline(transferWindow)
	defer clear()

	err = zmodem.Send(rw, f.Filename, f.Uploaded, content)
	if err != nil {
		if errors.Is(err, zmodem.ErrSkipped) {
			s.Notice("Transfer skipped.")
		} else {
			s.Notice("Transfer failed or was cancelled.")
		}
		return false
	}

	b.st.IncDownload(f.ID)
	if err := b.st.AddDownloadBytes(user.ID, f.Size); err == nil {
		user.DlBytes += f.Size
		user.Downloads++
	}
	s.Print("\r\n\x1b[1;32m  Transfer complete.\x1b[0m\r\n")
	return true
}

// uploadFile receives one file from the caller over ZMODEM into the given
// area, prompting for a description, then credits the user's upload bytes.
func (b *board) uploadFile(s *term.Session, user *store.User, area *store.FileArea) {
	s.Print("\r\n\x1b[0;37m  Description \x1b[1;30m(shown in the listing)\x1b[0;37m: \x1b[1;37m")
	s.Flush()
	desc := strings.TrimSpace(s.ReadLine(60))
	if desc == "" {
		desc = "(no description)"
	}

	s.Print("\r\n\x1b[0;37m  Ready. Start your ZMODEM upload now \x1b[1;30m(or run 'sz file')\x1b[0;37m.\x1b[0m\r\n")
	s.Flush()

	rw, done := s.Transfer()
	defer done()
	clear := s.TransferDeadline(transferWindow)
	defer clear()

	f, err := zmodem.Receive(rw, maxUpload)
	if err != nil {
		var tooBig zmodem.ErrTooLarge
		if errors.As(err, &tooBig) {
			s.Notice("That file is over the size limit.")
		} else {
			s.Notice("Upload failed or was cancelled.")
		}
		return
	}

	name := sanitizeUploadName(f.Name)
	id, err := b.st.AddFile(area.ID, name, desc, user.Handle, f.Data)
	if err != nil {
		s.Notice("Could not store the upload.")
		return
	}
	_ = id
	if err := b.st.AddUploadBytes(user.ID, int64(len(f.Data))); err == nil {
		user.UlBytes += int64(len(f.Data))
		user.Uploads++
	}
	s.Printf("\r\n\x1b[1;32m  Received \x1b[1;37m%s\x1b[1;32m (\x1b[1;37m%s\x1b[1;32m). Thanks for "+
		"the contribution.\x1b[0m\r\n", name, sizeStr(int64(len(f.Data))))
}

// sanitizeUploadName strips path separators and control bytes from a
// caller-supplied filename so it can't escape the listing or the area.
func sanitizeUploadName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "upload.bin"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}
