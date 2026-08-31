package kanban

// i18n: English and German strings for the kanban UI, declared in GKRegister
// (I18n.Translations) so the host can serve them per locale. Keys are looked
// up via t(lang, key) on the Go side and embedded into the generated HTML/JS;
// server-side text is English by default (the shell header shows the user's
// login, not a locale switcher).

var i18nEN = map[string]string{
	"title":               "Kanban",
	"boards_subtitle":     "Boards over your queues. Drag cards to change ticket states.",
	"no_boards":           "No boards yet — create one below.",
	"scope_all":           "All queues",
	"board_name_ph":       "Board name…",
	"new_board":           "New board",
	"create":              "Create",
	"cancel":              "Cancel",
	"save":                "Save",
	"confirm":             "Confirm",
	"back":                "Boards",
	"config":              "Configure",
	"config_title":        "Board settings",
	"ticket_view_label":   "Ticket view",
	"view_standard":       "Standard ticket page",
	"add_tickets":         "Add tickets",
	"search_ph":           "Search by number or title…",
	"no_results":          "No matching tickets",
	"unassigned":          "unassigned",
	"pending_title":       "Pending state",
	"pending_body":        "This state is pending — set the due time.",
	"delete_confirm":      "Delete this board and its membership? Tickets themselves are not touched.",
	"board_name_required": "Board name is required",
	"err_generic":         "Something went wrong",
	"err_invalid_request": "Invalid request",
	"err_not_found":       "Not found",
	"err_no_access":       "You do not have access to this board or queue",
}

var i18nDE = map[string]string{
	"title":               "Kanban",
	"boards_subtitle":     "Boards über Ihre Warteschlangen. Karten ziehen, um Ticketstatus zu ändern.",
	"no_boards":           "Noch keine Boards — unten eines erstellen.",
	"scope_all":           "Alle Warteschlangen",
	"board_name_ph":       "Board-Name…",
	"new_board":           "Neues Board",
	"create":              "Erstellen",
	"cancel":              "Abbrechen",
	"save":                "Speichern",
	"confirm":             "Bestätigen",
	"back":                "Boards",
	"config":              "Konfigurieren",
	"config_title":        "Board-Einstellungen",
	"ticket_view_label":   "Ticket-Ansicht",
	"view_standard":       "Standard-Ticketseite",
	"add_tickets":         "Tickets hinzufügen",
	"search_ph":           "Nach Nummer oder Titel suchen…",
	"no_results":          "Keine passenden Tickets",
	"unassigned":          "nicht zugewiesen",
	"pending_title":       "Ausstehender Status",
	"pending_body":        "Dieser Status ist ausstehend — Fälligkeitszeit setzen.",
	"delete_confirm":      "Dieses Board und seine Zuordnungen löschen? Die Tickets selbst bleiben erhalten.",
	"board_name_required": "Board-Name wird benötigt",
	"err_generic":         "Etwas ist schiefgelaufen",
	"err_invalid_request": "Ungültige Anfrage",
	"err_not_found":       "Nicht gefunden",
	"err_no_access":       "Sie haben keinen Zugriff auf dieses Board oder diese Warteschlange",
}

// t returns the translation for lang (falls back to English).
func t(lang, key string) string {
	m := i18nEN
	if lang == "de" {
		m = i18nDE
	}
	if s, ok := m[key]; ok && s != "" {
		return s
	}
	return key
}
