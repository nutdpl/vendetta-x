package web

import (
	"log"
	"net/http"
	"strings"

	"vendetta-x/server/internal/store"
)

// The upload review queue: when files.moderate is on, caller uploads wait
// here until the sysop approves (file goes live, uploader gets their ratio
// credit and a mail) or rejects (file deleted, uploader mailed the reason).

// pendingUpload pairs a queued file with its area name for the listing.
type pendingUpload struct {
	File store.FileEntry
	Area string
}

func (s *server) sysopUploads(w http.ResponseWriter, r *http.Request) {
	pending, err := s.st.PendingFiles()
	if err != nil {
		log.Printf("web: PendingFiles: %v", err)
	}
	areaName := map[int64]string{}
	if areas, err := s.st.FileAreas(); err == nil {
		for _, a := range areas {
			areaName[a.ID] = a.Name
		}
	}
	rows := make([]pendingUpload, 0, len(pending))
	for _, f := range pending {
		rows = append(rows, pendingUpload{File: f, Area: areaName[f.AreaID]})
	}
	s.render(w, "sysop_uploads", struct {
		pageData
		Pending  []pendingUpload
		Moderate bool
	}{s.base(r, "sysop / uploads", "uploads"), rows, s.st.SettingBool("files.moderate", false)})
}

// sysopUploadApprove releases a queued file: visible in its area, ratio
// credit lands, uploader gets the good news by mail.
func (s *server) sysopUploadApprove(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/uploads", http.StatusSeeOther)
		return
	}
	f, err := s.st.FileByID(id)
	if err != nil || f == nil || f.Approved {
		http.Redirect(w, r, "/sysop/uploads", http.StatusSeeOther)
		return
	}
	if err := s.st.ApproveFile(id); err != nil {
		log.Printf("web: ApproveFile: %v", err)
		http.Redirect(w, r, "/sysop/uploads", http.StatusSeeOther)
		return
	}
	if u, err := s.st.UserByHandle(f.Uploader); err == nil && u != nil {
		if err := s.st.AddUploadBytes(u.ID, f.Size); err != nil {
			log.Printf("web: AddUploadBytes: %v", err)
		}
	}
	admin := s.currentUser(r)
	from := "sysop"
	if admin != nil {
		from = admin.Handle
	}
	if err := s.mail.Send(from, f.Uploader, "Upload approved: "+f.Filename,
		"Your upload "+f.Filename+" is live in the file areas. Upload credit is on your account. Thanks for the contribution."); err != nil {
		log.Printf("web: approve mail: %v", err)
	}
	http.Redirect(w, r, "/sysop/uploads", http.StatusSeeOther)
}

// sysopUploadReject deletes a queued file and mails the uploader why.
func (s *server) sysopUploadReject(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		http.Redirect(w, r, "/sysop/uploads", http.StatusSeeOther)
		return
	}
	f, err := s.st.FileByID(id)
	if err != nil || f == nil || f.Approved {
		http.Redirect(w, r, "/sysop/uploads", http.StatusSeeOther)
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		reason = "no reason given"
	}
	if err := s.st.DeleteFile(id); err != nil {
		log.Printf("web: DeleteFile: %v", err)
		http.Redirect(w, r, "/sysop/uploads", http.StatusSeeOther)
		return
	}
	admin := s.currentUser(r)
	from := "sysop"
	if admin != nil {
		from = admin.Handle
	}
	if err := s.mail.Send(from, f.Uploader, "Upload rejected: "+f.Filename,
		"Your upload "+f.Filename+" was not accepted: "+reason); err != nil {
		log.Printf("web: reject mail: %v", err)
	}
	http.Redirect(w, r, "/sysop/uploads", http.StatusSeeOther)
}
