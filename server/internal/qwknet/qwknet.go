// Package qwknet federates the board into a QWK message network. The board
// plays the caller role against a hub BBS: on each exchange it uploads a .REP
// packet of locally-posted messages from networked boards and downloads the
// hub's .QWK packet of new network messages, importing them with the original
// author and date preserved.
//
// Transport is FTP -- what real QWK hubs (Synchronet boards carrying DOVE-Net
// and friends) serve for unattended packet exchange. All configuration lives
// in the shared settings table under qwknet.* keys, edited from the sysop
// panel; the exchange itself is normally fired by the event scheduler.
//
// Loop and duplicate protection:
//   - only messages with a blank Origin (posted locally) are ever exported,
//     so imports can't echo back out;
//   - export uses a per-board high-water mark (last exported message id),
//     advanced only after the hub accepts the upload;
//   - import skips any message whose (board, author, subject, date) already
//     exists, which also drops our own posts when the hub echoes them back.
package qwknet

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"vendetta-x/server/internal/ftp"
	"vendetta-x/server/internal/qwk"
	"vendetta-x/server/internal/store"
)

// Settings keys (values in the shared settings table).
const (
	KeyEnabled = "qwknet.enabled" // "1" to allow exchanges
	KeyHost    = "qwknet.host"    // hub host or host:port (port 21 default)
	KeyUser    = "qwknet.user"    // hub FTP account
	KeyPass    = "qwknet.pass"    // hub FTP password
	KeyHubID   = "qwknet.hubid"   // hub's BBSID (names the .REP/.QWK files)
	KeyNetName = "qwknet.netname" // network tag stored as imported messages' origin
	KeyConfMap = "qwknet.confmap" // conference<->board mapping, one "conf = boardtag" per line

	keyLastStatus = "qwknet.last_status"
	keyMarkPrefix = "qwknet.export.last_id." // + boardID -> high-water mark
)

// Summary reports what one exchange did.
type Summary struct {
	Exported int // messages uploaded to the hub
	Imported int // new network messages posted locally
	Skipped  int // downloaded messages dropped (dupes / unmapped conferences)
}

func (s Summary) String() string {
	return fmt.Sprintf("exported %d, imported %d, skipped %d", s.Exported, s.Imported, s.Skipped)
}

// exchangeMu serializes exchanges: the scheduler and the sysop panel's
// "run now" button share one process, and two concurrent exchanges would
// race the high-water marks and the hub's packet files.
var exchangeMu sync.Mutex

// Enabled reports whether QWK networking is switched on.
func Enabled(st *store.Store) bool { return st.SettingBool(KeyEnabled, false) }

// LastStatus returns the recorded outcome of the most recent exchange.
func LastStatus(st *store.Store) string { return st.Setting(keyLastStatus, "") }

// Exchange runs one full exchange with the configured hub and records the
// outcome in settings (visible on the sysop panel). It returns an error when
// networking is disabled, misconfigured, or the hub can't be reached.
func Exchange(st *store.Store) (Summary, error) {
	exchangeMu.Lock()
	defer exchangeMu.Unlock()

	sum, err := exchange(st)
	status := time.Now().Format("2006-01-02 15:04") + " -- "
	if err != nil {
		status += "FAILED: " + err.Error()
	} else {
		status += sum.String()
	}
	st.SetSetting(keyLastStatus, status)
	return sum, err
}

