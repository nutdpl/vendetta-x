package main

import (
	"net"
	"strings"
	"testing"
	"time"

	"vendetta-x/server/internal/bbslist"
	"vendetta-x/server/internal/chat"
	"vendetta-x/server/internal/door"
	"vendetta-x/server/internal/gfiles"
	"vendetta-x/server/internal/mail"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
	"vendetta-x/server/internal/throttle"
	"vendetta-x/server/internal/voting"
)

// ---- test harness ----------------------------------------------------------

// newTestBoard builds a fully-wired *board over an in-memory SQLite store,
// mirroring the feature stores main() constructs. The art directory points at
// the repo-root art/ (sibling of server/), so RenderScreen finds the real .pp
// files when the test runs from the server/ directory.
func newTestBoard(t *testing.T) *board {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(); err != nil {
		t.Fatalf("store.Seed: %v", err)
	}
	mailStore, err := mail.New(st.DB())
	if err != nil {
		t.Fatalf("mail.New: %v", err)
	}
	votingStore, err := voting.New(st.DB())
	if err != nil {
		t.Fatalf("voting.New: %v", err)
	}
	bbsStore, err := bbslist.New(st.DB())
	if err != nil {
		t.Fatalf("bbslist.New: %v", err)
	}
	gfileStore, err := gfiles.New(st.DB())
	if err != nil {
		t.Fatalf("gfiles.New: %v", err)
	}
	doorStore, err := door.New(st.DB())
	if err != nil {
		t.Fatalf("door.New: %v", err)
	}
	return &board{
		st:            st,
		pres:          newPresence(),
		art:           "../art",
		hub:           chat.NewHub(),
		mail:          mailStore,
		voting:        votingStore,
		bbslist:       bbsStore,
		gfiles:        gfileStore,
		doorStore:     doorStore,
		idle:          0,
		loginThrottle: throttle.New(8, 10*time.Minute),
	}
}

// session drives a full board session over an in-memory net.Pipe(). The board
// runs in a goroutine reading/writing the server end; the test reads/writes the
// client end. All reads are deadline-guarded so nothing can hang.
type session struct {
	t      *testing.T
	client net.Conn
	out    []byte // accumulated output seen on the client end
	done   chan struct{}
}

func newSession(t *testing.T, b *board) *session {
	t.Helper()
	serverEnd, clientEnd := net.Pipe()
	se := &session{t: t, client: clientEnd, done: make(chan struct{})}
	go func() {
		defer close(se.done)
		s := term.NewRW(serverEnd, "test:0")
		b.RunBoard(s)
		serverEnd.Close()
	}()
	return se
}

// enter dismisses the connect splash -- the modem handshake and the flagship
// loginscreen that now precede the login matrix -- the way a caller taps a key
// to get past it. Tests call this right after newSession to reach the matrix.
func (se *session) enter() {
	se.t.Helper()
	se.expect("enter the board") // the loginscreen's "press any key" gate
	se.send(" ")
}

// send writes input bytes to the board. Because net.Pipe is unbuffered, the
// board may still be mid-flush (blocked on a write) when we want to send the
// next key -- so we pump reads concurrently while writing, ensuring the board
// can drain its output and reach its next ReadKey to accept our input.
func (se *session) send(in string) {
	se.t.Helper()
	written := make(chan error, 1)
	go func() {
		se.client.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := se.client.Write([]byte(in))
		written <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-written:
			if err != nil {
				se.t.Fatalf("send %q: %v", in, err)
			}
			return
		default:
		}
		if time.Now().After(deadline) {
			se.t.Fatalf("send %q: timed out", in)
		}
		se.pump()
	}
}

// pump reads whatever output is currently available (one short, deadline-bound
// read) and appends it to the accumulated buffer. Returns false on EOF/close.
func (se *session) pump() bool {
	buf := make([]byte, 4096)
	se.client.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	n, err := se.client.Read(buf)
	if n > 0 {
		se.out = append(se.out, buf[:n]...)
	}
	if err != nil {
		// timeout just means "nothing more right now"; any other error
		// (EOF/closed pipe) means the session ended.
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return true
		}
		return false
	}
	return true
}

// expect drains output until substr appears, or fails after a bounded number of
// reads. net.Pipe is unbuffered, so the board blocks on its next write until we
// read; pumping repeatedly lets the board make progress until it next blocks on
// input (which is what we want to observe before sending the next key).
func (se *session) expect(substr string) {
	se.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(string(se.out), substr) {
			return
		}
		if !se.pump() {
			// session ended; one last check.
			if strings.Contains(string(se.out), substr) {
				return
			}
			se.t.Fatalf("expected %q but session ended; output so far:\n%s", substr, se.out)
		}
	}
	se.t.Fatalf("timed out waiting for %q; output so far:\n%s", substr, se.out)
}

