package term

import "io"

// rawSession is a thin io.ReadWriteCloser adapter over a Session for raw byte
// passthrough (door bridging). Reads come from the session's buffered reader so
// no already-buffered input is lost; writes flush any pending buffered output
// then go straight to the underlying connection; Close is a no-op because the
// session owns (and will close) the connection.
type rawSession struct {
	s *Session
}

func (r rawSession) Read(p []byte) (int, error) { return r.s.br.Read(p) }

func (r rawSession) Write(p []byte) (int, error) {
	r.s.bw.Flush()
	return r.s.rwc.Write(p)
}

func (r rawSession) Close() error { return nil }

// Raw returns an io.ReadWriteCloser for raw byte passthrough (door bridging).
// Reads come from the session's buffered reader (so no buffered input is lost);
// writes go straight to the underlying connection after flushing pending output.
func (s *Session) Raw() io.ReadWriteCloser { return rawSession{s: s} }
