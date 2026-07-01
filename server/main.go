// Vendetta/X -- unified BBS platform.
//
// One Go binary, two faces over one shared SQLite spine:
//   - telnet  (ANSI, the old-school terminal board: real .pp art, lightbar menus)
//   - http    (HTML, the modern web BBS)
//
// Connect over telnet and you appear in "who's online" on the web page in real
// time -- one backend, one dataset, rendered two ways.
//
// The telnet face renders the board's real pipe-code art (art/*.pp) through the
// render package, drives lightbar menus off the |{...} markers baked into the
// art, gates access with Iniquity-style ACS strings, authenticates with bcrypt,
// runs a full-screen message editor and a multi-node teleconference, and shows
// social leaderboards -- all reading/writing the same boards/files/users the web
// face serves.
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/auth"
	"vendetta-x/server/internal/bbslist"
	"vendetta-x/server/internal/bulletin"
	"vendetta-x/server/internal/chat"
	"vendetta-x/server/internal/door"
	"vendetta-x/server/internal/dragon"
	"vendetta-x/server/internal/editor"
	"vendetta-x/server/internal/gfiles"
	"vendetta-x/server/internal/mail"
	"vendetta-x/server/internal/render"
	"vendetta-x/server/internal/schedule"
	"vendetta-x/server/internal/social"
	"vendetta-x/server/internal/sshface"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/term"
	"vendetta-x/server/internal/throttle"
	"vendetta-x/server/internal/void"
	"vendetta-x/server/internal/voting"
	"vendetta-x/server/internal/web"
)

const (
	boardName = "Vendetta/X"
	version   = "0.9.0"
)

var (
	telnetAddr = flag.String("telnet", ":2323", "telnet (ANSI) listen address")
	sshAddr    = flag.String("ssh", ":2222", "ssh (ANSI) listen address")
	httpAddr   = flag.String("http", ":8080", "http (web BBS) listen address")
	dbPath     = flag.String("db", "vendetta.db", "path to the SQLite database")
	artDir     = flag.String("art", "art", "directory holding the .pp art files")
	hostKey    = flag.String("hostkey", "vendetta_host_key", "path to the ssh host key (generated if absent)")

	tlsCert       = flag.String("tls-cert", "", "PEM certificate file; serves the web face over HTTPS when set with -tls-key")
	tlsKey        = flag.String("tls-key", "", "PEM private-key file; serves the web face over HTTPS when set with -tls-cert")
	secureCookies = flag.Bool("secure-cookies", false, "mark session cookies Secure (set this when HTTPS is terminated by an upstream proxy)")
	trustProxy    = flag.Bool("trust-proxy", false, "honor X-Forwarded-For for the client IP (login throttling); set ONLY behind a trusted reverse proxy")

	maxNodes    = flag.Int("max-nodes", 64, "maximum concurrent telnet+ssh sessions (0 = unlimited)")
	idleTimeout = flag.Duration("idle", 15*time.Minute, "drop a telnet/ssh session after this much input inactivity (0 = never)")
)

func main() {
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	if err := st.Seed(); err != nil {
		log.Fatalf("store seed: %v", err)
	}

	// feature data layers (each creates/seeds its own table over the shared DB).
	mailStore, err := mail.New(st.DB())
	if err != nil {
		log.Fatalf("mail: %v", err)
	}
	votingStore, err := voting.New(st.DB())
	if err != nil {
		log.Fatalf("voting: %v", err)
	}
	bbsStore, err := bbslist.New(st.DB())
	if err != nil {
		log.Fatalf("bbslist: %v", err)
	}
	gfileStore, err := gfiles.New(st.DB())
	if err != nil {
		log.Fatalf("gfiles: %v", err)
	}
	doorStore, err := door.New(st.DB())
	if err != nil {
		log.Fatalf("door: %v", err)
	}
	dragonStore, err := dragon.New(st.DB())
	if err != nil {
		log.Fatalf("dragon: %v", err)
	}
	voidStore, err := void.New(st.DB())
	if err != nil {
		log.Fatalf("void: %v", err)
	}
	bulletinStore, err := bulletin.New(st.DB())
	if err != nil {
		log.Fatalf("bulletin: %v", err)
	}
	scheduleStore, err := schedule.New(st.DB())
	if err != nil {
		log.Fatalf("schedule: %v", err)
	}

	pres := newPresence()
	bbs := &board{
		st: st, pres: pres, art: *artDir, hub: chat.NewHub(),
		mail: mailStore, voting: votingStore, bbslist: bbsStore, gfiles: gfileStore,
		doorStore:     doorStore,
		dragon:        dragonStore,
		void:          voidStore,
		bulletins:     bulletinStore,
		events:        scheduleStore,
		idle:          *idleTimeout,
		loginThrottle: throttle.New(8, 10*time.Minute),
	}
	if *maxNodes > 0 {
		bbs.sem = make(chan struct{}, *maxNodes)
	}

	// Open the telnet + ssh listeners up front so shutdown can Close them to
	// stop accepting new callers (a fatal bind error still aborts startup).
	telnetLn, err := net.Listen("tcp", *telnetAddr)
	if err != nil {
		log.Fatalf("telnet listen: %v", err)
	}
	sshLn, err := net.Listen("tcp", *sshAddr)
	if err != nil {
		log.Fatalf("ssh listen: %v", err)
	}
	bbs.telnetLn, bbs.sshLn = telnetLn, sshLn
	go bbs.serveTelnet(telnetLn)
	go bbs.serveSSH(sshLn, *hostKey)

	schedCtx, stopSched := context.WithCancel(context.Background())
	defer stopSched()
	go bbs.runScheduler(schedCtx)

	useTLS := *tlsCert != "" && *tlsKey != ""
	webCfg := web.Config{
		BoardName: boardName,
		Version:   version,
		Telnet:    *telnetAddr,
		SSH:       *sshAddr,
		HTTP:      *httpAddr,
		DB:        *dbPath,
		Started:   time.Now(),
		// Session cookies are flagged Secure when we serve HTTPS directly or the
		// operator declares HTTPS is terminated upstream.
		SecureCookies: useTLS || *secureCookies,
		TrustProxy:    *trustProxy,
		// Share the failed-login limiter so an IP's attempts are counted across
		// telnet, ssh, AND web -- one face can't be used to dodge another's count.
		LoginThrottle: bbs.loginThrottle,
	}
	mux := http.NewServeMux()
	mux.Handle("/", web.New(st, pres.list, webCfg))
	srv := &http.Server{
		Addr:    *httpAddr,
		Handler: mux,
		// Slowloris / slow-read defenses. WriteTimeout is generous enough for a
		// full 5 MiB file download on a slow link; IdleTimeout reaps kept-alive
		// connections that go quiet.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	log.Printf("%s: web on %s (%s), telnet on %s, ssh on %s, db %s",
		boardName, *httpAddr, scheme, *telnetAddr, *sshAddr, *dbPath)

	// Run the web server in the background; the main goroutine waits for a
	// shutdown signal so the database (deferred Close) is flushed cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		var err error
		if useTLS {
			err = srv.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("web: %v", err)
		}
	}()

	<-ctx.Done()
	stop() // restore default signal handling so a second signal force-quits
	log.Printf("%s: shutting down...", boardName)

	// Stop accepting new telnet/ssh callers (closing flips first so the accept
	// loops treat the listener Close as a clean stop, not an error to back off).
	bbs.closing.Store(true)
	bbs.telnetLn.Close()
	bbs.sshLn.Close()

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("web shutdown: %v", err)
	}
	// Returning here runs the deferred st.Close(), checkpointing the database so
	// the WAL is folded back before exit.
}

