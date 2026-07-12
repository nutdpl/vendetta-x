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
	conn, err := acceptWebSocket(w, r)
	if err != nil {
		log.Printf("web: ws upgrade: %v", err)
		return
	}
	// Run the board synchronously on this hijacked connection; it returns when
	// the caller quits or the socket drops. RunTerminal owns closing conn.
	s.cfg.RunTerminal(conn, s.clientIP(r))
}
