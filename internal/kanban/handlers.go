package kanban

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	plugin "github.com/goatkit/goatflow/pkg/plugin"
)

// reqCtx is the per-request caller identity, extracted from the args the
// platform router injects (internal/platform/pluginui/router.go): _user_id,
// _user_login, _is_admin, _org_id|org_id, _user_role.
type reqCtx struct {
	OrgID   int64
	Role    string
	IsAdmin bool
	UserID  int64
	Login   string
}

// extractReqCtx pulls caller identity out of the router-injected args. Admin
// detection trusts the injected _is_admin (core derives it from membership of
// the group named "admin") and defensively also accepts role "Admin".
func extractReqCtx(args map[string]any) reqCtx {
	rc := reqCtx{UserID: argInt(args, "_user_id"), Login: argString(args, "_user_login")}
	if v := argBool(args, "_is_admin"); v {
		rc.IsAdmin = true
	}
	rc.Role = argString(args, "_user_role")
	if rc.Role == "Admin" || rc.Role == "admin" {
		rc.IsAdmin = true
	}
	rc.OrgID = argInt(args, "_org_id")
	if rc.OrgID == 0 {
		rc.OrgID = argInt(args, "org_id")
	}
	return rc
}

// argRaw finds a raw arg value: top-level first (harmless), then the router's
// params map (:id extraction), then query (url.Values shape). Values may be
// string, []string, []any, bool, or JSON numbers.
func argRaw(args map[string]any, key string) any {
	if v, ok := args[key]; ok && v != nil {
		return v
	}
	for _, subKey := range []string{"params", "query"} {
		sub, ok := args[subKey].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := sub[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

// unwrapFirst reduces a single-element []any / []string (url.Values from the
// router's query map) to its element.
func unwrapFirst(v any) any {
	switch t := v.(type) {
	case []any:
		if len(t) == 1 {
			return t[0]
		}
	case []string:
		if len(t) == 1 {
			return t[0]
		}
	}
	return v
}

// argString converts a raw arg to its first string value.
func argString(args map[string]any, key string) string {
	switch v := argRaw(args, key).(type) {
	case string:
		return v
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// argInt converts a raw arg to int64.
func argInt(args map[string]any, key string) int64 {
	switch v := unwrapFirst(argRaw(args, key)).(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		i, _ := strconv.ParseInt(v, 10, 64)
		return i
	case bool:
		if v {
			return 1
		}
	}
	return 0
}

// argBool converts a raw arg to bool (JSON bool or "1"/"true").
func argBool(args map[string]any, key string) bool {
	switch v := argRaw(args, key).(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v == "1" || strings.EqualFold(v, "true")
	}
	return false
}

// parseBody unmarshals the POST payload. The router delivers it as a raw
// string under "body"; fall back to top-level args for direct callers/tests.
func parseBody(args map[string]any) (map[string]any, error) {
	var raw []byte
	if b, ok := args["body"].(string); ok && b != "" {
		raw = []byte(b)
	} else {
		data, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("kanban: encode body: %w", err)
		}
		raw = data
	}
	out := map[string]any{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("kanban: invalid JSON body: %w", err)
	}
	return out, nil
}

// toInt64 coerces DB row values (driver-dependent int/float types) to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case []byte:
		return parseIntBytes(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	case time.Time:
		return n.Unix()
	}
	return 0
}

// toInt is the int flavour of toInt64 for small ids/counts.
func toInt(v any) int { return int(toInt64(v)) }

func parseIntBytes(b []byte) int64 {
	i, _ := strconv.ParseInt(string(b), 10, 64)
	return i
}

// isNullValue reports whether a DB row value is SQL NULL (nil interface or a
// typed nil []byte, depending on driver).
func isNullValue(v any) bool {
	if v == nil {
		return true
	}
	if b, ok := v.([]byte); ok {
		return b == nil
	}
	return false
}

// rowStr renders a DB row value as a string (empty for NULL).
func rowStr(v any) string {
	if isNullValue(v) {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case time.Time:
		return s.Format("2006-01-02T15:04")
	}
	return fmt.Sprintf("%v", v)
}

// jsonOut marshals a value as the handler's JSON response.
func jsonOut(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("kanban: encode response: %w", err)
	}
	return data, nil
}

// errorResponse is the goat-kb API error convention: {"error": msg, "status": N}.
// The router always answers HTTP 200; JS clients read status from the body.
func errorResponse(status int, msg string) json.RawMessage {
	data, _ := json.Marshal(map[string]any{"error": msg, "status": status})
	return data
}

// pageOut wraps rendered HTML in the {"html","title"} page shape the router
// injects into the shell's PluginHTML slot.
func pageOut(title, html string) (json.RawMessage, error) {
	return jsonOut(map[string]any{"html": html, "title": title})
}

// translate resolves a plugin i18n key via the host (the manager prefixes
// keys with the plugin name). Falls back to English.
func (p *Plugin) translate(ctx context.Context, rc reqCtx, key string) string {
	if p.host == nil {
		return t("en", key)
	}
	s := p.host.Translate(ctx, "goat-kanban."+key)
	if s == "" || s == "goat-kanban."+key {
		return t("en", key)
	}
	return s
}

// fmtDate renders a dialect-correct date-only formatting expression for col.
func (p *Plugin) fmtDate(col string) string {
	if p.d == dialectPostgres {
		return "TO_CHAR(" + col + ", 'YYYY-MM-DD')"
	}
	return "DATE_FORMAT(" + col + ", '%Y-%m-%d')"
}

// fmtDateTime renders a dialect-correct ISO-ish datetime expression for col.
func (p *Plugin) fmtDateTime(col string) string {
	if p.d == dialectPostgres {
		return "TO_CHAR(" + col + ", 'YYYY-MM-DD\"T\"HH24:MI')"
	}
	return "DATE_FORMAT(" + col + ", '%Y-%m-%dT%H:%i')"
}

// ---------------------------------------------------------------------------
// Board list page
// ---------------------------------------------------------------------------

type boardRow struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	QueueID     *int64 `json:"queue_id"`
	QueueName   string `json:"queue_name,omitempty"`
	CreatedBy   int64  `json:"created_by"`
	CreateTime  string `json:"create_time"`
	TicketCount int64  `json:"ticket_count"`
}

// handleBoards renders the board list page with the inline create form.
func (p *Plugin) handleBoards(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if p.host == nil {
		return errorResponse(503, "host API not available"), nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		m = map[string]any{}
	}
	rc := extractReqCtx(m)

	rows, err := p.host.DBQuery(ctx, `
		SELECT b.id, b.name, b.queue_id, b.created_by,
		       `+p.fmtDate("b.create_time")+` AS create_time,
		       COALESCE(q.name, '') AS queue_name,
		       (SELECT COUNT(*) FROM gk_kanban_ticket kt WHERE kt.board_id = b.id) AS ticket_count
		FROM gk_kanban_board b
		LEFT JOIN queue q ON b.queue_id = q.id
		ORDER BY b.create_time DESC`)
	if err != nil {
		return errorResponse(500, "failed to load boards"), nil
	}

	boards := make([]boardRow, 0, len(rows))
	for _, r := range rows {
		b := boardRow{
			ID:          toInt64(r["id"]),
			Name:        rowStr(r["name"]),
			CreatedBy:   toInt64(r["created_by"]),
			CreateTime:  rowStr(r["create_time"]),
			TicketCount: toInt64(r["ticket_count"]),
		}
		if v, ok := r["queue_id"]; ok && !isNullValue(v) {
			qid := toInt64(v)
			b.QueueID = &qid
			b.QueueName = rowStr(r["queue_name"])
		}
		boards = append(boards, b)
	}

	// Queue picker options: admins see every queue; agents see accessible ones.
	var queues []map[string]any
	if rc.IsAdmin {
		queues, _ = p.host.DBQuery(ctx, "SELECT id, name FROM queue WHERE valid_id = 1 ORDER BY name")
	} else if set, err := p.access.accessibleQueueIDs(ctx, rc.UserID); err == nil && len(set) > 0 {
		ids := make([]int64, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		ph, phArgs := inPlaceholders(ids)
		if qrows, err := p.host.DBQuery(ctx,
			"SELECT id, name FROM queue WHERE valid_id = 1 AND id IN ("+ph+") ORDER BY name", phArgs...); err == nil {
			queues = qrows
		}
	}

	title := p.translate(ctx, rc, "title")
	return pageOut(title, renderBoardListHTML(rc, boards, queues))
}

// ---------------------------------------------------------------------------
// Board page
// ---------------------------------------------------------------------------

type boardColumn struct {
	StateID   int64  `json:"state_id"`
	SortOrder int    `json:"sort_order"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	TypeName  string `json:"type_name"`
	Pending   bool   `json:"pending"`
}

type boardCard struct {
	ID         int64  `json:"id"`
	TN         string `json:"tn"`
	Title      string `json:"title"`
	StateID    int64  `json:"state_id"`
	Priority   string `json:"priority"`
	Assignee   string `json:"assignee"`
	CreateTime string `json:"create_time"`
}

// boardPage is the full render payload for one board.
type boardPage struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	CreatorID int64         `json:"creator_id"`
	IsCreator bool          `json:"is_creator"`
	Columns   []boardColumn `json:"columns"`
	Cards     []boardCard   `json:"cards"`
}

// handleBoard renders one board: columns from gk_kanban_column (merged with
// state metadata from HostAPI ListTicketStates) and cards for the tickets on
// it. A board with no saved column config falls back to showing every valid
// state so fresh boards are usable immediately; hide/reorder persists via
// /api/board/config.
func (p *Plugin) handleBoard(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if p.host == nil {
		return errorResponse(503, "host API not available"), nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		m = map[string]any{}
	}
	rc := extractReqCtx(m)

	boardID := argInt(m, "id")
	if boardID == 0 {
		return errorResponse(400, p.translate(ctx, rc, "err_invalid_request")), nil
	}

	brows, err := p.host.DBQuery(ctx, "SELECT id, name, queue_id, created_by FROM gk_kanban_board WHERE id = ?", boardID)
	if err != nil {
		return errorResponse(500, "failed to load board"), nil
	}
	if len(brows) == 0 {
		return errorResponse(404, p.translate(ctx, rc, "err_not_found")), nil
	}
	brow := brows[0]
	var boardQueueID *int64
	if v, ok := brow["queue_id"]; ok && !isNullValue(v) {
		qid := toInt64(v)
		boardQueueID = &qid
	}
	if ok, err := p.access.boardAllowed(ctx, rc, boardQueueID); err != nil {
		return errorResponse(500, "failed to check access"), nil
	} else if !ok {
		return errorResponse(403, p.translate(ctx, rc, "err_no_access")), nil
	}

	states, err := p.host.ListTicketStates(ctx)
	if err != nil {
		return errorResponse(500, "failed to load ticket states"), nil
	}
	stateByID := make(map[int64]plugin.TicketStateInfo, len(states))
	for _, s := range states {
		stateByID[s.ID] = s
	}

	crows, err := p.host.DBQuery(ctx,
		"SELECT state_id, sort_order FROM gk_kanban_column WHERE board_id = ? ORDER BY sort_order, state_id", boardID)
	if err != nil {
		return errorResponse(500, "failed to load board columns"), nil
	}

	columns := make([]boardColumn, 0, len(crows))
	if len(crows) == 0 {
		for _, s := range states {
			col := columnFromState(s)
			col.SortOrder = int(col.StateID)
			columns = append(columns, col)
		}
	} else {
		for i, r := range crows {
			sid := toInt64(r["state_id"])
			s, ok := stateByID[sid]
			if !ok {
				continue // state deleted/invalidated since config was saved
			}
			col := columnFromState(s)
			if so := toInt(r["sort_order"]); so > 0 {
				col.SortOrder = so
			} else {
				col.SortOrder = i + 1
			}
			columns = append(columns, col)
		}
	}

	trows, err := p.host.DBQuery(ctx, `
		SELECT t.id, t.tn, t.title, t.ticket_state_id,
		       COALESCE(p.name, '') AS priority,
		       COALESCE(u.login, '') AS assignee,
		       `+p.fmtDateTime("t.create_time")+` AS create_time
		FROM gk_kanban_ticket kt
		JOIN ticket t ON kt.ticket_id = t.id
		LEFT JOIN ticket_priority p ON t.ticket_priority_id = p.id
		LEFT JOIN users u ON t.responsible_user_id = u.id
		WHERE kt.board_id = ?
		ORDER BY t.create_time DESC`, boardID)
	if err != nil {
		return errorResponse(500, "failed to load tickets"), nil
	}
	cards := make([]boardCard, 0, len(trows))
	for _, r := range trows {
		cards = append(cards, boardCard{
			ID:         toInt64(r["id"]),
			TN:         rowStr(r["tn"]),
			Title:      rowStr(r["title"]),
			StateID:    toInt64(r["ticket_state_id"]),
			Priority:   rowStr(r["priority"]),
			Assignee:   rowStr(r["assignee"]),
			CreateTime: rowStr(r["create_time"]),
		})
	}

	page := boardPage{
		ID:        toInt64(brow["id"]),
		Name:      rowStr(brow["name"]),
		CreatorID: toInt64(brow["created_by"]),
		IsCreator: rc.IsAdmin || rc.UserID == toInt64(brow["created_by"]),
		Columns:   columns,
		Cards:     cards,
	}
	return pageOut(page.Name+" — "+p.translate(ctx, rc, "title"), renderBoardHTML(p, ctx, rc, page))
}

// columnFromState builds a display column from HostAPI state metadata.
func columnFromState(s plugin.TicketStateInfo) boardColumn {
	return boardColumn{
		StateID:  s.ID,
		Name:     s.Name,
		Color:    s.Color,
		TypeName: s.TypeName,
		Pending:  strings.HasPrefix(strings.ToLower(s.TypeName), "pending"),
	}
}

// ---------------------------------------------------------------------------
// Palette API
// ---------------------------------------------------------------------------

// handlePalette lists unboarded tickets the caller can access, for the
// palette's search. A per-queue board restricts to its queue; an all-queues
// board restricts to the caller's accessible queues (admins: all).
func (p *Plugin) handlePalette(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if p.host == nil {
		return errorResponse(503, "host API not available"), nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		m = map[string]any{}
	}
	rc := extractReqCtx(m)

	boardID := argInt(m, "board_id")
	q := strings.TrimSpace(argString(m, "q"))
	limit := int(argInt(m, "limit"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	brows, err := p.host.DBQuery(ctx, "SELECT id, queue_id FROM gk_kanban_board WHERE id = ?", boardID)
	if err != nil || len(brows) == 0 {
		return errorResponse(404, "board not found"), nil
	}
	brow := brows[0]

	var where []string
	var dbArgs []any
	if v, ok := brow["queue_id"]; ok && !isNullValue(v) {
		queueID := toInt64(v)
		if ok, err := p.access.canAccessQueue(ctx, rc, queueID); err != nil {
			return errorResponse(500, "failed to check access"), nil
		} else if !ok {
			return errorResponse(403, "no access to this board's queue"), nil
		}
		where = append(where, "t.queue_id = ?")
		dbArgs = append(dbArgs, queueID)
	} else if !rc.IsAdmin {
		set, err := p.access.accessibleQueueIDs(ctx, rc.UserID)
		if err != nil {
			return errorResponse(500, "failed to check access"), nil
		}
		if len(set) == 0 {
			return jsonOut([]any{}) // no queues → nothing visible
		}
		ids := make([]int64, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		ph, phArgs := inPlaceholders(ids)
		where = append(where, "t.queue_id IN ("+ph+")")
		dbArgs = append(dbArgs, phArgs...)
	}
	if q != "" {
		where = append(where, "(t.tn LIKE ? OR t.title LIKE ?)")
		dbArgs = append(dbArgs, "%"+q+"%", "%"+q+"%")
	}

	query := `
		SELECT t.id, t.tn, t.title, t.ticket_state_id,
		       COALESCE(p.name, '') AS priority,
		       COALESCE(u.login, '') AS assignee,
		       ` + p.fmtDateTime("t.create_time") + ` AS create_time
		FROM ticket t
		LEFT JOIN gk_kanban_ticket kt ON kt.ticket_id = t.id AND kt.board_id = ?
		LEFT JOIN ticket_priority p ON t.ticket_priority_id = p.id
		LEFT JOIN users u ON t.responsible_user_id = u.id
		WHERE kt.ticket_id IS NULL` + whereClause(where) + `
		ORDER BY t.create_time DESC
		LIMIT ?`
	queryArgs := append([]any{boardID}, dbArgs...)
	queryArgs = append(queryArgs, limit)

	rows, err := p.host.DBQuery(ctx, query, queryArgs...)
	if err != nil {
		return errorResponse(500, "failed to load palette"), nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"id":          toInt64(r["id"]),
			"tn":          rowStr(r["tn"]),
			"title":       rowStr(r["title"]),
			"state_id":    toInt64(r["ticket_state_id"]),
			"priority":    rowStr(r["priority"]),
			"assignee":    rowStr(r["assignee"]),
			"create_time": rowStr(r["create_time"]),
		})
	}
	return jsonOut(out)
}

// ---------------------------------------------------------------------------
// Move API
// ---------------------------------------------------------------------------

// handleMove drags a card to a new state. Access is checked against the
// ticket's own queue (not just board visibility), then the change goes
// through HostAPI ChangeTicketStatus so core pending due-time semantics apply.
func (p *Plugin) handleMove(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if p.host == nil {
		return errorResponse(503, "host API not available"), nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		m = map[string]any{}
	}
	rc := extractReqCtx(m)

	body, err := parseBody(m)
	if err != nil {
		return errorResponse(400, "invalid request body"), nil
	}
	boardID := int64FromAny(body["board_id"])
	ticketID := int64FromAny(body["ticket_id"])
	stateID := int64FromAny(body["state_id"])
	untilTime := int64FromAny(body["until_time"])
	if boardID == 0 || ticketID == 0 || stateID == 0 {
		return errorResponse(400, "board_id, ticket_id and state_id are required"), nil
	}

	brows, err := p.host.DBQuery(ctx, "SELECT id FROM gk_kanban_board WHERE id = ?", boardID)
	if err != nil || len(brows) == 0 {
		return errorResponse(404, "board not found"), nil
	}
	trows, err := p.host.DBQuery(ctx, "SELECT id, queue_id FROM ticket WHERE id = ?", ticketID)
	if err != nil || len(trows) == 0 {
		return errorResponse(404, "ticket not found"), nil
	}
	queueID := toInt64(trows[0]["queue_id"])
	if ok, err := p.access.canAccessQueue(ctx, rc, queueID); err != nil {
		return errorResponse(500, "failed to check access"), nil
	} else if !ok {
		return errorResponse(403, "no access to this ticket's queue"), nil
	}

	if err := p.host.ChangeTicketStatus(ctx, ticketID, stateID, rc.UserID, untilTime); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "pending time is required") {
			return errorResponse(400, "pending time required"), nil
		}
		return errorResponse(500, msg), nil
	}
	return jsonOut(map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Board create / delete / config / add
// ---------------------------------------------------------------------------

// handleBoardCreate inserts a new board. Creator-or-admin gate: non-admins
// may only scope the board to a queue they can access. HostAPI DBExec returns
// only affected rows (no LastInsertId), so the new id is fetched by name +
// creator after insert; columns are then seeded from all valid states via
// ListTicketStates (ticket_state itself is outside this plugin's db scope).
func (p *Plugin) handleBoardCreate(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if p.host == nil {
		return errorResponse(503, "host API not available"), nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		m = map[string]any{}
	}
	rc := extractReqCtx(m)

	body, err := parseBody(m)
	if err != nil {
		return errorResponse(400, "invalid request body"), nil
	}
	name := strings.TrimSpace(fmt.Sprintf("%v", body["name"]))
	if name == "" {
		return errorResponse(400, "board name is required"), nil
	}

	var queueID *int64
	if v := int64FromAny(body["queue_id"]); v > 0 {
		if !rc.IsAdmin {
			ok, err := p.access.canAccessQueue(ctx, rc, v)
			if err != nil {
				return errorResponse(500, "failed to check access"), nil
			}
			if !ok {
				return errorResponse(403, "no access to this queue"), nil
			}
		}
		qid := v
		queueID = &qid
	}

	var insertSQL string
	var dbArgs []any
	if queueID != nil {
		insertSQL = "INSERT INTO gk_kanban_board (name, queue_id, created_by) VALUES (?, ?, ?)"
		dbArgs = []any{name, *queueID, rc.UserID}
	} else {
		insertSQL = "INSERT INTO gk_kanban_board (name, queue_id, created_by) VALUES (?, NULL, ?)"
		dbArgs = []any{name, rc.UserID}
	}
	if _, err := p.host.DBExec(ctx, insertSQL, dbArgs...); err != nil {
		return errorResponse(500, "failed to create board"), nil
	}

	idrows, err := p.host.DBQuery(ctx,
		"SELECT id FROM gk_kanban_board WHERE name = ? AND created_by = ? ORDER BY id DESC LIMIT 1",
		name, rc.UserID)
	if err != nil || len(idrows) == 0 {
		return errorResponse(500, "failed to create board"), nil
	}
	boardID := toInt64(idrows[0]["id"])

	// Fresh boards start with every valid state as a column.
	states, err := p.host.ListTicketStates(ctx)
	if err == nil {
		for i, s := range states {
			if _, err := p.host.DBExec(ctx,
				"INSERT INTO gk_kanban_column (board_id, state_id, sort_order) VALUES (?, ?, ?)",
				boardID, s.ID, i+1); err != nil {
				p.host.Log(ctx, "warn", "kanban: failed to seed column for new board", map[string]any{"state": s.ID, "err": err.Error()})
			}
		}
	}
	return jsonOut(map[string]any{"ok": true, "board_id": boardID})
}

// handleBoardDelete removes a board and its membership/column rows.
// Creator-or-admin gate.
func (p *Plugin) handleBoardDelete(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if p.host == nil {
		return errorResponse(503, "host API not available"), nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		m = map[string]any{}
	}
	rc := extractReqCtx(m)

	body, err := parseBody(m)
	if err != nil {
		return errorResponse(400, "invalid request body"), nil
	}
	boardID := int64FromAny(body["board_id"])
	if boardID == 0 {
		return errorResponse(400, "board_id is required"), nil
	}

	brows, err := p.host.DBQuery(ctx, "SELECT id, created_by FROM gk_kanban_board WHERE id = ?", boardID)
	if err != nil || len(brows) == 0 {
		return errorResponse(404, "board not found"), nil
	}
	if !rc.IsAdmin && rc.UserID != toInt64(brows[0]["created_by"]) {
		return errorResponse(403, "only the board creator or an admin can delete this board"), nil
	}

	for _, q := range []string{
		"DELETE FROM gk_kanban_ticket WHERE board_id = ?",
		"DELETE FROM gk_kanban_column WHERE board_id = ?",
		"DELETE FROM gk_kanban_board WHERE id = ?",
	} {
		if _, err := p.host.DBExec(ctx, q, boardID); err != nil {
			return errorResponse(500, "failed to delete board"), nil
		}
	}
	return jsonOut(map[string]any{"ok": true})
}

// handleBoardConfig replaces the board's visible columns. Creator-or-admin
// gate. No transaction is available over HostAPI, so this is DELETE all +
// INSERT per state — acceptable for a single admin action; a failure mid-way
// leaves a partial column set that the next save repairs.
func (p *Plugin) handleBoardConfig(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if p.host == nil {
		return errorResponse(503, "host API not available"), nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		m = map[string]any{}
	}
	rc := extractReqCtx(m)

	body, err := parseBody(m)
	if err != nil {
		return errorResponse(400, "invalid request body"), nil
	}
	boardID := int64FromAny(body["board_id"])
	if boardID == 0 {
		return errorResponse(400, "board_id is required"), nil
	}

	brows, err := p.host.DBQuery(ctx, "SELECT id, created_by FROM gk_kanban_board WHERE id = ?", boardID)
	if err != nil || len(brows) == 0 {
		return errorResponse(404, "board not found"), nil
	}
	if !rc.IsAdmin && rc.UserID != toInt64(brows[0]["created_by"]) {
		return errorResponse(403, "only the board creator or an admin can configure this board"), nil
	}

	states, err := p.host.ListTicketStates(ctx)
	if err != nil {
		return errorResponse(500, "failed to load ticket states"), nil
	}
	validStates := make(map[int64]bool, len(states))
	for _, s := range states {
		validStates[s.ID] = true
	}

	type colIn struct {
		StateID   int64 `json:"state_id"`
		SortOrder int   `json:"sort_order"`
	}
	var cols []colIn
	if raw, ok := body["states"].([]any); ok {
		for _, item := range raw {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			cols = append(cols, colIn{StateID: int64FromAny(obj["state_id"]), SortOrder: toInt(obj["sort_order"])})
		}
	}

	if _, err := p.host.DBExec(ctx, "DELETE FROM gk_kanban_column WHERE board_id = ?", boardID); err != nil {
		return errorResponse(500, "failed to update board columns"), nil
	}
	for i, c := range cols {
		if !validStates[c.StateID] {
			continue
		}
		sortOrder := c.SortOrder
		if sortOrder <= 0 {
			sortOrder = i + 1
		}
		if _, err := p.host.DBExec(ctx,
			"INSERT INTO gk_kanban_column (board_id, state_id, sort_order) VALUES (?, ?, ?)",
			boardID, c.StateID, sortOrder); err != nil {
			return errorResponse(500, "failed to update board columns"), nil
		}
	}
	return jsonOut(map[string]any{"ok": true})
}

// handleBoardAdd inserts a palette ticket onto the board. The only raw write
// this plugin performs on ticket data is this membership row; state changes
// always go through ChangeTicketStatus.
func (p *Plugin) handleBoardAdd(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if p.host == nil {
		return errorResponse(503, "host API not available"), nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		m = map[string]any{}
	}
	rc := extractReqCtx(m)

	body, err := parseBody(m)
	if err != nil {
		return errorResponse(400, "invalid request body"), nil
	}
	boardID := int64FromAny(body["board_id"])
	ticketID := int64FromAny(body["ticket_id"])
	if boardID == 0 || ticketID == 0 {
		return errorResponse(400, "board_id and ticket_id are required"), nil
	}

	brows, err := p.host.DBQuery(ctx, "SELECT id FROM gk_kanban_board WHERE id = ?", boardID)
	if err != nil || len(brows) == 0 {
		return errorResponse(404, "board not found"), nil
	}
	trows, err := p.host.DBQuery(ctx, "SELECT id, queue_id FROM ticket WHERE id = ?", ticketID)
	if err != nil || len(trows) == 0 {
		return errorResponse(404, "ticket not found"), nil
	}
	queueID := toInt64(trows[0]["queue_id"])
	if ok, err := p.access.canAccessQueue(ctx, rc, queueID); err != nil {
		return errorResponse(500, "failed to check access"), nil
	} else if !ok {
		return errorResponse(403, "no access to this ticket's queue"), nil
	}

	if _, err := p.host.DBExec(ctx,
		"INSERT INTO gk_kanban_ticket (board_id, ticket_id, added_by) VALUES (?, ?, ?)",
		boardID, ticketID, rc.UserID); err != nil {
		return errorResponse(500, "failed to add ticket to board"), nil
	}
	return jsonOut(map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Small shared helpers
// ---------------------------------------------------------------------------

func int64FromAny(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

// esc HTML-escapes a value for safe interpolation into generated markup.
func esc(s string) string { return html.EscapeString(s) }

// escAttr escapes for use inside double-quoted attributes.
func escAttr(s string) string {
	s = html.EscapeString(s)
	return strings.ReplaceAll(s, `"`, "&quot;")
}

// urlQueryEsc escapes a value for query-string interpolation.
func urlQueryEsc(s string) string { return url.QueryEscape(s) }

// ageClass buckets a ticket's age into a CSS class: "" (fresh), "age-warn"
// (>7d), "age-old" (>30d).
func ageClass(createTime string) string {
	t, err := time.Parse("2006-01-02T15:04", createTime)
	if err != nil {
		return ""
	}
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days > 30:
		return "age-old"
	case days > 7:
		return "age-warn"
	}
	return ""
}

// ageLabel renders a relative age ("3d", "14h") from an ISO-ish timestamp.
func ageLabel(createTime string) string {
	t, err := time.Parse("2006-01-02T15:04", createTime)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d.Hours() < 1:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d.Hours() < 48:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func whereClause(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return " AND " + strings.Join(parts, " AND ")
}
