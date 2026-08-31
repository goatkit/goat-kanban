package kanban

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	plugin "github.com/goatkit/goatflow/pkg/plugin"
	grpcutil "github.com/goatkit/goatflow/pkg/plugin/grpcutil"
)

// Compile-time: the plugin must satisfy the gRPC host-API contract.
var _ grpcutil.GKPluginWithHost = (*Plugin)(nil)

// fakeHost implements plugin.HostAPI for tests. DBQuery/DBExec route through
// pluggable functions so each test scripts its own rows; everything else is a
// harmless stub matching the interface exactly.
type fakeHost struct {
	mu       sync.Mutex
	queries  []string
	execs    []string
	queryFn  func(query string, args ...any) ([]map[string]any, error)
	execFn   func(query string, args ...any) (int64, error)
	changeFn func(ticketID, stateID, userID, untilTime int64) error
	states   []plugin.TicketStateInfo
}

func (f *fakeHost) DBQuery(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	f.mu.Lock()
	f.queries = append(f.queries, query)
	fn := f.queryFn
	f.mu.Unlock()
	if fn != nil {
		return fn(query, args...)
	}
	return routeQuery(query, args...)
}

func (f *fakeHost) DBExec(ctx context.Context, query string, args ...any) (int64, error) {
	f.mu.Lock()
	f.execs = append(f.execs, query)
	fn := f.execFn
	f.mu.Unlock()
	if fn != nil {
		return fn(query, args...)
	}
	return 1, nil
}

func (f *fakeHost) ChangeTicketStatus(ctx context.Context, ticketID, stateID, userID int64, untilTime int64) error {
	if f.changeFn != nil {
		return f.changeFn(ticketID, stateID, userID, untilTime)
	}
	return nil
}

func (f *fakeHost) ListTicketStates(ctx context.Context) ([]plugin.TicketStateInfo, error) {
	return f.states, nil
}

// routeQuery returns canned rows for the queries the plugin issues, keyed on
// table substrings. Tests that need specific shapes set queryFn instead.
func routeQuery(query string, args ...any) ([]map[string]any, error) {
	q := strings.ToLower(query)
	switch {
	case strings.Contains(q, "gk_organisation"):
		return []map[string]any{{"v": 1}}, nil
	case strings.Contains(q, "group_user"), strings.Contains(q, "role_user"):
		return []map[string]any{}, nil
	case strings.Contains(q, "from queue q"):
		return []map[string]any{}, nil
	case strings.Contains(q, "max(version)"):
		return []map[string]any{{"v": 0}}, nil
	}
	return nil, nil
}

// --- stub methods (interface completeness) ---

func (f *fakeHost) CacheGet(ctx context.Context, key string) ([]byte, bool, error) {
	return nil, false, nil
}
func (f *fakeHost) CacheSet(ctx context.Context, key string, value []byte, ttlSeconds int) error {
	return nil
}
func (f *fakeHost) CacheDelete(ctx context.Context, key string) error { return nil }
func (f *fakeHost) HTTPRequest(ctx context.Context, method, url string, headers map[string]string, body []byte) (int, []byte, error) {
	return 0, nil, nil
}
func (f *fakeHost) SendEmail(ctx context.Context, to, subject, body string, html bool) error {
	return nil
}
func (f *fakeHost) Log(ctx context.Context, level, message string, fields map[string]any) {}
func (f *fakeHost) ConfigGet(ctx context.Context, key string) (string, error)             { return "", nil }

func (f *fakeHost) Translate(ctx context.Context, key string, args ...any) string {
	return "goat-kanban." + key
}

func (f *fakeHost) CallPlugin(ctx context.Context, pluginName, fn string, args json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeHost) PublishEvent(ctx context.Context, channel string, eventType string, data string) error {
	return nil
}
func (f *fakeHost) EntitySoftDelete(ctx context.Context, entityType string, entityID int64, reason string) error {
	return nil
}
func (f *fakeHost) EntityRestore(ctx context.Context, entityType string, entityID int64) error {
	return nil
}
func (f *fakeHost) EntityHardDelete(ctx context.Context, entityType string, entityID int64, reason string) error {
	return nil
}
func (f *fakeHost) RecycleBinList(ctx context.Context, entityType string) (json.RawMessage, error) {
	return nil, nil
}
func (f *fakeHost) SecureConfigGet(ctx context.Context, key string) (string, error) { return "", nil }
func (f *fakeHost) SecureConfigSet(ctx context.Context, key, value string) error    { return nil }
func (f *fakeHost) OrgID(ctx context.Context) int64                                 { return 0 }