// ---- presence: the shared who's-online tracker ----------------------------

// presence is the live node list, shared by every face. A telnet caller and a
// web visitor read the exact same set.
type presence struct {
	mu   sync.Mutex
	next int
	who  map[int]string
}

func newPresence() *presence { return &presence{who: map[int]string{}} }

func (p *presence) join(who string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.next++
	p.who[p.next] = who
	return p.next
}

func (p *presence) leave(id int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.who, id)
}

func (p *presence) rename(id int, who string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.who[id]; ok {
		p.who[id] = who
	}
}

// list is the web face's online() callback: a stable, sorted snapshot.
func (p *presence) list() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.who))
	for _, w := range p.who {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// ---- the telnet board ------------------------------------------------------

type board struct {
	st   *store.Store
	pres *presence
	art  string
	hub  *chat.Hub

	// feature data layers (each owns its own table over st.DB()).
	mail      *mail.Store
	voting    *voting.Store
	bbslist   *bbslist.Store
	gfiles    *gfiles.Store
	doorStore *door.Store
	dragon    *dragon.Store
	void      *void.Store
	bulletins *bulletin.Store
	events    *schedule.Store

	// sem bounds concurrent telnet+ssh sessions (nil = unlimited); idle is the
	// per-session input-inactivity timeout (0 = never).
	sem  chan struct{}
	idle time.Duration

	loginThrottle *throttle.Throttle // per-IP failed-login limiter

	// listeners + closing flag let shutdown stop accepting new callers cleanly;
	// closing distinguishes a deliberate listener Close from a transient accept
	// error so the accept loop returns instead of hot-spinning or backing off.
	telnetLn net.Listener
	sshLn    net.Listener
	closing  atomic.Bool
}

func (b *board) serveTelnet(ln net.Listener) {
	var delay time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			if b.closing.Load() {
				return // deliberate shutdown: stop accepting
			}
			// Transient accept error (e.g. fd pressure): back off with a capped
			// exponential delay instead of spinning the CPU at 100%.
			if delay == 0 {
				delay = 5 * time.Millisecond
			} else if delay *= 2; delay > time.Second {
				delay = time.Second
			}
			log.Printf("telnet: accept error: %v (retrying in %v)", err, delay)
			time.Sleep(delay)
			continue
		}
		delay = 0
		if !b.acquire() {
			// Board full: tell the caller and drop, rather than spawning an
			// unbounded goroutine.
			conn.Write([]byte("\r\n  All nodes are busy right now. Try again shortly.\r\n"))
			conn.Close()
			continue
		}
		go func() {
			defer b.release()
			b.handle(conn) // goroutine per caller == multinode for free
		}()
	}
}

