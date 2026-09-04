// Package kanban implements the GoatFlow Kanban board plugin.
//
// The kanban plugin provides drag-and-drop ticket boards over real ticket
// states: columns are ticket states, dragging a card changes the ticket's
// state through the platform's ChangeTicketStatus API (so pending due-time
// semantics stay in one place), and boards are scoped to the caller's queues.
package kanban

import (
	"context"
	"encoding/json"
	"fmt"

	plugin "github.com/goatkit/goatflow/pkg/plugin"
)

// Plugin implements the GoatFlow Kanban plugin as a gRPC plugin.
type Plugin struct {
	host   plugin.HostAPI
	d      dialect
	access *AccessService
}

// New returns a new kanban plugin instance.
func New() *Plugin {
	return &Plugin{}
}

// GKRegister returns the plugin self-description, including the board UI.
func (p *Plugin) GKRegister() (*plugin.GKRegistration, error) {
	return &plugin.GKRegistration{
		Name:        "goat-kanban",
		Version:     "0.1.0",
		Description: "Kanban boards over ticket states with drag-and-drop, palette, and queue-scoped access",
		Author:      "GoatKit Team",
		License:     "Apache-2.0",
		Homepage:    "https://github.com/goatkit/goat-kanban",
		Icon:        "https://raw.githubusercontent.com/goatkit/goat-kanban/main/icon.svg",

		UIs: []plugin.UISpec{
			{
				ID:          "board",
				Name:        "Kanban",
				Description: "Drag-and-drop ticket boards over real ticket states",
				Type:        "agent_app",
				Icon:        "fa-table-columns",
				Shell:       "standard",
				Routes: []plugin.UIRouteSpec{
					{Path: "/", Method: "GET", Handler: "render_boards"},
					{Path: "/board/:id", Method: "GET", Handler: "render_board"},
					{Path: "/api/palette", Method: "GET", Handler: "api_palette"},
					{Path: "/api/move", Method: "POST", Handler: "api_move"},
					{Path: "/api/board/create", Method: "POST", Handler: "api_board_create"},
					{Path: "/api/board/delete", Method: "POST", Handler: "api_board_delete"},
					{Path: "/api/board/config", Method: "POST", Handler: "api_board_config"},
					{Path: "/api/board/add", Method: "POST", Handler: "api_board_add"},
				},
			},
		},
		MenuItems: []plugin.MenuItemSpec{
			{
				ID:       "kanban-board",
				Label:    "Kanban Board",
				Icon:     "fa-table-columns",
				Path:     "/ui/goat-kanban_board/",
				Location: "agent",
				Order:    40,
			},
		},

		I18n: &plugin.I18nSpec{
			Namespace:    "goat-kanban",
			Languages:    []string{"en", "de"},
			Translations: map[string]map[string]string{"en": i18nEN, "de": i18nDE},
		},

		ErrorCodes: []plugin.ErrorCodeSpec{
			{Code: "board_not_found", Message: "Board not found", HTTPStatus: 404},
			{Code: "no_access", Message: "You do not have access to this board or queue", HTTPStatus: 403},
			{Code: "pending_time_required", Message: "Pending time is required for pending states", HTTPStatus: 400},
			{Code: "invalid_request", Message: "Invalid request", HTTPStatus: 400},
		},

		// Must mirror plugin.yaml resources.permissions. The sandbox enforces
		// the FIRST db entry's scope per query; states come from the HostAPI
		// ListTicketStates method, not raw SQL, so no ticket_state* tables.
		Resources: &plugin.ResourceRequest{
			MemoryMB:        256,
			CallTimeout:     "30s",
			InitTimeout:     "10s",
			ShutdownTimeout: "5s",
			Permissions: []plugin.Permission{
				{Type: "db", Access: "readwrite", Scope: []string{"gk_kanban_*", "ticket", "queue", "groups", "group_user", "role_user", "roles", "group_role", "users", "customer_user", "customer_company", "ticket_priority"}},
			},
		},
	}, nil
}

// InitWithHost receives the HostAPI from the platform and brings the kanban
// schema up to the current version before the plugin serves requests.
func (p *Plugin) InitWithHost(config map[string]string, host plugin.HostAPI) error {
	p.host = host
	p.d = detectDialect(context.Background(), host)
	p.access = NewAccessService(host, p.d)
	host.Log(context.Background(), "info", "kanban plugin initialized", map[string]any{"dialect": string(p.d)})
	if err := migrateSchema(context.Background(), host, p.d); err != nil {
		return fmt.Errorf("kanban plugin init: %w", err)
	}
	return nil
}

// Init is the fallback for hosts that don't provide HostAPI.
func (p *Plugin) Init(config map[string]string) error {
	return fmt.Errorf("kanban plugin requires HostAPI — host must support GKPluginWithHost")
}

// Call dispatches handler calls from the host.
func (p *Plugin) Call(fn string, args json.RawMessage) (json.RawMessage, error) {
	ctx := context.Background()
	switch fn {
	case "render_boards":
		return p.handleBoards(ctx, args)
	case "render_board":
		return p.handleBoard(ctx, args)
	case "api_palette":
		return p.handlePalette(ctx, args)
	case "api_move":
		return p.handleMove(ctx, args)
	case "api_board_create":
		return p.handleBoardCreate(ctx, args)
	case "api_board_delete":
		return p.handleBoardDelete(ctx, args)
	case "api_board_config":
		return p.handleBoardConfig(ctx, args)
	case "api_board_add":
		return p.handleBoardAdd(ctx, args)
	default:
		return nil, fmt.Errorf("unknown function: %s", fn)
	}
}

// Shutdown cleans up plugin resources.
func (p *Plugin) Shutdown() error {
	if p.host != nil {
		p.host.Log(context.Background(), "info", "kanban plugin shut down", nil)
	}
	return nil
}
