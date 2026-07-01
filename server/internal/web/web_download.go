package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vendetta-x/server/internal/store"
)

// downloadTTL is how long a signed download link stays valid. Links are
// short-lived capabilities; the per-process secret also invalidates them on
// restart.
const downloadTTL = 10 * time.Minute

// newDownloadSecret returns a random HMAC key. Generated once per process, so
// links naturally expire across restarts on top of their TTL.
func newDownloadSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to a time-seeded key so signing still
		// works (links remain unforgeable within this process).
		return []byte(time.Now().String() + "vendetta-download-secret")
	}
	return b
}

// signDownload builds a signed, time-limited path for downloading a file:
//
//	/dl/<base64url(payload)>.<base64url(hmac)>   payload = "<fileID>:<userID>:<expiryUnix>"
//
// The signature binds the file id, the caller it was issued to (for ratio
// accounting; 0 = anonymous), and the expiry, so a link can't be altered to
// point at another file, be re-attributed, or outlive its window.
func (s *server) signDownload(fileID, userID int64, ttl time.Duration) string {
	payload := strconv.FormatInt(fileID, 10) + ":" + strconv.FormatInt(userID, 10) +
		":" + strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
	mac := hmac.New(sha256.New, s.dlSecret)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)
	tok := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
	return "/dl/" + tok
}

// verifyDownload validates a token and returns the file id and issuing user id
// it authorizes (userID 0 = anonymous).
func (s *server) verifyDownload(token string) (fileID, userID int64, ok bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	// Strict decoding rejects non-canonical encodings (e.g. a flipped final
	// character whose unused trailing bits are non-zero), so even single-char
	// tampering is caught before the HMAC check.
	enc := base64.RawURLEncoding.Strict()
	payload, err := enc.DecodeString(parts[0])
	if err != nil {
		return 0, 0, false
	}
	sig, err := enc.DecodeString(parts[1])
	if err != nil {
		return 0, 0, false
	}
	mac := hmac.New(sha256.New, s.dlSecret)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return 0, 0, false // forged or tampered
	}
	fields := strings.SplitN(string(payload), ":", 3)
	if len(fields) != 3 {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || id < 0 {
		return 0, 0, false
	}
	uid, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || uid < 0 {
		return 0, 0, false
	}
	exp, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if time.Now().Unix() > exp {
		return id, uid, false // expired (ids returned only for messaging; treated invalid)
	}
	return id, uid, true
}

// download serves a file's stored bytes for a valid, unexpired signed token.
func (s *server) download(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := s.verifyDownload(r.PathValue("token"))
	if !ok {
		http.Error(w, "link expired or invalid", http.StatusForbidden)
		return
	}
	f, err := s.st.FileByID(id)
	if err != nil {
		log.Printf("web: download FileByID: %v", err)
		http.Error(w, "not available", http.StatusInternalServerError)
		return
	}
	if f == nil {
		http.NotFound(w, r)
		return
	}
	// Ratio gate: if enforcement is on, a logged-in caller must have credit.
	// Anonymous links (userID 0) predate ratio and are left ungated -- they
	// only exist when the sysop hasn't required login to browse.
	var dlUser *store.User
	if userID > 0 {
		if u, uerr := s.st.UserByID(userID); uerr == nil && u != nil {
			dlUser = u
			if s.st.RatioBlocks(u, f.Size) {
				http.Error(w, "ratio: not enough download credit -- upload something first",
					http.StatusForbidden)
				return
			}
		}
	}
	content, err := s.st.FileContent(id)
	if err != nil {
		log.Printf("web: download FileContent: %v", err)
		http.Error(w, "not available", http.StatusInternalServerError)
		return
	}
	if content == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+safeFilename(f.Filename)+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(content); err != nil {
		return // client hung up
	}
	if dlUser != nil {
		if err := s.st.AddDownloadBytes(dlUser.ID, f.Size); err != nil {
			log.Printf("web: AddDownloadBytes: %v", err)
		}
	} else if err := s.st.IncDownload(id); err != nil {
		// AddDownloadBytes already bumps the counter; only the anonymous path
		// needs a bare IncDownload.
		log.Printf("web: IncDownload: %v", err)
	}
}

// safeFilename strips characters that would break a Content-Disposition header
// (quotes, control bytes, path separators), keeping downloads safe.
func safeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '\\' || r == '/' {
			return '_'
		}
		return r
	}, name)
	if name == "" {
		return "download"
	}
	return name
}
