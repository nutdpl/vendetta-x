package door

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Caller carries the per-call data a drop file needs to describe the user and
// session to the door.
type Caller struct {
	Node        int
	Handle      string
	RealName    string
	SL          int
	MinutesLeft int
	Emulation   int // 1 = ANSI
	Baud        int // use a sane default like 38400
}

// System carries the board-wide identity a drop file advertises -- the system
// name and the sysop's name -- so they come from the running board's settings
// instead of being baked into the door layer.
type System struct {
	Name  string // board name, e.g. "Vendetta/X"
	Sysop string // sysop's name, e.g. "nut"
}

// defaultBaud is used when a Caller leaves Baud unset.
const defaultBaud = 38400

// orDefault returns s trimmed, or def when s is blank.
func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// WriteDropFile writes the drop file for this door (DOOR.SYS or DORINFO1.DEF,
// per d.DropType) into d.WorkDir and returns the path. sys supplies the board's
// identity. The format is chosen to be standard enough that real doors parse it.
func (d Door) WriteDropFile(c Caller, sys System) (string, error) {
	dir := strings.TrimSpace(d.WorkDir)
	if dir == "" {
		dir = "."
	}
	baud := c.Baud
	if baud <= 0 {
		baud = defaultBaud
	}
	ansi := 0
	if c.Emulation == 1 {
		ansi = 1
	}

	var (
		name    string
		content string
	)
	if strings.EqualFold(strings.TrimSpace(d.DropType), "DORINFO1.DEF") {
		name = "DORINFO1.DEF"
		content = dorinfo1(c, baud, ansi, sys)
	} else {
		name = "DOOR.SYS"
		content = doorSys(c, baud, ansi, strings.TrimSpace(d.DOSPath), sys)
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// firstLast splits a real name into a first and last word; if there's no last
// name the last field is empty.
func firstLast(name string) (string, string) {
	fields := strings.Fields(name)
	switch len(fields) {
	case 0:
		return "", ""
	case 1:
		return fields[0], ""
	default:
		return fields[0], strings.Join(fields[1:], " ")
	}
}

// doorSys renders the classic ~52-line Synchronet-style DOOR.SYS. Line 1 is the
// COM port (COM0: for a local/telnet caller). dosPath is the DOS-side path the
// door reads (fields 33/34); empty leaves those blank rather than guessing. sys
// supplies the sysop name. Lines use CRLF.
func doorSys(c Caller, baud, ansi int, dosPath string, sys System) string {
	real := c.RealName
	if strings.TrimSpace(real) == "" {
		real = c.Handle
	}
	sysop := orDefault(sys.Sysop, "SysOp")
	ansiYN := "N"
	if ansi == 1 {
		ansiYN = "Y"
	}
	node := c.Node
	if node <= 0 {
		node = 1
	}
	mins := c.MinutesLeft
	if mins < 0 {
		mins = 0
	}
	secs := mins * 60

	lines := []string{
		"COM0:",                   // 1  comm port (0 = local/telnet)
		fmt.Sprintf("%d", baud),   // 2  baud rate
		"8",                       // 3  data bits
		fmt.Sprintf("%d", node),   // 4  node number
		fmt.Sprintf("%d", baud),   // 5  locked DTE rate
		"Y",                       // 6  screen display (Y)
		"Y",                       // 7  printer toggle
		"Y",                       // 8  page bell
		"Y",                       // 9  caller alarm
		c.Handle,                  // 10 user full name / handle
		"",                        // 11 calling from (city, state)
		"",                        // 12 home phone
		"",                        // 13 work/data phone
		"PASSWORD",                // 14 password (not exposed)
		fmt.Sprintf("%d", c.SL),   // 15 security level
		"1",                       // 16 total times on
		"01/01/01",                // 17 last date called
		fmt.Sprintf("%d", secs),   // 18 seconds remaining this call
		fmt.Sprintf("%d", mins),   // 19 minutes remaining this call
		"GR",                      // 20 graphics mode (GR=ANSI,RIP,NG)
		"24",                      // 21 page length
		"Y",                       // 22 user mode (expert)
		"",                        // 23 conferences registered in
		"",                        // 24 conference exited to door from
		"12/31/99",                // 25 user expiration date
		fmt.Sprintf("%d", c.Node), // 26 user record position
		"",                        // 27 default protocol
		"0",                       // 28 total uploads
		"0",                       // 29 total downloads
		"0",                       // 30 daily download K total
		"0",                       // 31 daily download max K
		"01/01/01",                // 32 user birthday
		dosPath,                   // 33 path to main directory files (sysop-set)
		dosPath,                   // 34 path to gen/menu files (sysop-set)
		sysop,                     // 35 sysop name (from board settings)
		c.Handle,                  // 36 alias / handle
		"00:00",                   // 37 event time (HH:MM)
		"Y",                       // 38 error-correcting connection
		ansiYN,                    // 39 ANSI in NG mode (Y/N)
		"Y",                       // 40 use record locking
		"7",                       // 41 BBS default text colour
		"0",                       // 42 time credits in minutes
		"01/01/01",                // 43 last new-files scan date
		"00:00",                   // 44 time of this call
		"00:00",                   // 45 time of last call
		"32767",                   // 46 max daily files available
		"0",                       // 47 files downloaded today
		"0",                       // 48 total K bytes uploaded
		"0",                       // 49 total K bytes downloaded
		"User Comment",            // 50 user comment
		"0",                       // 51 total doors opened
		"0",                       // 52 total messages posted
	}
	// Field 50 (user comment) carries the caller's real name for the door's logs.
	lines[49] = real

	return strings.Join(lines, "\r\n") + "\r\n"
}

// dorinfo1 renders the Fido-style DORINFO1.DEF (the simpler 11+ line format).
// sys supplies the system and sysop names.
func dorinfo1(c Caller, baud, ansi int, sys System) string {
	first, last := firstLast(c.RealName)
	if first == "" {
		first = c.Handle
	}
	sysopFirst, sysopLast := firstLast(orDefault(sys.Sysop, "SysOp"))
	if sysopLast == "" {
		sysopLast = "SysOp" // DORINFO1.DEF expects both name fields
	}
	mins := c.MinutesLeft
	if mins < 0 {
		mins = 0
	}
	lines := []string{
		orDefault(sys.Name, "BBS"),         // 1  system name (from settings)
		sysopFirst,                         // 2  sysop first name
		sysopLast,                          // 3  sysop last name
		"COM0",                             // 4  comm port
		fmt.Sprintf("%d BAUD,N,8,1", baud), // 5  baud / settings
		"0",                                // 6  reserved (network type)
		first,                              // 7  user first name
		last,                               // 8  user last name
		c.Handle,                           // 9  user location (use handle as alias/location)
		fmt.Sprintf("%d", ansi),            // 10 ANSI (0/1)
		fmt.Sprintf("%d", c.SL),            // 11 user security level
		fmt.Sprintf("%d", mins),            // 12 time left in minutes
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}