func exchange(st *store.Store) (Summary, error) {
	var sum Summary
	if !Enabled(st) {
		return sum, errors.New("qwknet: networking is disabled")
	}
	host := strings.TrimSpace(st.Setting(KeyHost, ""))
	hubID := strings.ToUpper(strings.TrimSpace(st.Setting(KeyHubID, "")))
	if host == "" || hubID == "" {
		return sum, errors.New("qwknet: hub host and BBSID must be configured")
	}
	if !strings.Contains(host, ":") {
		host += ":21"
	}
	netName := strings.TrimSpace(st.Setting(KeyNetName, ""))
	if netName == "" {
		netName = hubID
	}

	confToBoard, err := ConfMap(st)
	if err != nil {
		return sum, err
	}
	if len(confToBoard) == 0 {
		return sum, errors.New("qwknet: no conferences are mapped to boards")
	}

	// Gather the outgoing feed before dialing, so a config problem never
	// leaves a half-done FTP session behind.
	type export struct {
		boardID int64
		lastID  int64
		msgs    []qwk.Message
	}
	var (
		outgoing []qwk.Message
		marks    []export
	)
	for conf, boardID := range confToBoard {
		mark := int64(st.SettingInt(markKey(boardID), 0))
		local, err := st.LocalMessagesAfter(boardID, mark)
		if err != nil {
			return sum, err
		}
		last := mark
		for _, m := range local {
			outgoing = append(outgoing, qwk.Message{
				Conference: conf,
				To:         m.To,
				From:       m.From,
				Subject:    m.Subject,
				Body:       m.Body,
				Date:       m.Posted,
			})
			if m.ID > last {
				last = m.ID
			}
		}
		if last > mark {
			marks = append(marks, export{boardID: boardID, lastID: last})
		}
	}

	c, err := ftp.Dial(host, 60*time.Second)
	if err != nil {
		return sum, err
	}
	defer c.Quit()
	if err := c.Login(st.Setting(KeyUser, ""), st.Setting(KeyPass, "")); err != nil {
		return sum, err
	}

	// Upload our replies first (hubs fold them into the next packet build).
	if len(outgoing) > 0 {
		rep, err := qwk.BuildReply(hubID, outgoing)
		if err != nil {
			return sum, err
		}
		if err := c.Stor(hubID+".REP", rep); err != nil {
			return sum, err
		}
		sum.Exported = len(outgoing)
		// The hub has the messages; advance the high-water marks now so a
		// later download failure can't cause a duplicate re-upload.
		for _, e := range marks {
			st.SetSetting(markKey(e.boardID), strconv.FormatInt(e.lastID, 10))
		}
	}

	// Fetch the hub's packet. None waiting is a normal, successful outcome.
	packet, err := c.Retr(hubID + ".QWK")
	if err != nil {
		return sum, err
	}
	if packet == nil {
		return sum, nil
	}

	parsed, err := qwk.Parse(packet)
	if err != nil {
		return sum, err
	}
	for _, m := range parsed.Messages {
		boardID, ok := confToBoard[m.Conference]
		if !ok {
			sum.Skipped++
			continue
		}
		posted := m.Date
		if posted.IsZero() {
			posted = time.Now()
		}
		if m.From == "" || m.Body == "" {
			sum.Skipped++
			continue
		}
		if dup, err := st.HasMessage(boardID, m.From, m.Subject, posted); err != nil {
			return sum, err
		} else if dup {
			sum.Skipped++
			continue
		}
		to := m.To
		if to == "" {
			to = "All"
		}
		if _, err := st.PostMessage(&store.Message{
			BoardID: boardID,
			From:    m.From,
			To:      to,
			Subject: m.Subject,
			Body:    m.Body,
			Posted:  posted,
			Origin:  netName,
		}); err != nil {
			return sum, err
		}
		sum.Imported++
	}

	// The packet is safely imported; clear it from the hub so the next
	// exchange doesn't re-fetch it (hubs that auto-delete return 550, fine).
	if err := c.Delete(hubID + ".QWK"); err != nil {
		return sum, err
	}
	return sum, nil
}

// ConfMap parses the qwknet.confmap setting into hub-conference -> board-id.
// The format is one mapping per line: "<conference number> = <board tag>",
// with '#' comments and blank lines ignored. Unknown board tags are an error
// (a silent skip would quietly drop a whole conference's traffic).
func ConfMap(st *store.Store) (map[uint16]int64, error) {
	raw := st.Setting(KeyConfMap, "")
	boards, err := st.Boards()
	if err != nil {
		return nil, err
	}
	byTag := make(map[string]int64, len(boards))
	for _, b := range boards {
		byTag[strings.ToLower(b.Tag)] = b.ID
	}

	out := map[uint16]int64{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		confStr, tag, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("qwknet: bad confmap line %q (want \"conf = boardtag\")", line)
		}
		conf, err := strconv.Atoi(strings.TrimSpace(confStr))
		if err != nil || conf < 0 || conf > 65535 {
			return nil, fmt.Errorf("qwknet: bad conference number in %q", line)
		}
		tag = strings.ToLower(strings.TrimSpace(tag))
		boardID, found := byTag[tag]
		if !found {
			return nil, fmt.Errorf("qwknet: confmap names unknown board tag %q", tag)
		}
		out[uint16(conf)] = boardID
	}
	return out, nil
}

func markKey(boardID int64) string {
	return keyMarkPrefix + strconv.FormatInt(boardID, 10)
}