// drain reads any remaining output until the session ends, with an overall
// guard. Used after sending the quit sequence.
func (se *session) drain() {
	se.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !se.pump() {
			return
		}
	}
}

// waitDone asserts RunBoard returned within a bounded time after the quit
// sequence was sent.
func (se *session) waitDone() {
	se.t.Helper()
	select {
	case <-se.done:
	case <-time.After(5 * time.Second):
		se.t.Fatal("RunBoard did not return after quit sequence")
	}
}

// ---- full-session tests ----------------------------------------------------

// TestNewUserSignupAndQuit drives the matrix New User flow end to end: it picks
// a unique handle, sets a password (with verify), supplies real name and
// location, then quits from the main menu. It asserts the welcome text appears,
// the session ends with NO CARRIER, and the new user persisted to the store.
func TestNewUserSignupAndQuit(t *testing.T) {
	b := newTestBoard(t)
	se := newSession(t, b)
	se.enter()

	const handle = "tester01"

	// At the matrix, choose New User (hotkey 'n' selects the |{...,N,...} marker).
	se.expect("New User")
	se.send("n")

	// newUser() prompt order: handle, password, verify password, real name,
	// location. Each line is terminated with CR.
	se.expect("Pick a handle:")
	se.send(handle + "\r")
	se.expect("Choose a password:")
	se.send("hunter2pw\r")
	se.expect("Verify password:")
	se.send("hunter2pw\r")
	se.expect("Real name:")
	se.send("Test Person\r")
	se.expect("Location:")
	se.send("Somewhere\r")

	// Account created -> the welcome ceremony plays (credentials / ACCESS
	// GRANTED / entry-granted banner) and ends on a Pause(); one key skips the
	// rest and dismisses the final pause, landing us in the logon sequence.
	se.expect("ACCESS GRANTED")
	se.send(" ")

	// The logon sequence offers a "quick logon" -- answer yes to skip the tour
	// (who's online / sysinfo / bulletins / wall) and land at the main menu.
	se.expect("Quick logon?")
	se.send("y")

	// At the main menu, Goodbye (hotkey 'g').
	se.expect("main menu")
	se.send("g")

	se.drain()
	if !strings.Contains(string(se.out), "NO CARRIER") {
		t.Fatalf("expected NO CARRIER in final output, got:\n%s", se.out)
	}
	se.waitDone()

	// The new user must now exist in the store.
	u, err := b.st.UserByHandle(handle)
	if err != nil {
		t.Fatalf("UserByHandle: %v", err)
	}
	if u == nil {
		t.Fatalf("expected user %q to exist after signup", handle)
	}
	if u.Handle != handle {
		t.Fatalf("handle mismatch: got %q want %q", u.Handle, handle)
	}
}

// TestLoginPasswordlessUserAndUserList logs in as the seeded passwordless user
// "phantom" (login() prompts to set a password the first time), then opens the
// User List and asserts the banner and the handle appear, before quitting.
func TestLoginPasswordlessUserAndUserList(t *testing.T) {
	b := newTestBoard(t)
	se := newSession(t, b)
	se.enter()

	// At the matrix, choose Login (hotkey 'l').
	se.expect("Login")
	se.send("l")

	// login() prompt order for a passwordless account: handle, then -- because
	// no password is on file -- it asks to set one (password + verify), via
	// setPassword(): "Password:" then "Verify:".
	se.expect("Handle:")
	se.send("phantom\r")
	se.expect("set one now")
	se.expect("Password:")
	se.send("ghostpass\r")
	se.expect("Verify:")
	se.send("ghostpass\r")
	se.expect("Welcome, phantom!")

	// The logon sequence offers a "quick logon" -- answer yes to skip the tour
	// and land at the main menu.
	se.expect("Quick logon?")
	se.send("y")

	// Main menu -> User List (hotkey 'u').
	se.expect("main menu")
	se.send("u")
	se.expect("user list") // screenHeader title
	se.expect("phantom")   // the seeded handle appears in the list
	se.send(" ")           // dismiss the list's Pause()

	// Back at the main menu, Goodbye.
	se.expect("main menu")
	se.send("g")

	se.drain()
	if !strings.Contains(string(se.out), "NO CARRIER") {
		t.Fatalf("expected NO CARRIER in final output, got:\n%s", se.out)
	}
	se.waitDone()
}

