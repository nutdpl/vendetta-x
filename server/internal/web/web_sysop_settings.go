package web

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"vendetta-x/server/internal/store"
)

// Global, board-wide configuration: identity strings, new-user defaults, and
// the per-feature on/off toggles. Backed by the settings key/value table.

// featureToggle is one row in the settings screen's feature list.
type featureToggle struct {
	store.Feature
	On bool
}

// sysopSettings renders the global settings form: board identity, new-user
// defaults, and a checkbox per toggleable feature.
func (s *server) sysopSettings(w http.ResponseWriter, r *http.Request) {
	toggles := make([]featureToggle, 0, len(store.Features))
	for _, f := range store.Features {
		toggles = append(toggles, featureToggle{Feature: f, On: s.st.FeatureEnabled(f.Key)})
	}
	s.render(w, "sysop_settings", struct {
		pageData
		BoardName     string
		Tagline       string
		Sysop         string
		NewUserSL     int
		NewUserDSL    int
		NewUserGroup  string
		RatioOn       bool
		RatioBytes    int
		RatioFreeMB   int
		Moderate      bool
		PurgeDays     int
		OnelinerKeep  int
		FeatureToggle []featureToggle
		Saved         bool
	}{
		pageData:      s.base(r, "sysop / settings", "settings"),
		BoardName:     s.st.Setting("board.name", s.cfg.BoardName),
		Tagline:       s.st.Setting("board.tagline", "this is not a bbs"),
		Sysop:         s.st.Setting("board.sysop", "SysOp"),
		NewUserSL:     s.st.SettingInt("newuser.sl", 10),
		NewUserDSL:    s.st.SettingInt("newuser.dsl", 10),
		NewUserGroup:  s.st.Setting("newuser.group", "Users"),
		RatioOn:       s.st.RatioEnabled(),
		RatioBytes:    s.st.RatioBytes(),
		RatioFreeMB:   int(s.st.RatioFreeBytes() >> 20),
		Moderate:      s.st.SettingBool("files.moderate", false),
		PurgeDays:     s.st.SettingInt("schedule.messages.purge_days", 180),
		OnelinerKeep:  s.st.SettingInt("schedule.oneliners.keep", 200),
		FeatureToggle: toggles,
		Saved:         r.URL.Query().Get("ok") != "",
	})
}

// sysopSettingsSave persists every settings field, then redirects back with a
// success flag. Feature checkboxes are stored "1"/"0" (absent box = off).
func (s *server) sysopSettingsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/settings", http.StatusSeeOther)
		return
	}
	set := func(key, val string) {
		if err := s.st.SetSetting(key, val); err != nil {
			log.Printf("web: sysop set %s: %v", key, err)
		}
	}
	set("board.name", strings.TrimSpace(r.FormValue("board_name")))
	set("board.tagline", strings.TrimSpace(r.FormValue("tagline")))
	set("board.sysop", strings.TrimSpace(r.FormValue("board_sysop")))
	set("newuser.sl", strconv.Itoa(clampLevel(r.FormValue("newuser_sl"), 10)))
	set("newuser.dsl", strconv.Itoa(clampLevel(r.FormValue("newuser_dsl"), 10)))
	set("newuser.group", strings.TrimSpace(r.FormValue("newuser_group")))
	// File ratio economy: on/off + the ratio and free allowance.
	ratioOn := "0"
	if r.FormValue("ratio_enabled") != "" {
		ratioOn = "1"
	}
	set("files.ratio.enabled", ratioOn)
	set("files.ratio.bytes", strconv.Itoa(clampRange(r.FormValue("ratio_bytes"), 3, 1, 100)))
	set("files.ratio.free_mb", strconv.Itoa(clampRange(r.FormValue("ratio_free_mb"), 5, 0, 100000)))
	// Upload moderation: hold new uploads in the review queue until approved.
	moderate := "0"
	if r.FormValue("files_moderate") != "" {
		moderate = "1"
	}
	set("files.moderate", moderate)
	set("schedule.messages.purge_days", strconv.Itoa(clampRange(r.FormValue("purge_days"), 180, 1, 3650)))
	set("schedule.oneliners.keep", strconv.Itoa(clampRange(r.FormValue("oneliner_keep"), 200, 1, 100000)))
	// Each feature: a present checkbox (value "1") means on, absent means off.
	for _, f := range store.Features {
		on := "0"
		if r.FormValue("feature_"+f.Key) != "" {
			on = "1"
		}
		set("feature."+f.Key, on)
	}
	http.Redirect(w, r, "/sysop/settings?ok=1", http.StatusSeeOther)
}
