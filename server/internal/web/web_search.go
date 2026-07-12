package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/store"
)

// searchLimit caps how many hits either corpus returns to one search page.
const searchLimit = 100

// msgHit pairs a matched message with its board's id and display name, so the
// results list can link to and label the base without re-querying.
type msgHit struct {
	Msg       store.Message
	BoardID   int64
	BoardName string
}

// search renders board-wide search: one query box over both corpora, results
// split into message hits and file hits. Everything is ACS-scoped to the
// viewer -- only the bases and areas they may open are searched -- so an
// anonymous or low-SL caller can never surface restricted content. Owned by
// the search feature (sysop-toggleable).
func (s *server) search(w http.ResponseWriter, r *http.Request) {
	base := s.base(r, "search", "search")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	subj := acsSubjectOf(base.User)

	var msgs []msgHit
	var files []fileDL
	searched := query != ""

	if searched {
		// Message hits, scoped to the boards this viewer can read.
		boardName := map[int64]string{}
		var boardIDs []int64
		if bs, err := s.st.Boards(); err != nil {
			log.Printf("web: search Boards: %v", err)
		} else {
			for i := range bs {
				if acs.Eval(bs[i].ReadACS, subj) {
					boardIDs = append(boardIDs, bs[i].ID)
					boardName[bs[i].ID] = bs[i].Name
				}
			}
		}
		if hits, err := s.st.SearchMessages(query, boardIDs, searchLimit); err != nil {
			log.Printf("web: SearchMessages: %v", err)
		} else {
			for _, m := range hits {
				msgs = append(msgs, msgHit{Msg: m, BoardID: m.BoardID, BoardName: boardName[m.BoardID]})
			}
		}

		// File hits, scoped to the areas this viewer can access, each with a
		// freshly signed link (blanked when ratio blocks the download).
		var areaIDs []int64
		if as, err := s.st.FileAreas(); err != nil {
			log.Printf("web: search FileAreas: %v", err)
		} else {
			for i := range as {
				if acs.Eval(as[i].ACS, subj) {
					areaIDs = append(areaIDs, as[i].ID)
				}
			}
		}
		if hits, err := s.st.SearchFiles(query, areaIDs, searchLimit); err != nil {
			log.Printf("web: SearchFiles: %v", err)
		} else {
			var uid int64
			if base.User != nil {
				uid = base.User.ID
			}
			for _, e := range hits {
				dl := fileDL{File: e, URL: s.signDownload(e.ID, uid, downloadTTL)}
				if base.User != nil && s.st.RatioBlocks(base.User, e.Size) {
					dl.URL = ""
				}
				files = append(files, dl)
			}
		}
	}

	s.render(w, "search", struct {
		pageData
		Query    string
		Searched bool
		Messages []msgHit
		Files    []fileDL
	}{base, query, searched, msgs, files})
}