func (f *fakeHost) CustomFieldsGet(ctx context.Context, entityType string, objectID int64, fields []string) (map[string]any, error) {
	return nil, nil
}
func (f *fakeHost) CustomFieldsSet(ctx context.Context, entityType string, objectID int64, values map[string]any) error {
	return nil
}
func (f *fakeHost) CustomFieldsQuery(ctx context.Context, entityType string, filters []plugin.CustomFieldFilter) ([]int64, error) {
	return nil, nil
}

func (f *fakeHost) StoreFile(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	return nil
}
func (f *fakeHost) GetFile(ctx context.Context, key string) ([]byte, map[string]string, error) {
	return nil, nil, nil
}
func (f *fakeHost) DeleteFile(ctx context.Context, key string) error { return nil }
func (f *fakeHost) ListFiles(ctx context.Context, prefix string) ([]plugin.FileInfo, error) {
	return nil, nil
}
func (f *fakeHost) GenerateThumbnail(ctx context.Context, data []byte, contentType string, maxWidth, maxHeight int) ([]byte, string, error) {
	return nil, "", nil
}

func (f *fakeHost) CreateArticleAttachment(ctx context.Context, articleID, createdBy int64, filename, contentType string, content []byte) (int64, error) {
	return 0, nil
}
func (f *fakeHost) ListArticleAttachments(ctx context.Context, articleID int64) ([]plugin.ArticleAttachment, error) {
	return nil, nil
}
func (f *fakeHost) DeleteArticleAttachment(ctx context.Context, articleID, attachmentID int64) error {
	return nil
}
func (f *fakeHost) CreateArticle(ctx context.Context, ticketID, createdBy int64, subject, body string, visibleToCustomer bool) (int64, error) {
	return 0, nil
}
func (f *fakeHost) RenderMarkdownToPdf(ctx context.Context, markdown string, options plugin.PdfRenderOptions) ([]byte, error) {
	return nil, nil
}

// TestGKRegisterContract pins the self-description: identity, UI routes, i18n
// parity and the db permission scope (it must mirror plugin.yaml; the sandbox
// enforces it per query).
func TestGKRegisterContract(t *testing.T) {
	reg, err := New().GKRegister()
	if err != nil {
		t.Fatalf("GKRegister: %v", err)
	}
	if reg.Name != "goat-kanban" || reg.Version != "0.1.0" {
		t.Fatalf("identity = %s %s, want goat-kanban 0.1.0", reg.Name, reg.Version)
	}

	if len(reg.UIs) != 1 {
		t.Fatalf("expected exactly 1 UI, got %d", len(reg.UIs))
	}
	ui := reg.UIs[0]
	if ui.ID != "board" || ui.Type != "agent_app" || ui.Shell != "minimal" {
		t.Fatalf("UI spec = %+v", ui)
	}
	wantRoutes := map[string]string{
		"/":                 "GET",
		"/board/:id":        "GET",
		"/api/palette":      "GET",
		"/api/move":         "POST",
		"/api/board/create": "POST",
		"/api/board/delete": "POST",
		"/api/board/config": "POST",
		"/api/board/add":    "POST",
	}
	if len(ui.Routes) != len(wantRoutes) {
		t.Fatalf("expected %d routes, got %d", len(wantRoutes), len(ui.Routes))
	}
	for _, r := range ui.Routes {
		if wantRoutes[r.Path] != r.Method {
			t.Errorf("route %s: method = %s, want %s", r.Path, r.Method, wantRoutes[r.Path])
		}
		if r.Handler == "" {
			t.Errorf("route %s: empty handler", r.Path)
		}
	}

	if reg.I18n == nil || reg.I18n.Namespace != "goat-kanban" {
		t.Fatalf("I18n spec = %+v", reg.I18n)
	}
	en, de := reg.I18n.Translations["en"], reg.I18n.Translations["de"]
	if len(en) == 0 || len(de) == 0 {
		t.Fatal("translations empty")
	}
	for k := range en {
		if dv, ok := de[k]; !ok || dv == "" {
			t.Errorf("en key %q missing in de", k)
		}
	}
	for k := range de {
		if _, ok := en[k]; !ok {
			t.Errorf("de key %q missing in en", k)
		}
	}

	if reg.Resources == nil || len(reg.Resources.Permissions) != 1 {
		t.Fatalf("Resources = %+v", reg.Resources)
	}
	perm := reg.Resources.Permissions[0]
	if perm.Type != "db" || perm.Access != "readwrite" {
		t.Fatalf("permission = %+v", perm)
	}
	scope := strings.Join(perm.Scope, ",")
	for _, want := range []string{"gk_kanban_*", "ticket", "queue", "groups", "group_user", "role_user", "roles", "group_role", "users", "customer_user", "customer_company", "ticket_priority"} {
		if !strings.Contains(scope, want) {
			t.Errorf("db scope missing %q: %s", want, scope)
		}
	}
}

