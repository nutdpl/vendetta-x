package web

import (
	"log"
	"net/http"
	"strings"
	"time"

	"vendetta-x/server/internal/social"
	"vendetta-x/server/internal/store"
)

// home renders the dashboard: live stats, recent activity, the oneliner wall,
// and the social leaderboards. Owned by the home/dashboard feature.
func (s *server) home(w http.ResponseWriter, r *http.Request) {
	base := s.base(r, "home", "home")

	recent, err := s.st.RecentMessages(10)
	if err != nil {
		log.Printf("web: RecentMessages: %v", err)
	}
	ones, err := s.st.Oneliners(10)
	if err != nil {
		log.Printf("web: Oneliners: %v", err)
	}
	users, err := s.st.Users()
	if err != nil {
		log.Printf("web: Users: %v", err)
	}

	// stat tiles -----------------------------------------------------------
	// total messages: limit <= 0 returns the whole table.
	totalMsgs := 0
	if all, err := s.st.RecentMessages(0); err != nil {
		log.Printf("web: RecentMessages(0): %v", err)
	} else {
		totalMsgs = len(all)
	}

	// total files: sum of the file count across every area.
	totalFiles := 0
	if areas, err := s.st.FileAreas(); err != nil {
		log.Printf("web: FileAreas: %v", err)
	} else {
		for _, a := range areas {
			if fs, err := s.st.Files(a.ID); err != nil {
				log.Printf("web: Files(%d): %v", a.ID, err)
			} else {
				totalFiles += len(fs)
			}
		}
	}

	// leaderboards + caller lists over the user base.
	leaders := social.Rank(users, 5)
	lastCallers := social.LastCallers(users, 5)

	// topPosts is the denominator for the leaderboard ratio bars.
	topPosts := 0
	if len(leaders.TopPosters) > 0 {
		topPosts = leaders.TopPosters[0].Posts
	}
	topCalls := 0
	if len(leaders.TopCallers) > 0 {
		topCalls = leaders.TopCallers[0].Calls
	}

	s.render(w, "home", struct {
		pageData
		Recent      []store.Message
		Oneliners   []store.Oneliner
		TotalUsers  int
		TotalMsgs   int
		TotalFiles  int
		NodesOnline int
		TopPosters  []store.User
		TopCallers  []store.User
		NewestUsers []store.User
		LastCallers []store.User
		TopPosts    int
		TopCalls    int
	}{
		pageData:    base,
		Recent:      recent,
		Oneliners:   ones,
		TotalUsers:  len(users),
		TotalMsgs:   totalMsgs,
		TotalFiles:  totalFiles,
		NodesOnline: len(base.Online),
		TopPosters:  leaders.TopPosters,
		TopCallers:  leaders.TopCallers,
		NewestUsers: leaders.NewestUsers,
		LastCallers: lastCallers,
		TopPosts:    topPosts,
		TopCalls:    topCalls,
	})
}

// postOneliner appends to the wall and bounces back to the home wall anchor.
// It requires a logged-in caller and attributes the entry to that account --
// the wall is no longer an anonymous, spoofable endpoint where a form field
// chose the author (which let anyone post as the sysop).
func (s *server) postOneliner(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err == nil {
		text := strings.TrimSpace(r.FormValue("text"))
		if text != "" {
			if err := s.st.AddOneliner(&store.Oneliner{
				Author: u.Handle, // attributed to the authenticated caller, not a form field
				Text:   text,
				Posted: time.Now(),
			}); err != nil {
				log.Printf("web: AddOneliner: %v", err)
			}
		}
	}
	http.Redirect(w, r, "/#wall", http.StatusSeeOther)
}