// acquire takes a node slot, returning false when the board is at capacity. A
// nil sem means unlimited.
func (b *board) acquire() bool {
	if b.sem == nil {
		return true
	}
	select {
	case b.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// release returns a node slot taken by acquire.
func (b *board) release() {
	if b.sem != nil {
		<-b.sem
	}
}

// serveSSH runs the SSH face over an already-open listener: every interactive
// SSH session drives the exact same board flow as telnet, over the encrypted
// channel (no telnet IAC). A clean shutdown (b.closing) suppresses the expected
// "use of closed network connection" error from the closed listener.
func (b *board) serveSSH(ln net.Listener, hostKeyPath string) {
	err := sshface.ServeListener(ln, hostKeyPath, func(ch io.ReadWriteCloser, remote, termType string) {
		defer sessionRecover(remote)
		if !b.acquire() {
			ch.Write([]byte("\r\n  All nodes are busy right now. Try again shortly.\r\n"))
			ch.Close()
			return
		}
		defer b.release()
		s := term.NewRW(ch, remote)
		defer s.Close()
		s.SetIdleTimeout(b.idle)
		// SSH channels have no read deadline for a CPR probe, but the client
		// named its terminal in the pty request -- decide the charset from it.
		s.SetTermType(termType)
		b.runBoard(s)
	})
	if err != nil && !b.closing.Load() {
		log.Printf("ssh: %v", err)
	}
}

// handle serves one telnet caller: telnet negotiation, the anti-bot ESC gate,
// then the shared board flow. SSH callers reach runBoard via term.NewRW (see
// RunBoard); they're already past an SSH handshake, so they skip the gate.
func (b *board) handle(conn net.Conn) {
	defer sessionRecover(conn.RemoteAddr().String())
	s := term.New(conn)
	defer s.Close()
	s.SetIdleTimeout(b.idle)
	s.Negotiate()
	if !b.escGate(s) {
		return // bot / idle scanner -- never reaches the board
	}
	b.runBoard(s)
}

// sessionRecover contains a panic in one caller's goroutine so a single bad
// session can never take down the whole board -- every other node stays up.
func sessionRecover(who string) {
	if r := recover(); r != nil {
		log.Printf("session panic (%s): %v\n%s", who, r, debug.Stack())
	}
}

// escGate is the telnet anti-bot gate: a human presses ESC twice to enter. The
// gate is time-bounded, so scanners that connect and dump bytes (or sit idle)
// never send two ESCs in the window and are dropped before reaching the matrix.
func (b *board) escGate(s *term.Session) bool {
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Print("\x1b[1;30m  " + b.siteName() + "\x1b[0m\r\n\r\n")
	s.Print("  \x1b[0;37mPress \x1b[1;37mESC\x1b[0;37m twice to connect...\x1b[0m")
	s.Flush()

	// Bound the whole gate so idle/garbage connections are reaped.
	if err := s.SetReadDeadline(time.Now().Add(60 * time.Second)); err == nil {
		defer s.SetReadDeadline(time.Time{})
	}
	escs := 0
	for tries := 0; tries < 256; tries++ {
		switch k, _ := s.ReadKey(); k {
		case term.KeyEsc:
			if escs++; escs >= 2 {
				return true
			}
		case term.KeyEOF:
			return false
		}
	}
	return false
}

// RunBoard runs the full board session over an already-built term.Session,
// regardless of transport (telnet socket or SSH channel). Exported so the SSH
// face can drive the same experience.
func (b *board) RunBoard(s *term.Session) { b.runBoard(s) }

// runBoard is the transport-agnostic session: presence, matrix login, and the
// main menu loop.
func (b *board) runBoard(s *term.Session) {
	host := hostOf(s.RemoteAddr())
	id := b.pres.join("connecting@" + host)
	defer b.pres.leave(id)

	// every session carries the same token set the art splices in.
	tok := map[string]string{
		"BN": b.siteName(),
		"VR": version,
		"UH": "guest",
	}
	b.loginTokens(tok)

	b.connect(s, tok)
	user := b.matrix(s, tok)
	if user == nil {
		s.Print("\x1b[0m\r\n  NO CARRIER\r\n")
		s.Flush()
		return
	}
	b.st.RecordLogin(user.ID)
	tok["UH"] = user.Handle
	b.pres.rename(id, user.Handle+"@"+host)

	b.logon(s, tok, user)
	b.mainMenu(s, tok, user)

	s.Print("\x1b[0m\r\n  Later, " + user.Handle + ". NO CARRIER\r\n")
	s.Flush()
}

// isLoopbackHost reports whether host (already stripped of any port) is a
// loopback address -- i.e. the connection came from the machine running the
// board (the "console"). It's the trust boundary for enrolling a passwordless
// privileged account: only the console may set the sysop's first password.
func isLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// subjectOf maps a user onto an ACS subject for access checks.
func subjectOf(u *store.User) acs.Subject {
	return acs.Subject{
		SL:        u.SL,
		DSL:       u.DSL,
		Flags:     u.Flags,
		UserID:    u.ID,
		Group:     u.Group,
		Ansi:      true, // telnet callers are assumed ANSI-capable
		LocalNode: false,
	}
}

// loginTokens splices the live "front porch" stats the login art shows --
// nodes online, total users, total calls, and the board's local time -- into
// tok. Cheap enough to recompute per connection; it makes the loginscreen and
// matrix feel alive instead of like static images.
func (b *board) loginTokens(tok map[string]string) {
	tok["CN"] = strconv.Itoa(len(b.pres.list()))
	if users, err := b.st.Users(); err == nil {
		tok["TU"] = strconv.Itoa(len(users))
		calls := 0
		for i := range users {
			calls += users[i].Calls
		}
		tok["TC"] = strconv.Itoa(calls)
	} else {
		tok["TU"], tok["TC"] = "?", "?"
	}
	tok["TI"] = time.Now().Format("15:04")
}

// matrix runs the login screen: Login / New User / Goodbye. Returns the logged
// in user, or nil if the caller bailed / lost carrier. The first paint animates
// in line by line for the connect entrance; redraws after a failed login are
// instant so a fumbled password doesn't replay the whole show.
func (b *board) matrix(s *term.Session, tok map[string]string) *store.User {
	first := true
	for {
		b.loginTokens(tok)
		var opts []render.Marker
		if first {
			opts, _ = s.Reveal(b.art+"/matrix.pp", tok, 28*time.Millisecond)
			first = false
		} else {
			opts = s.RenderScreen(b.art+"/matrix.pp", tok)
		}
		key, ok := s.Lightbar(opts, 0)
		if !ok {
			return nil
		}
		switch lc(key) {
		case 'l':
			if u := b.login(s); u != nil {
				return u
			}
		case 'n':
			if u := b.newUser(s, tok); u != nil {
				return u
			}
		case 'g':
			return nil
		}
	}
}

func (b *board) login(s *term.Session) *store.User {
	ip := hostOf(s.RemoteAddr())
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Print("\x1b[1;36m  Login\x1b[0m\r\n\r\n")
	if b.loginThrottle.Blocked(ip) {
		s.Print("\x1b[1;31m  Too many login attempts. Wait a few minutes.\x1b[0m\r\n")
		s.Pause()
		return nil
	}
	for tries := 0; tries < 3; tries++ {
		s.Print("\x1b[0;37m  Handle: \x1b[1;37m")
		s.Flush()
		handle := s.ReadLine(20)
		if handle == "" {
			return nil
		}
		u, err := b.st.UserByHandle(handle)
		if err != nil {
			s.Print("\x1b[1;31m  database error, try later.\x1b[0m\r\n")
			return nil
		}
		if u == nil {
			b.loginThrottle.Fail(ip)
			s.Print("\x1b[1;31m  No such user.\x1b[0m\r\n")
			continue
		}
		if u.Password == "" {
			// A privileged account (the seeded sysop) is enrolled on first login,
			// but only from the console -- a loopback connection. Letting any
			// remote caller claim a passwordless admin is a board takeover, so a
			// remote caller is refused and pointed at the console. Run the BBS and
			// log in locally (or tunnel in over SSH) to set the sysop password.
			if u.Privileged() && !isLoopbackHost(ip) {
				b.loginThrottle.Fail(ip)
				s.Print("\x1b[1;31m  That account is reserved. Set its password from the console (a local login).\x1b[0m\r\n")
				s.Pause()
				return nil
			}
			// First login for a passwordless account (the console sysop, or an
			// ordinary seed/legacy user): set a password now.
			s.Print("\x1b[1;33m  No password on file -- set one now.\x1b[0m\r\n")
			if b.setPassword(s, u) {
				s.Print("\x1b[1;32m  Welcome, " + u.Handle + "!\x1b[0m\r\n")
				s.Flush()
				return u
			}
			continue
		}
		s.Print("\x1b[0;37m  Password: \x1b[1;37m")
		s.Flush()
		pw := s.ReadPassword(40)
		if !auth.Verify(u.Password, pw) {
			b.loginThrottle.Fail(ip)
			s.Print("\x1b[1;31m  Bad password.\x1b[0m\r\n")
			continue
		}
		// Note: a successful login deliberately does NOT clear the failure
		// counter -- see throttle.Reset. The window expires it instead, so an
		// attacker can't interleave a success to reset the limiter.
		s.Print("\x1b[1;32m  Welcome back, " + u.Handle + "!\x1b[0m\r\n")
		s.Flush()
		return u
	}
	return nil
}

// setPassword prompts for a password + verification and stores its hash.
func (b *board) setPassword(s *term.Session, u *store.User) bool {
	s.Print("\x1b[0;37m  Password: \x1b[1;37m")
	s.Flush()
	pw := s.ReadPassword(40)
	if pw == "" {
		return false
	}
	if err := auth.ValidatePassword(pw); err != nil {
		s.Print("\x1b[1;31m  " + err.Error() + ".\x1b[0m\r\n")
		return false
	}
	s.Print("\x1b[0;37m  Verify:   \x1b[1;37m")
	s.Flush()
	pw2 := s.ReadPassword(40)
	if pw != pw2 {
		s.Print("\x1b[1;31m  Passwords did not match.\x1b[0m\r\n")
		return false
	}
	hash, err := auth.Hash(pw)
	if err != nil {
		s.Print("\x1b[1;31m  Could not set password.\x1b[0m\r\n")
		return false
	}
	if err := b.st.SetPassword(u.ID, hash); err != nil {
		s.Print("\x1b[1;31m  Could not save password.\x1b[0m\r\n")
		return false
	}
	u.Password = hash
	return true
}

// newUser is the signup: handle + password + real name + location. The full
// Iniquity-style form (email, birthdate, protocol, ...) layers on from here.
func (b *board) newUser(s *term.Session, tok map[string]string) *store.User {
	s.RenderScreen(b.art+"/newuser.pp", tok)
	s.Print("\x1b[0m\r\n")
	var handle string
	for {
		s.Print("\x1b[0;37m  Pick a handle: \x1b[1;37m")
		s.Flush()
		handle = s.ReadLine(20)
		if handle == "" {
			return nil
		}
		if err := store.ValidateHandle(handle); err != nil {
			s.Print("\x1b[1;31m  " + err.Error() + ".\x1b[0m\r\n")
			continue
		}
		u, err := b.st.UserByHandle(handle)
		if err != nil {
			s.Print("\x1b[1;31m  database error, try later.\x1b[0m\r\n")
			return nil
		}
		if u != nil {
			s.Print("\x1b[1;31m  That handle is taken. Try another.\x1b[0m\r\n")
			continue
		}
		break
	}

	s.Print("\x1b[0;37m  Choose a password: \x1b[1;37m")
	s.Flush()
	pw := s.ReadPassword(40)
	if pw == "" {
		return nil
	}
	if err := auth.ValidatePassword(pw); err != nil {
		s.Print("\x1b[1;31m  " + err.Error() + ". Signup cancelled.\x1b[0m\r\n")
		s.Pause()
		return nil
	}
	s.Print("\x1b[0;37m  Verify password:   \x1b[1;37m")
	s.Flush()
	if s.ReadPassword(40) != pw {
		s.Print("\x1b[1;31m  Passwords did not match. Signup cancelled.\x1b[0m\r\n")
		s.Pause()
		return nil
	}
	hash, err := auth.Hash(pw)
	if err != nil {
		s.Print("\x1b[1;31m  Could not hash password.\x1b[0m\r\n")
		return nil
	}

	s.Print("\x1b[0;37m  Real name: \x1b[1;37m")
	s.Flush()
	real := s.ReadLine(30)
	s.Print("\x1b[0;37m  Location: \x1b[1;37m")
	s.Flush()
	loc := s.ReadLine(30)

	now := time.Now()
	u := &store.User{
		Handle:    handle,
		RealName:  real,
		Location:  loc,
		Group:     b.st.Setting("newuser.group", "Users"),
		Tagline:   "New blood.",
		SL:        b.st.SettingInt("newuser.sl", 10),
		DSL:       b.st.SettingInt("newuser.dsl", 10),
		Password:  hash,
		FirstCall: now,
		LastCall:  now,
	}
	id, err := b.st.AddUser(u)
	if err != nil {
		s.Print("\x1b[1;31m  Could not create account.\x1b[0m\r\n")
		return nil
	}
	u.ID = id
	b.welcomeNewUser(s, tok, u, loc)
	return u
}

// mainMenu is the top-level lightbar. Loops until Goodbye / carrier loss.
func (b *board) mainMenu(s *term.Session, tok map[string]string, user *store.User) {
	first := true
	for {
		var opts []render.Marker
		if first {
			// Paint the menu in line by line the first time the caller lands on
			// it; redraws after backing out of a sub-area are instant.
			opts, _ = s.Reveal(b.art+"/mainmenu.pp", tok, 16*time.Millisecond)
			first = false
		} else {
			opts = s.RenderScreen(b.art+"/mainmenu.pp", tok)
		}
		key, ok := s.Lightbar(opts, 0)
		if !ok {
			return
		}
		// gated runs a toggleable feature, or shows a closed notice when the
		// sysop has switched it off in the configuration program.
		gated := func(key string, fn func()) {
			if b.st.FeatureEnabled(key) {
				fn()
				return
			}
			s.Notice("That area is closed by the sysop.")
		}
		switch lc(key) {
		case 'm':
			b.messageMenu(s, tok, user)
		case 'f':
			b.fileMenu(s, tok, user)
		case 'e':
			gated("email", func() { b.email(s, tok, user) })
		case 'o':
			gated("oneliners", func() { b.oneliners(s, user) })
		case 'w':
			b.whosOnline(s)
		case 'c':
			gated("teleconference", func() { b.teleconference(s, user) })
		case 'd':
			gated("doors", func() { b.doors(s, tok, user) })
		case 'q':
			gated("qwk", func() { b.qwk(s, tok, user) })
		case 'n':
			gated("newfiles", func() { b.newFiles(s, user) })
		case 't':
			gated("gfiles", func() { b.gFiles(s, tok, user) })
		case 'b':
			gated("bbslist", func() { b.bbsList(s, tok, user) })
		case 'v':
			gated("voting", func() { b.votingBooth(s, tok, user) })
		case 'u':
			b.userList(s)
		case 'l':
			b.lastCallers(s)
		case 'z':
			b.profile(s, user)
		case 'i':
			b.sysInfo(s, tok, user)
		case 'x':
			b.settings(s, tok, user)
		case 'g':
			return
		default:
			s.Notice("That feature isn't wired up yet -- coming soon.")
		}
	}
}

// messageMenu is the message command menu (Iniquity-style): it operates on a
// "current" base -- Read it, Post to it, scan for New, or Change Base via the
// numbered area picker. Pressing M from the main menu lands here, on the first
// base the caller can read.
func (b *board) messageMenu(s *term.Session, tok map[string]string, user *store.User) {
	current := b.firstReadableBoard(user)
	if current == nil {
		s.Notice("No message bases are available to you.")
		return
	}
	for {
		tok["MB"] = current.Name
		opts := s.RenderScreen(b.art+"/msgmenu.pp", tok)
		key, ok := s.Lightbar(opts, 0)
		if !ok {
			return
		}
		switch lc(key) {
		case 'r':
			b.readBoard(s, current.Tag, user)
		case 'p':
			if !acs.Eval(current.PostACS, subjectOf(user)) {
				s.Notice("You don't have post access on " + current.Name + ".")
				break
			}
			b.postMessage(s, current, user)
		case 'n':
			b.newScan(s, user)
		case 'a':
			if pick := b.pickBase(s, user); pick != nil {
				current = pick
			}
		case 'q':
			return
		}
	}
}

// firstReadableBoard returns the first base the user may read, or nil.
func (b *board) firstReadableBoard(user *store.User) *store.Board {
	boards, err := b.st.Boards()
	if err != nil {
		return nil
	}
	subj := subjectOf(user)
	for i := range boards {
		if acs.Eval(boards[i].ReadACS, subj) {
			return &boards[i]
		}
	}
	return nil
}

// pickBase shows the Iniquity-style numbered base list (MSGAREA.PAS maListAreas):
// Num / Area Title / Msgs / Last Post under a header rule, select by number.
// Restricted bases are listed dimmed and can't be chosen. Returns the selected
// readable base, or nil if the caller quits.
func (b *board) pickBase(s *term.Session, user *store.User) *store.Board {
	subj := subjectOf(user)
	for {
		boards, err := b.st.Boards()
		if err != nil {
			s.Notice("Could not load boards.")
			return nil
		}

		s.Print("\x1b[0m\x1b[2J\x1b[H")
		s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m change message base\x1b[0m\r\n\r\n", boardName)
		s.Print("\x1b[1;30m   #  \x1b[0;37mArea Title                    \x1b[1;30mMsgs  Last Post\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")

		for i := range boards {
			bd := &boards[i]
			n := i + 1
			count := len(mustMessages(b, bd.ID))
			if !acs.Eval(bd.ReadACS, subj) {
				s.Printf("\x1b[1;30m  %2d  %-28s \x1b[31m[%s]\x1b[1;30m %4d  --\x1b[0m\r\n",
					n, truncStr(bd.Name, 28), bd.ReadACS, count)
				continue
			}
			last := "\x1b[1;30m--"
			if recent := mustMessages(b, bd.ID); len(recent) > 0 {
				last = "\x1b[1;36m" + recent[0].From + " \x1b[0;37m" + relTime(recent[0].Posted)
			}
			s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-28s \x1b[0;37m%4d  %s\x1b[0m\r\n",
				n, truncStr(bd.Name, 28), count, last)
		}

		s.Print("\r\n\x1b[0;37m  Message base \x1b[1;37m#\x1b[0;37m (\x1b[1;37mQ\x1b[0;37m to quit) \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		line := strings.TrimSpace(s.ReadLine(5))
		if line == "" || lc(line[0]) == 'q' {
			return nil
		}
		n, perr := strconv.Atoi(line)
		if perr != nil || n < 1 || n > len(boards) {
			continue
		}
		bd := &boards[n-1]
		if !acs.Eval(bd.ReadACS, subj) {
			s.Notice("Access denied: " + bd.Name + " requires " + bd.ReadACS + ".")
			continue
		}
		return bd
	}
}

// newScan lists the most recent messages across every base the caller can read
// (the Iniquity [N]ew scan).
func (b *board) newScan(s *term.Session, user *store.User) {
	subj := subjectOf(user)
	boards, _ := b.st.Boards()
	readable := map[int64]string{}
	for i := range boards {
		if acs.Eval(boards[i].ReadACS, subj) {
			readable[boards[i].ID] = boards[i].Name
		}
	}

	msgs, _ := b.st.RecentMessages(40)
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m new messages\x1b[0m\r\n", boardName)
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n\r\n")
	shown := 0
	for _, m := range msgs {
		base, ok := readable[m.BoardID]
		if !ok {
			continue
		}
		s.Printf("  \x1b[1;33m%-14s \x1b[1;37m%-28s \x1b[1;36m%-12s \x1b[0;37m%s\x1b[0m\r\n",
			truncStr(base, 14), truncStr(m.Subject, 28), truncStr(m.From, 12), relTime(m.Posted))
		shown++
	}
	if shown == 0 {
		s.Print("\x1b[0;37m  Nothing new across your bases.\x1b[0m\r\n")
	}
	s.Pause()
}

// mustMessages returns a board's messages newest-first (nil on error), a small
// helper so the listing stays readable.
func mustMessages(b *board, boardID int64) []store.Message {
	msgs, err := b.st.Messages(boardID, 0)
	if err != nil {
		return nil
	}
	return msgs
}

func (b *board) readBoard(s *term.Session, tag string, user *store.User) {
	boards, err := b.st.Boards()
	if err != nil {
		s.Notice("Could not load boards.")
		return
	}
	var bd *store.Board
	for i := range boards {
		if boards[i].Tag == tag {
			bd = &boards[i]
			break
		}
	}
	if bd == nil {
		s.Notice("That base is empty.")
		return
	}

	// ACS read gate -- the Iniquity-style access check.
	subj := subjectOf(user)
	if !acs.Eval(bd.ReadACS, subj) {
		s.Notice("Access denied: you lack the access for " + bd.Name + ".")
		return
	}

	msgs, err := b.st.Messages(bd.ID, 50)
	if err != nil {
		s.Notice("Could not load messages.")
		return
	}

	canPost := acs.Eval(bd.PostACS, subj)

	// Empty base: a quick notice, with a post affordance if the caller can write.
	if len(msgs) == 0 {
		s.Print("\x1b[0m\x1b[2J\x1b[H")
		s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m %s\x1b[0m\r\n", boardName, bd.Name)
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n\r\n")
		s.Print("\x1b[0;37m  No messages yet. Be the first.\x1b[0m\r\n")
		if canPost {
			s.Print("\r\n\x1b[0;37m  [\x1b[1;37mP\x1b[0;37m]ost a message, or any key to go back: \x1b[0m")
			s.Flush()
			if k, ch := s.ReadKey(); k == term.KeyChar && lc(ch) == 'p' {
				b.postMessage(s, bd, user)
			}
			return
		}
		s.Pause()
		return
	}

	// Read one message at a time, Iniquity-style: a framed header (group, sender,
	// subject, date) over the body, with prev/next/reply/quit navigation.
	i := 0
	for {
		b.showMessage(s, bd, msgs, i, canPost)
		k, ch := s.ReadKey()
		switch {
		case k == term.KeyEsc, k == term.KeyEOF, k == term.KeyChar && lc(ch) == 'q':
			return
		case k == term.KeyRight, k == term.KeyDown, k == term.KeyEnter,
			k == term.KeyChar && (lc(ch) == 'n' || ch == ' '):
			if i < len(msgs)-1 {
				i++
			}
		case k == term.KeyLeft, k == term.KeyUp, k == term.KeyChar && lc(ch) == 'p':
			if i > 0 {
				i--
			}
		case canPost && k == term.KeyChar && lc(ch) == 'r':
			b.postReply(s, bd, user, msgs[i])
		}
	}
}

// postMessage composes a fresh public message addressed to All.
func (b *board) postMessage(s *term.Session, bd *store.Board, user *store.User) {
	b.compose(s, bd, user, "All", "")
}

// compose runs the subject prompt + full-screen body editor and posts the
// result. toDefault is the recipient (All for public posts, the original sender
// for a reply); subjDefault pre-fills the subject (blank for a new message,
// "Re: ..." for a reply) and is kept if the caller just presses enter.
func (b *board) compose(s *term.Session, bd *store.Board, user *store.User, toDefault, subjDefault string) {
	if subjDefault != "" {
		s.Printf("\r\n\x1b[0;37m  Subject \x1b[1;30m[%s]\x1b[0;37m: \x1b[1;37m", subjDefault)
	} else {
		s.Print("\r\n\x1b[0;37m  Subject: \x1b[1;37m")
	}
	s.Flush()
	subj := strings.TrimSpace(s.ReadLine(50))
	if subj == "" {
		subj = subjDefault
	}
	if subj == "" {
		return
	}

	// Full-screen editor for the body.
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m post to %s\x1b[0m\r\n", boardName, bd.Name)
	s.Printf("\x1b[1;30m  to: \x1b[0;37m%s  \x1b[1;30msubject: \x1b[0;37m%s\x1b[0m\r\n", toDefault, subj)
	s.Print("\x1b[1;30m  Ctrl-Z to save and post \xfa Esc to abort \xfa arrows to move\x1b[0m\r\n")
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	s.Flush()

	ed := editor.New(editorConsole{s}, 5, 3, 72, 16, nil)
	lines, saved := ed.Run()
	// Restore a clean cursor/attr state after the editor.
	s.Print("\x1b[0m\x1b[24;1H\r\n")
	if !saved {
		s.Print("\x1b[1;33m  Post aborted.\x1b[0m\r\n")
		s.Pause()
		return
	}
	body := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if strings.TrimSpace(body) == "" {
		s.Print("\x1b[1;33m  Empty message -- not posted.\x1b[0m\r\n")
		s.Pause()
		return
	}

	if _, err := b.st.PostMessage(&store.Message{
		BoardID: bd.ID,
		From:    user.Handle,
		To:      toDefault,
		Subject: subj,
		Body:    body,
		Posted:  time.Now(),
	}); err != nil {
		s.Notice("Could not post your message.")
		return
	}
	b.st.IncPosts(user.Handle)
	user.Posts++
	s.Print("\x1b[1;32m  Posted!\x1b[0m\r\n")
	s.Pause()
}

// fileMenu is the file command menu (Iniquity FILEAREA.PAS-style): it operates
// on a "current" area -- List its files, scan New uploads, or Change Area via
// the numbered picker. Pressing F from the main menu lands here.
func (b *board) fileMenu(s *term.Session, tok map[string]string, user *store.User) {
	current := b.firstAccessibleArea(user)
	if current == nil {
		s.Notice("No file areas are available to you.")
		return
	}
	for {
		tok["FA"] = current.Name
		opts := s.RenderScreen(b.art+"/filemenu.pp", tok)
		key, ok := s.Lightbar(opts, 0)
		if !ok {
			return
		}
		switch lc(key) {
		case 'l':
			b.listFiles(s, user, current)
		case 'n':
			b.newFiles(s, user)
		case 'a':
			if pick := b.pickFileArea(s, user); pick != nil {
				current = pick
			}
		case 'q':
			return
		}
	}
}

// firstAccessibleArea returns the first file area the user may access, or nil.
func (b *board) firstAccessibleArea(user *store.User) *store.FileArea {
	areas, err := b.st.FileAreas()
	if err != nil {
		return nil
	}
	subj := subjectOf(user)
	for i := range areas {
		if acs.Eval(areas[i].ACS, subj) {
			return &areas[i]
		}
	}
	return nil
}

// pickFileArea shows the Iniquity-style numbered area list (FILEAREA.PAS
// faListAreas): Num / Area Title / Files / About, select by number. Restricted
// areas list dimmed and can't be chosen. Returns the selected area, or nil.
func (b *board) pickFileArea(s *term.Session, user *store.User) *store.FileArea {
	subj := subjectOf(user)
	for {
		areas, err := b.st.FileAreas()
		if err != nil {
			s.Notice("Could not load file areas.")
			return nil
		}

		s.Print("\x1b[0m\x1b[2J\x1b[H")
		s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m change file area\x1b[0m\r\n\r\n", boardName)
		s.Print("\x1b[1;30m   #  \x1b[0;37mArea Title                    \x1b[1;30mFiles  About\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")

		for i := range areas {
			a := &areas[i]
			n := i + 1
			files, _ := b.st.Files(a.ID)
			if !acs.Eval(a.ACS, subj) {
				s.Printf("\x1b[1;30m  %2d  %-28s \x1b[31m[%s]\x1b[1;30m %4d  --\x1b[0m\r\n",
					n, truncStr(a.Name, 28), a.ACS, len(files))
				continue
			}
			s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-28s \x1b[0;37m%4d  \x1b[1;30m%s\x1b[0m\r\n",
				n, truncStr(a.Name, 28), len(files), truncStr(a.Desc, 30))
		}

		s.Print("\r\n\x1b[0;37m  File area \x1b[1;37m#\x1b[0;37m (\x1b[1;37mQ\x1b[0;37m to quit) \x1b[1;36m> \x1b[1;37m")
		s.Flush()
		line := strings.TrimSpace(s.ReadLine(5))
		if line == "" || lc(line[0]) == 'q' {
			return nil
		}
		n, perr := strconv.Atoi(line)
		if perr != nil || n < 1 || n > len(areas) {
			continue
		}
		a := &areas[n-1]
		if !acs.Eval(a.ACS, subj) {
			s.Notice("Access denied: " + a.Name + " requires " + a.ACS + ".")
			continue
		}
		return a
	}
}

// listFiles prints an area's files the Iniquity way (lfDescLn): Num / Filename /
// Size / DLs / Description. Downloads happen on the web face (signed links).
func (b *board) listFiles(s *term.Session, user *store.User, area *store.FileArea) {
	for {
		files, err := b.st.Files(area.ID)
		if err != nil {
			s.Notice("Could not load files.")
			return
		}

		s.Print("\x1b[0m\x1b[2J\x1b[H")
		s.Printf("\x1b[1;35m  %s \x1b[1;30m\xfa\x1b[0;36m %s\x1b[0m\r\n", boardName, area.Name)
		if area.Desc != "" {
			s.Printf("\x1b[1;30m  %s\x1b[0m\r\n", area.Desc)
		}
		s.Print("\r\n\x1b[1;35m   #  \x1b[0;36mFilename             \x1b[1;30m   Size  DLs  Description\x1b[0m\r\n")
		s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")

		if len(files) == 0 {
			s.Print("\x1b[0;37m  No files in this area yet.\x1b[0m\r\n")
		}
		for i, f := range files {
			s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-20s \x1b[0;37m%7s  %3d  \x1b[1;30m%s\x1b[0m\r\n",
				i+1, truncStr(f.Filename, 20), sizeStr(f.Size), f.Downloads, truncStr(f.Desc, 30))
		}
		if b.ratioEnabled() && !b.ratioExempt(user) {
			s.Printf("\r\n\x1b[1;30m  %s credit left \xfa upload to earn more.\x1b[0m\r\n",
				sizeStr(b.downloadAllowance(user)))
		}
		s.Print("\r\n\x1b[0;37m  [\x1b[1;37m#\x1b[0;37m] download  [\x1b[1;37mU\x1b[0;37m]pload  " +
			"[\x1b[1;37mQ\x1b[0;37m]uit \x1b[1;36m> \x1b[1;37m")
		s.Flush()

		line := strings.TrimSpace(s.ReadLine(8))
		switch {
		case line == "" || strings.EqualFold(line, "q"):
			return
		case strings.EqualFold(line, "u"):
			b.uploadFile(s, user, area)
			s.Pause()
		default:
			n, convErr := strconv.Atoi(line)
			if convErr != nil || n < 1 || n > len(files) {
				s.Notice("No such file number.")
				continue
			}
			b.downloadFile(s, user, &files[n-1])
			s.Pause()
		}
	}
}

// newFiles lists the most recent uploads across every area the caller can
// access (the Iniquity [N]ew files scan).
func (b *board) newFiles(s *term.Session, user *store.User) {
	subj := subjectOf(user)
	areas, _ := b.st.FileAreas()
	type row struct {
		area string
		f    store.FileEntry
	}
	var rows []row
	for i := range areas {
		if !acs.Eval(areas[i].ACS, subj) {
			continue
		}
		fs, _ := b.st.Files(areas[i].ID)
		for _, f := range fs {
			rows = append(rows, row{areas[i].Name, f})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].f.Uploaded.After(rows[j].f.Uploaded) })

	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Printf("\x1b[1;36m  %s \x1b[1;30m\xfa\x1b[0;37m new files\x1b[0m\r\n", boardName)
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n\r\n")
	shown := 0
	for _, r := range rows {
		if shown >= 20 {
			break
		}
		s.Printf("  \x1b[1;33m%-14s \x1b[1;37m%-20s \x1b[0;37m%7s  \x1b[1;36m%-10s \x1b[0;37m%s\x1b[0m\r\n",
			truncStr(r.area, 14), truncStr(r.f.Filename, 20), sizeStr(r.f.Size),
			truncStr(r.f.Uploader, 10), relTime(r.f.Uploaded))
		shown++
	}
	if shown == 0 {
		s.Print("\x1b[0;37m  Nothing uploaded yet.\x1b[0m\r\n")
	}
	s.Pause()
}

// siteName is the board's display name, sysop-configurable via the settings
// table (falling back to the compiled-in default for a fresh database).
func (b *board) siteName() string { return b.st.Setting("board.name", boardName) }

// screenHeader clears the screen and prints the standard "Board . title" banner
// used across every list/info screen, for one consistent look.
func (b *board) screenHeader(s *term.Session, title string) {
	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Printf("\x1b[1;35m  %s \x1b[1;30m\xfa\x1b[0;36m %s\x1b[0m\r\n\r\n", b.siteName(), title)
}

func (b *board) oneliners(s *term.Session, user *store.User) {
	liners, err := b.st.Oneliners(15)
	if err != nil {
		s.Notice("Could not load the wall.")
		return
	}
	b.screenHeader(s, "the wall \xfa oneliners")
	if len(liners) == 0 {
		s.Print("\x1b[0;37m  The wall is blank. Tag it.\x1b[0m\r\n")
	}
	for _, o := range liners {
		s.Printf("  \x1b[1;35m%-12s \x1b[1;30m\xb3 \x1b[0;37m%s\x1b[0m\r\n", truncStr(o.Author, 12), o.Text)
	}
	s.Print("\r\n\x1b[0;37m  Leave your mark \x1b[1;30m(enter to skip)\x1b[0;37m: \x1b[1;37m")
	s.Flush()
	text := s.ReadLine(80)
	if strings.TrimSpace(text) != "" {
		if err := b.st.AddOneliner(&store.Oneliner{Author: user.Handle, Text: text, Posted: time.Now()}); err != nil {
			s.Notice("Could not post your line.")
			return
		}
		s.Print("\x1b[1;32m  Tagged!\x1b[0m\r\n")
		s.Pause()
	}
}

func (b *board) whosOnline(s *term.Session) {
	online := b.pres.list()
	b.screenHeader(s, "who's online")
	s.Print("\x1b[1;35m   #  \x1b[0;36mCaller\x1b[0m\r\n")
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	for i, w := range online {
		s.Printf("  \x1b[1;33m%2d  \x1b[1;36m\xfe \x1b[0;37m%s\x1b[0m\r\n", i+1, w)
	}
	if len(online) == 0 {
		s.Print("\x1b[0;37m  Nobody on the wire right now.\x1b[0m\r\n")
	}
	s.Printf("\r\n\x1b[1;30m  %d %s connected.\x1b[0m\r\n", len(online), plural(len(online), "node", "nodes"))
	s.Pause()
}

func (b *board) userList(s *term.Session) {
	users, err := b.st.Users()
	if err != nil {
		s.Notice("Could not load users.")
		return
	}
	b.screenHeader(s, "user list")
	s.Printf("\x1b[1;35m  %-16s %-9s %4s %6s  %s\x1b[0m\r\n", "Handle", "Group", "SL", "Posts", "Tagline")
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	for _, u := range users {
		s.Printf("  \x1b[1;37m%-16s \x1b[0;37m%-9s \x1b[1;36m%4d \x1b[0;37m%6d  \x1b[1;30m%s\x1b[0m\r\n",
			truncStr(u.Handle, 16), truncStr(u.Group, 9), u.SL, u.Posts, truncStr(u.Tagline, 28))
	}
	s.Printf("\r\n\x1b[1;30m  %d %s on file.\x1b[0m\r\n", len(users), plural(len(users), "user", "users"))
	s.Pause()
}

// lastCallers shows the last-callers table plus the social leaderboards.
func (b *board) lastCallers(s *term.Session) {
	users, err := b.st.Users()
	if err != nil {
		s.Notice("Could not load users.")
		return
	}
	b.screenHeader(s, "last callers")
	s.Printf("\x1b[1;35m   #  %-16s %-9s %s\x1b[0m\r\n", "Handle", "Group", "Last Call")
	s.Print("\x1b[1;30m  " + cp437rule(72) + "\x1b[0m\r\n")
	for i, u := range social.LastCallers(users, 12) {
		s.Printf("  \x1b[1;33m%2d  \x1b[1;37m%-16s \x1b[0;37m%-9s \x1b[1;30m%s\x1b[0m\r\n",
			i+1, truncStr(u.Handle, 16), truncStr(u.Group, 9), relTime(u.LastCall))
	}
	s.Print("\r\n")
	s.Print(social.LeaderBoard(social.Rank(users, 5)))
	s.Pause()
}

// profile shows the caller's own stats as a clean key/value block.
func (b *board) profile(s *term.Session, user *store.User) {
	// re-read to pick up freshly bumped counters
	if u, err := b.st.UserByHandle(user.Handle); err == nil && u != nil {
		user = u
	}
	b.screenHeader(s, "your stats")
	row := func(label, val string) {
		if val == "" {
			return
		}
		s.Printf("  \x1b[1;36m%-12s\x1b[1;30m\xb3 \x1b[1;37m%s\x1b[0m\r\n", label, val)
	}
	row("Handle", user.Handle)
	row("Real Name", user.RealName)
	row("Location", user.Location)
	row("Group", user.Group)
	row("Security", "SL "+strconv.Itoa(user.SL)+" / DSL "+strconv.Itoa(user.DSL))
	row("Posts", strconv.Itoa(user.Posts))
	row("Calls", strconv.Itoa(user.Calls))
	row("Files", b.ratioLine(user))
	row("First Call", dateOr(user.FirstCall))
	row("Last Call", dateOr(user.LastCall))
	row("Tagline", user.Tagline)
	row("Flags", user.Flags)
	s.Pause()
}

// ---- teleconference (multi-node chat) -------------------------------------

// teleconference joins the caller to the shared chat channel and runs a live
// duplex loop: a goroutine reads keystrokes while the main goroutine (the sole
// writer) prints both incoming lines and the caller's own echo.
func (b *board) teleconference(s *term.Session, user *store.User) {
	const channel = "main"
	me := b.hub.Join(channel, user.Handle)
	defer me.Leave()

	s.Print("\x1b[0m\x1b[2J\x1b[H")
	s.Printf("\x1b[1;35m  Teleconference \x1b[0;37m-- channel #%s\x1b[0m\r\n", channel)
	s.Print("\x1b[1;30m  Type and press enter to talk. /q quits, /w lists who's here.\x1b[0m\r\n\r\n")
	s.Printf("\x1b[1;32m  -!- on channel: %s\x1b[0m\r\n", strings.Join(b.hub.Who(channel), ", "))
	s.Print("\x1b[1;36m> \x1b[0m")
	s.Flush()

	type keyEv struct {
		k  term.Kind
		ch byte
	}
	keys := make(chan keyEv, 16)
	done := make(chan struct{})
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		for {
			k, ch := s.ReadKey()
			select {
			case keys <- keyEv{k, ch}:
			case <-done:
				return
			}
			if k == term.KeyEOF {
				return
			}
		}
	}()

	var line []byte
	reprompt := func() {
		s.Print("\x1b[1;36m> \x1b[0m" + string(line))
		s.Flush()
	}

chatLoop:
	for {
		select {
		case m, ok := <-me.Recv():
			if !ok {
				break chatLoop
			}
			s.Print("\r\x1b[K") // wipe the prompt line
			if m.Sys {
				s.Printf("\x1b[1;33m  -!- %s\x1b[0m\r\n", m.Text)
			} else {
				s.Printf("\x1b[1;36m  <%s>\x1b[0m %s\r\n", m.From, m.Text)
			}
			reprompt()
		case ev := <-keys:
			switch ev.k {
			case term.KeyEOF:
				break chatLoop
			case term.KeyEsc:
				break chatLoop
			case term.KeyEnter:
				txt := strings.TrimSpace(string(line))
				line = line[:0]
				s.Print("\r\n")
				switch {
				case txt == "/q":
					break chatLoop
				case txt == "/w":
					s.Printf("\x1b[1;32m  -!- on channel: %s\x1b[0m\r\n", strings.Join(b.hub.Who(channel), ", "))
				case txt != "":
					me.Say(txt)
					s.Printf("\x1b[1;36m  <%s>\x1b[0m %s\r\n", user.Handle, txt) // echo own line
				}
				reprompt()
			case term.KeyChar:
				if ev.ch == 8 || ev.ch == 127 {
					if len(line) > 0 {
						line = line[:len(line)-1]
						s.Print("\b \b")
						s.Flush()
					}
				} else if ev.ch >= 32 && len(line) < 200 {
					line = append(line, ev.ch)
					s.Write([]byte{ev.ch})
					s.Flush()
				}
			}
		}
	}

	// Tear down the key reader cleanly: signal done, interrupt its blocked
	// ReadKey with a read deadline, wait for it, then clear the deadline so the
	// rest of the session reads normally. Transports without a read deadline
	// (e.g. SSH channels) can't be interrupted mid-Read, so we detach the reader
	// instead -- it exits on the caller's next key or hangup.
	close(done)
	if err := s.SetReadDeadline(time.Now()); err == nil {
		<-gone
		s.SetReadDeadline(time.Time{})
	}
	s.Print("\x1b[0m\r\n  Left the teleconference.\r\n")
	s.Flush()
}

// editorConsole adapts a term.Session to the editor.Console interface.
type editorConsole struct{ s *term.Session }

func (e editorConsole) Write(str string) { e.s.Print(str) }
func (e editorConsole) Flush()           { e.s.Flush() }

func (e editorConsole) ReadKey() (editor.Key, rune) {
	for {
		k, ch := e.s.ReadKey()
		switch k {
		case term.KeyEnter:
			return editor.KeyEnter, 0
		case term.KeyUp:
			return editor.KeyUp, 0
		case term.KeyDown:
			return editor.KeyDown, 0
		case term.KeyLeft:
			return editor.KeyLeft, 0
		case term.KeyRight:
			return editor.KeyRight, 0
		case term.KeyEsc:
			return editor.KeyAbort, 0
		case term.KeyEOF:
			return editor.KeyEOF, 0
		case term.KeyChar:
			switch ch {
			case 8, 127:
				return editor.KeyBackspace, 0
			case 26: // Ctrl-Z
				return editor.KeySave, 0
			case 3: // Ctrl-C
				return editor.KeyAbort, 0
			case 4: // Ctrl-D
				return editor.KeyDelete, 0
			case 1: // Ctrl-A -> home
				return editor.KeyHome, 0
			case 5: // Ctrl-E -> end
				return editor.KeyEnd, 0
			default:
				if ch >= 32 {
					return editor.KeyRune, rune(ch)
				}
				// ignore other control bytes
			}
		}
	}
}

// ---- small helpers ---------------------------------------------------------

// hostOf strips the port from a "host:port" address, returning just the host.
func hostOf(addr string) string {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return h
}

func lc(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

// cp437rule returns n bytes of CP437 0xC4 (the single horizontal line), matching
// the box-drawing the .pp art uses.
func cp437rule(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = 0xC4
	}
	return string(out)
}

// sizeStr renders a byte count compactly (e.g. "623B", "47.1K", "8.5M").
func sizeStr(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + "B"
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	units := []string{"K", "M", "G", "T"}
	return strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64) + units[exp]
}

// plural returns sing for n==1, else plur.
func plural(n int, sing, plur string) string {
	if n == 1 {
		return sing
	}
	return plur
}

// dateOr formats t, or "never" for the zero time.
func dateOr(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("2006-01-02 15:04")
}

// truncStr clamps s to n runes (the listing columns are fixed width).
func truncStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// relTime renders a compact relative time for the message-base "last post".
func relTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}
