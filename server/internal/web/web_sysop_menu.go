package web

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"vendetta-x/server/internal/menu"
)

// The main menu editor: which command lives in each on-screen telnet/ssh
// menu slot, its label, and the key that picks it. The physical slots are
// fixed (server/server_menu.go's mainMenuSlotPos); this page edits the
// binding stored in each one. See internal/menu for the data model.

// menuRow is one editable slot, ready for the template.
type menuRow struct {
	Slot    string
	Display string
	Action  string
	Label   string
	Hotkey  string
	Enabled bool
}

func (s *server) sysopMenu(w http.ResponseWriter, r *http.Request) {
	items, err := s.menu.Items("main")
	if err != nil {
		log.Printf("web: menu items: %v", err)
	}
	rows := make([]menuRow, 0, len(menu.MainMenuSlots))
	for _, slot := range menu.MainMenuSlots {
		it := items[slot]
		rows = append(rows, menuRow{
			Slot: slot, Display: slotDisplay(slot),
			Action: it.Action, Label: it.Label, Hotkey: it.Hotkey, Enabled: it.Enabled,
		})
	}
	s.render(w, "sysop_menu", struct {
		pageData
		Rows    []menuRow
		Catalog []menu.ActionDef
		Err     string
	}{s.base(r, "sysop / main menu", "menu"), rows, menu.Catalog, r.URL.Query().Get("err")})
}

// sysopMenuSave replaces the whole main-menu binding set: the form always
// submits every fixed slot at once (see internal/menu.Store.Save).
func (s *server) sysopMenuSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/sysop/menu", http.StatusSeeOther)
		return
	}
	items := make([]menu.Item, 0, len(menu.MainMenuSlots))
	for _, slot := range menu.MainMenuSlots {
		items = append(items, menu.Item{
			Slot:    slot,
			Action:  r.FormValue("action_" + slot),
			Label:   r.FormValue("label_" + slot),
			Hotkey:  r.FormValue("hotkey_" + slot),
			Enabled: r.FormValue("enabled_"+slot) != "",
		})
	}
	if err := s.menu.Save("main", items); err != nil {
		http.Redirect(w, r, "/sysop/menu?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/sysop/menu", http.StatusSeeOther)
}

// slotDisplay turns a slot id ("L0", "R8", ...) into a sysop-friendly label
// ("Left #1", "Right #9").
func slotDisplay(slot string) string {
	side := "Left"
	if strings.HasPrefix(slot, "R") {
		side = "Right"
	}
	n, _ := strconv.Atoi(slot[1:])
	return fmt.Sprintf("%s #%d", side, n+1)
}