func TestCallUnknownFunction(t *testing.T) {
	if _, err := New().Call("nope", nil); err == nil || !strings.Contains(err.Error(), "unknown function") {
		t.Fatalf("expected unknown-function error, got %v", err)
	}
}

func TestInitRequiresHost(t *testing.T) {
	if err := New().Init(nil); err == nil {
		t.Fatal("Init without HostAPI must fail")
	}
}

// newTestPlugin wires a plugin against a fake host with the given dialect.
func newTestPlugin(host *fakeHost, d dialect) *Plugin {
	p := New()
	p.host = host
	p.d = d
	p.access = NewAccessService(host, d)
	return p
}

func statusOf(t *testing.T, out json.RawMessage) int {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("response is not a JSON object: %s", out)
	}
	s, _ := m["status"].(float64)
	return int(s)
}

// TestMovePendingWithoutTimeReturns400: core rejects pending states without a
// due time; the plugin must surface that as status 400 (not 500) so the UI can
// open the due-time dialog.
func TestMovePendingWithoutTimeReturns400(t *testing.T) {
	host := &fakeHost{queryFn: func(query string, args ...any) ([]map[string]any, error) {
		q := strings.ToLower(query)
		switch {
		case strings.Contains(q, "from gk_kanban_board"):
			return []map[string]any{{"id": int64(1)}}, nil
		case strings.Contains(q, "from ticket where"):
			return []map[string]any{{"id": int64(42), "queue_id": int64(1)}}, nil
		}
		return nil, nil
	}, changeFn: func(ticketID, stateID, userID, untilTime int64) error {
		return errors.New("pending time is required for pending states")
	}}
	p := newTestPlugin(host, dialectMySQL)

	out, err := p.Call("api_move", json.RawMessage(
		`{"_user_id":7,"_is_admin":true,"board_id":1,"ticket_id":42,"state_id":8,"until_time":0}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := statusOf(t, out); got != 400 {
		t.Fatalf("status = %d, want 400 (%s)", got, out)
	}
}

// TestMoveForeignQueueForbidden: the access check runs against the ticket's
// own queue. A non-admin with ro on group 1 (queue 1 only) must get 403 for a
// queue-2 ticket even though they can see the board.
func TestMoveForeignQueueForbidden(t *testing.T) {
	host := &fakeHost{}
	host.queryFn = func(query string, args ...any) ([]map[string]any, error) {
		q := strings.ToLower(query)
		switch {
		case strings.Contains(q, "from gk_kanban_board"):
			return []map[string]any{{"id": int64(1)}}, nil
		case strings.Contains(q, "from ticket where"):
			return []map[string]any{{"id": int64(42), "queue_id": int64(2)}}, nil
		case strings.Contains(q, "group_user"), strings.Contains(q, "role_user"):
			return []map[string]any{{"group_id": int64(1)}}, nil
		case strings.Contains(q, "from queue q"):
			return []map[string]any{{"id": int64(1)}}, nil
		}
		return nil, nil
	}
	p := newTestPlugin(host, dialectMySQL)

	out, err := p.Call("api_move", json.RawMessage(
		`{"_user_id":7,"board_id":1,"ticket_id":42,"state_id":5}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := statusOf(t, out); got != 403 {
		t.Fatalf("status = %d, want 403 (%s)", got, out)
	}
}

// TestMoveSuccessGoesThroughHostAPI: a normal move must call ChangeTicketStatus
// (never a raw ticket UPDATE) and report ok.
func TestMoveSuccessGoesThroughHostAPI(t *testing.T) {
	var calledWith struct {
		ticketID, stateID, userID, untilTime int64
	}
	host := &fakeHost{changeFn: func(ticketID, stateID, userID, untilTime int64) error {
		calledWith = struct {
			ticketID, stateID, userID, untilTime int64
		}{ticketID, stateID, userID, untilTime}
		return nil
	}}
	host.queryFn = func(query string, args ...any) ([]map[string]any, error) {
		q := strings.ToLower(query)
		switch {
		case strings.Contains(q, "from gk_kanban_board"):
			return []map[string]any{{"id": int64(1)}}, nil
		case strings.Contains(q, "from ticket where"):
			return []map[string]any{{"id": int64(42), "queue_id": int64(1)}}, nil
		case strings.Contains(q, "group_user"), strings.Contains(q, "role_user"):
			return []map[string]any{{"group_id": int64(1)}}, nil
		case strings.Contains(q, "from queue q"):
			return []map[string]any{{"id": int64(1)}}, nil
		}
		return nil, nil
	}
	p := newTestPlugin(host, dialectMySQL)

	out, err := p.Call("api_move", json.RawMessage(
		`{"_user_id":7,"board_id":1,"ticket_id":42,"state_id":5,"until_time":0}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil || m["ok"] != true {
		t.Fatalf("response = %s, want ok:true", out)
	}
	if calledWith.ticketID != 42 || calledWith.stateID != 5 || calledWith.userID != 7 {
		t.Fatalf("ChangeTicketStatus args = %+v", calledWith)
	}
	for _, e := range host.execs {
		if strings.Contains(strings.ToLower(e), "update ticket") {
			t.Fatalf("plugin must not UPDATE ticket directly: %s", e)
		}
	}
}

// TestBoardListScopesByQueueAccess: a non-admin's board list query must carry
// the access filter (queue_id IS NULL OR IN (...)) bound to exactly their
// accessible queue ids — a per-queue board on a foreign queue must never be
// listed to them.
func TestBoardListScopesByQueueAccess(t *testing.T) {
	var boardQuery string
	var boardArgs []any
	host := &fakeHost{}
	host.queryFn = func(query string, args ...any) ([]map[string]any, error) {
		q := strings.ToLower(query)
		switch {
		case strings.Contains(q, "from gk_kanban_board"):
			boardQuery, boardArgs = query, args
			return []map[string]any{
				{"id": int64(1), "name": "Everything", "queue_id": nil, "create_time": "2026-01-01", "queue_name": "", "ticket_count": int64(2)},
				{"id": int64(2), "name": "Coaching", "queue_id": int64(37), "create_time": "2026-01-02", "queue_name": "Coaching", "ticket_count": int64(1)},
			}, nil
		case strings.Contains(q, "group_user"), strings.Contains(q, "role_user"):
			return []map[string]any{{"group_id": int64(11)}}, nil
		case strings.Contains(q, "from queue"):
			return []map[string]any{{"id": int64(13), "name": "CopperForge"}}, nil
		}
		return nil, nil
	}
	p := newTestPlugin(host, dialectMySQL)

	out, err := p.Call("render_boards", json.RawMessage(`{"_user_id":6}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if boardQuery == "" {
		t.Fatal("board list query was not issued")
	}
	if !strings.Contains(boardQuery, "b.queue_id IS NULL OR b.queue_id IN") {
		t.Fatalf("board list query not scoped by queue access: %s", boardQuery)
	}
	if len(boardArgs) != 1 || toInt64(boardArgs[0]) != 13 {
		t.Fatalf("board list args = %v, want [13] (the only accessible queue)", boardArgs)
	}
	if !strings.Contains(string(out), "kb-page") {
		t.Fatalf("response = %s, want the board list page", out)
	}
}
