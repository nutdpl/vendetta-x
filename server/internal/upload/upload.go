// Package upload is the shared intake logic both faces run on a new file:
// pull the release's own description out of the archive (FILE_ID.DIZ, the
// scene standard every upload processor honored) so listings describe the
// file even when the uploader couldn't be bothered.
package upload

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"

	"vendetta-x/server/internal/sanitize"
)

// Hash returns the content fingerprint (hex SHA-256) used for the
// duplicate-upload check -- the same value store.addFile records.
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// dizMaxLines/dizMaxCols bound what we take from a FILE_ID.DIZ -- the format
// spec is 10 lines x 45 columns, and anything wilder gets clipped rather
// than trusted.
const (
	dizMaxLines = 10
	dizMaxCols  = 45
	dizMaxBytes = 4096
)

// Describe returns the description for an upload: the archive's FILE_ID.DIZ
// flattened to one listing line when content is a ZIP that carries one, else
// fallback. The result is sanitized and never empty.
func Describe(content []byte, fallback string) string {
	if diz := fileIDDiz(content); diz != "" {
		return diz
	}
	fallback = strings.TrimSpace(sanitize.Line(fallback))
	if fallback == "" {
		fallback = "(no description)"
	}
	return fallback
}

// fileIDDiz extracts and flattens FILE_ID.DIZ from a ZIP payload ("" when
// content isn't a ZIP or has none). Case-insensitive, root or nested, size
// capped, control bytes stripped.
func fileIDDiz(content []byte) string {
	if !bytes.HasPrefix(content, []byte("PK\x03\x04")) {
		return ""
	}
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return ""
	}
	for _, zf := range zr.File {
		name := zf.Name
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		if !strings.EqualFold(name, "FILE_ID.DIZ") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return ""
		}
		raw, err := io.ReadAll(io.LimitReader(rc, dizMaxBytes))
		rc.Close()
		if err != nil {
			return ""
		}
		return flattenDiz(string(raw))
	}
	return ""
}

// flattenDiz turns DIZ text into a single listing-safe line: per-line trim
// and width clip, blank lines dropped, joined with a space, line count
// capped. DIZ art (block glyphs) survives -- it's CP437's native tongue.
func flattenDiz(raw string) string {
	var parts []string
	for _, ln := range strings.Split(sanitize.Text(raw), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if len(ln) > dizMaxCols {
			ln = ln[:dizMaxCols]
		}
		parts = append(parts, ln)
		if len(parts) == dizMaxLines {
			break
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