// TestSecureSeedAccountsClaimsAdmin verifies the seed-takeover hole is shut:
// the seeded sysop "nut" (SL 255, flag A) starts passwordless, and after
// secureSeedAccounts runs it has a real password (so it can no longer be claimed
// by whoever connects first), while a non-privileged seed account is untouched.
func TestSecureSeedAccountsClaimsAdmin(t *testing.T) {
	b := newTestBoard(t)

	nut, err := b.st.UserByHandle("nut")
	if err != nil || nut == nil {
		t.Fatalf("seed nut missing: %v", err)
	}
	if nut.Password != "" {
		t.Fatalf("expected nut seeded passwordless, got a password already")
	}
	if !isPrivileged(nut) {
		t.Fatalf("nut should be privileged (SL %d flags %q)", nut.SL, nut.Flags)
	}

	secureSeedAccounts(b.st)

	nut, _ = b.st.UserByHandle("nut")
	if nut.Password == "" {
		t.Fatal("secureSeedAccounts left the privileged account claimable (empty password)")
	}
	// A non-privileged passwordless account (phantom) is intentionally left
	// alone -- it's safe to claim on first login.
	if ph, _ := b.st.UserByHandle("phantom"); ph == nil || isPrivileged(ph) || ph.Password != "" {
		t.Fatalf("phantom should remain a non-privileged passwordless account, got %+v", ph)
	}
}

// ---- pure helper tests (no session) ----------------------------------------

func TestSizeStr(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{623, "623B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{48213, "47.1K"},
		{196734, "192.1K"},
		{1457620, "1.4M"},
		{8923400, "8.5M"},
	}
	for _, c := range cases {
		if got := sizeStr(c.in); got != c.want {
			t.Errorf("sizeStr(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "node", "nodes"); got != "node" {
		t.Errorf("plural(1) = %q, want %q", got, "node")
	}
	if got := plural(0, "node", "nodes"); got != "nodes" {
		t.Errorf("plural(0) = %q, want %q", got, "nodes")
	}
	if got := plural(2, "user", "users"); got != "users" {
		t.Errorf("plural(2) = %q, want %q", got, "users")
	}
}

func TestTruncStr(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello"},
		{"", 3, ""},
		{"abc", 0, ""},
	}
	for _, c := range cases {
		if got := truncStr(c.in, c.n); got != c.want {
			t.Errorf("truncStr(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestRelTime(t *testing.T) {
	if got := relTime(time.Time{}); got != "never" {
		t.Errorf("relTime(zero) = %q, want %q", got, "never")
	}
	if got := relTime(time.Now().Add(-30 * time.Second)); got != "just now" {
		t.Errorf("relTime(30s ago) = %q, want %q", got, "just now")
	}
	if got := relTime(time.Now().Add(-5 * time.Minute)); got != "5m ago" {
		t.Errorf("relTime(5m ago) = %q, want %q", got, "5m ago")
	}
	if got := relTime(time.Now().Add(-3 * time.Hour)); got != "3h ago" {
		t.Errorf("relTime(3h ago) = %q, want %q", got, "3h ago")
	}
	if got := relTime(time.Now().Add(-48 * time.Hour)); got != "2d ago" {
		t.Errorf("relTime(48h ago) = %q, want %q", got, "2d ago")
	}
}

func TestDateOr(t *testing.T) {
	if got := dateOr(time.Time{}); got != "never" {
		t.Errorf("dateOr(zero) = %q, want %q", got, "never")
	}
	ts := time.Date(2024, 1, 2, 15, 4, 0, 0, time.UTC)
	if got := dateOr(ts); got != "2024-01-02 15:04" {
		t.Errorf("dateOr(%v) = %q, want %q", ts, got, "2024-01-02 15:04")
	}
}

func TestCp437rule(t *testing.T) {
	for _, n := range []int{0, 1, 72} {
		got := cp437rule(n)
		if len(got) != n {
			t.Errorf("cp437rule(%d) len = %d, want %d", n, len(got), n)
		}
		for i := 0; i < len(got); i++ {
			if got[i] != 0xC4 {
				t.Errorf("cp437rule(%d)[%d] = %#x, want 0xC4", n, i, got[i])
			}
		}
	}
}

func TestLc(t *testing.T) {
	cases := []struct {
		in   byte
		want byte
	}{
		{'A', 'a'},
		{'Z', 'z'},
		{'a', 'a'},
		{'z', 'z'},
		{'5', '5'},
		{'@', '@'},
	}
	for _, c := range cases {
		if got := lc(c.in); got != c.want {
			t.Errorf("lc(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSessionRecoverContainsPanic verifies the per-session recover stops a panic
// from unwinding past the goroutine boundary -- one bad session must never crash
// the whole board. If sessionRecover failed to recover, the panic would escape
// the immediately-invoked func below and fail (crash) the test.
func TestSessionRecoverContainsPanic(t *testing.T) {
	func() {
		defer sessionRecover("test:panic")
		panic("boom")
	}()
	// Reaching this point means the panic was contained.
}
