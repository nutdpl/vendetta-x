package web

import (
	"log"
	"net/http"
)

// terminal renders the browser terminal page: a full ANSI board session in a
// tab, driven by the vendored xterm.js over a WebSocket to /ws-term. Owned by
// the webterm feature.
func (s *server) terminal(w http.ResponseWriter, r *http.Request) {
	s.render(w, "terminal", struct {
		pageData
	}{s.base(r, "terminal", "terminal")})
}

// wsTerminal upgrades the request to a WebSocket and hands the socket to the
// board runner as an io.ReadWriteCloser -- the exact entry the ssh face uses,
// so a browser caller gets the real board (lightbars, editor, doors) with the
// node cap and bans applied. Nothing happens unless main wired a runner in.
func (s *server) wsTerminal(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RunTerminal == nil {
		http.Error(w, "web terminal not available", http.StatusServiceUnavailable)
		return
	}
	// Same-origin gate. The upgrade is a GET, so csrfGuard (which only checks
	// unsafe methods) never sees it -- without this, any cross-origin page could
	// open a readable+writable board session in a victim's browser (cross-site
	// WebSocket hijacking), and from a co-located browser that reaches the loopback
	// enrollment path it could claim a still-passwordless privileged account.
	// Browsers always send Origin on a WebSocket handshake; sameOrigin rejects a
	// foreign one while still allowing header-less non-browser clients.
	if !sameOrigin(r) {
		http.Error(w, "cross-origin websocket blocked", http.StatusForbidden)
		return
	}
	conn, err := acceptWebSocket(w, r)
	if err != nil {
		log.Printf("web: ws upgrade: %v", err)
		return
	}
	// Run the board synchronously on this hijacked connection; it returns when
	// the caller quits or the socket drops. RunTerminal owns closing conn.
	s.cfg.RunTerminal(conn, s.clientIP(r))
}
