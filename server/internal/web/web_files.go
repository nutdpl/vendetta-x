package web

import (
	"io"
	"log"
	"net/http"
	"path"
	"strconv"

	"vendetta-x/server/internal/acs"
	"vendetta-x/server/internal/store"
	"vendetta-x/server/internal/upload"
)

// maxUploadBytes caps a single uploaded file. Content is stored in SQLite, so
// keep it modest.
const maxUploadBytes = 5 << 20 // 5 MiB

// areaTab pairs a file area with its file count, so the tab bar can show a
// per-area count chip without the template re-querying the store.
type areaTab struct {
	Area  store.FileArea
	Count int
}

// fileDL pairs a file entry with a freshly signed, time-limited download URL.
type fileDL struct {
	File store.FileEntry
	URL  string
}

// files renders the file areas and the selected area's listing. Owned by the
// files feature.
func (s *server) files(w http.ResponseWriter, r *http.Request) {
	base := s.base(r, "files", "files")

	areas, err := s.st.FileAreas()
	if err != nil {
		log.Printf("web: FileAreas: %v", err)
	}

	// Build a tab per area, each carrying its file count for the count chip.
	tabs := make([]areaTab, 0, len(areas))
	for i := range areas {
		fs, err := s.st.Files(areas[i].ID)
		if err != nil {
			log.Printf("web: Files (count) area %d: %v", areas[i].ID, err)
		}
		tabs = append(tabs, areaTab{Area: areas[i], Count: len(fs)})
	}

	var selected *store.FileArea
	var files []fileDL
	if raw := r.URL.Query().Get("area"); raw != "" {
		if aid, ok := parseID(raw); ok {
			for i := range areas {
				if areas[i].ID == aid {
					selected = &areas[i]
					break
				}
			}
			if selected != nil {
				entries, err := s.st.Files(aid)
				if err != nil {
					log.Printf("web: Files: %v", err)
				}
				var uid int64
				if base.User != nil {
					uid = base.User.ID
				}
				for _, e := range entries {
					dl := fileDL{File: e, URL: s.signDownload(e.ID, uid, downloadTTL)}
					// Ratio gate: if a logged-in caller lacks the credit for
					// this file, offer no working link (the URL is blanked and
					// the template shows it as locked).
					if base.User != nil && s.st.RatioBlocks(base.User, e.Size) {
						dl.URL = ""
					}
					files = append(files, dl)
				}
			}
		}
	}

	// Upload is allowed when an area is selected, the caller is logged in, and
	// they satisfy the area's ACS.
	canUpload := selected != nil && base.User != nil &&
		acs.Eval(selected.ACS, acsSubjectOf(base.User))

	s.render(w, "files", struct {
		pageData
		Tabs      []areaTab
		Areas     []store.FileArea
		Selected  *store.FileArea
		Files     []fileDL
		CanUpload bool
		MaxMB     int
	}{base, tabs, areas, selected, files, canUpload, maxUploadBytes >> 20})
}

// uploadFile accepts a multipart file upload into an area. It requires a logged
// in user who satisfies the area's ACS, caps the size, and stores the bytes.
func (s *server) uploadFile(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	dest := "/files"
	if ok {
		dest = "/files?area=" + strconv.FormatInt(id, 10)
	}

	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+dest, http.StatusSeeOther)
		return
	}
	if !ok {
		http.Redirect(w, r, "/files", http.StatusSeeOther)
		return
	}
	area := s.areaByID(id)
	if area == nil || !acs.Eval(area.ACS, acsSubjectOf(u)) {
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	// Cap the whole request, then parse the multipart form.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+4096)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil || len(content) == 0 || len(content) > maxUploadBytes {
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	filename := safeFilename(path.Base(header.Filename))

	// Same intake as the terminal face: refuse exact duplicates, let a ZIP's
	// FILE_ID.DIZ describe the release, and honor the moderation queue.
	if dupe, err := s.st.FileByHash(upload.Hash(content)); err == nil && dupe != nil {
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	desc := upload.Describe(content, r.FormValue("description"))

	if s.st.SettingBool("files.moderate", false) && !s.st.RatioExempt(u) {
		if _, err := s.st.AddPendingFile(id, filename, desc, u.Handle, content); err != nil {
			log.Printf("web: AddPendingFile: %v", err)
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	if _, err := s.st.AddFile(id, filename, desc, u.Handle, content); err != nil {
		log.Printf("web: AddFile: %v", err)
	} else if err := s.st.AddUploadBytes(u.ID, int64(len(content))); err != nil {
		log.Printf("web: AddUploadBytes: %v", err)
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
