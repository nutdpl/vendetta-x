package web

import "net/http"

// styleguide renders the design-system reference page: a living catalog of
// every component class so feature pages can compose them. Owned by the
// design-system agent.
func (s *server) styleguide(w http.ResponseWriter, r *http.Request) {
	s.render(w, "styleguide", s.base(r, "styleguide", ""))
}
