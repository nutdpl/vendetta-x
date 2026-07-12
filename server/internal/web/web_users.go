package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/badges"
	"vendetta-x/server/internal/store"
)

// users renders the user directory. Owned by the users feature.
func (s *server) users(w http.ResponseWriter, r *http.Request) {
	us, err := s.st.Users()
	if err != nil {
		log.Printf("web: Users: %v", err)
	}
	s.render(w, "users", struct {
		pageData
		Users []store.User
	}{s.base(r, "users", "users"), us})
}

// userProfile renders a single user's profile page. It loads the user by
// handle (case-insensitive); an unknown handle redirects to /users rather than
// 500-ing. It also gathers the user's recent posts by scanning the global
// message feed for messages whose From matches the handle.
func (s *server) userProfile(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	u, err := s.st.UserByHandle(handle)
	if err != nil {
		log.Printf("web: UserByHandle: %v", err)
	}
	if u == nil {
		http.Redirect(w, r, "/users", http.StatusSeeOther)
		return
	}

	// Gather this user's recent posts (newest-first), capped at 10.
	const maxPosts = 10
	var posts []store.Message
	msgs, err := s.st.RecentMessages(0)
	if err != nil {
		log.Printf("web: RecentMessages: %v", err)
	}
	for i := range msgs {
		if strings.EqualFold(strings.TrimSpace(msgs[i].From), u.Handle) {
			posts = append(posts, msgs[i])
			if len(posts) >= maxPosts {
				break
			}
		}
	}

	// Split the ACS flag string ("AC") into individual flags so the template
	// can render each as its own badge (html/template can't range a string).
	var flags []string
	for _, r := range u.Flags {
		if s := strings.TrimSpace(string(r)); s != "" {
			flags = append(flags, s)
		}
	}

	s.render(w, "profile", struct {
		pageData
		User   store.User
		Flags  []string
		Posts  []store.Message
		Badges []badges.Badge
	}{s.base(r, u.Handle, "users"), *u, flags, posts, badges.Earned(*u)})
}
