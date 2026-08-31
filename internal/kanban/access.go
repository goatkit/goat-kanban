package kanban

import (
	"context"
	"fmt"
)

// AccessService centralises every queue-scoping decision the plugin makes.
// It mirrors internal/service/queue_access_service.go from the platform
// (copied, not imported — the platform boundary forbids it): effective
// groups are the UNION of direct group_user grants and role-based grants
// (role_user -> roles -> group_role), with 'rw' superseding all other
// permission keys. Admins (members of the group named "admin") bypass queue
// scoping entirely.
type AccessService struct {
	host hostQuerier
	d    dialect
}

// NewAccessService returns a queue-access service bound to the host DB and
// the detected dialect.
func NewAccessService(host hostQuerier, d dialect) *AccessService {
	return &AccessService{host: host, d: d}
}

// effectiveGroupIDs returns all group IDs the user can access with the given
// permission type (OTRS rule: 'rw' supersedes all other permissions, so a
// 'ro' check also matches users holding 'rw'). Mirrors core's
// QueueAccessService.GetUserEffectiveGroupIDs exactly.
func (a *AccessService) effectiveGroupIDs(ctx context.Context, userID int64, permType string) ([]int64, error) {
	g := a.d.quoteIdent("groups")
	query := `
		SELECT DISTINCT gu.group_id
		FROM group_user gu
		JOIN ` + g + ` g ON gu.group_id = g.id
		WHERE gu.user_id = ?
		  AND g.valid_id = 1
		  AND (gu.permission_key = ? OR gu.permission_key = 'rw')
		UNION
		SELECT DISTINCT gr.group_id
		FROM role_user ru
		JOIN roles r ON ru.role_id = r.id
		JOIN group_role gr ON ru.role_id = gr.role_id
		JOIN ` + g + ` g ON gr.group_id = g.id
		WHERE ru.user_id = ?
		  AND r.valid_id = 1
		  AND g.valid_id = 1
		  AND (gr.permission_key = ? OR gr.permission_key = 'rw')
		  AND gr.permission_value = 1`

	rows, err := a.host.DBQuery(ctx, query, userID, permType, userID, permType)
	if err != nil {
		return nil, fmt.Errorf("kanban: get effective groups: %w", err)
	}
	groupIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		groupIDs = append(groupIDs, toInt64(row["group_id"]))
	}
	return groupIDs, nil
}

// accessibleQueueIDs returns the set of queue IDs the user can access with
// read permission ('ro' check; 'rw' holders are included via supersession).
// Mirrors core's QueueAccessService.GetAccessibleQueueIDs.
func (a *AccessService) accessibleQueueIDs(ctx context.Context, userID int64) (map[int64]bool, error) {
	groupIDs, err := a.effectiveGroupIDs(ctx, userID, "ro")
	if err != nil {
		return nil, err
	}
	set := make(map[int64]bool, len(groupIDs))
	if len(groupIDs) == 0 {
		return set, nil
	}

	ph, args := inPlaceholders(groupIDs)
	g := a.d.quoteIdent("groups")
	query := `
		SELECT q.id
		FROM queue q
		JOIN ` + g + ` g ON q.group_id = g.id
		WHERE q.valid_id = 1
		  AND g.valid_id = 1
		  AND q.group_id IN (` + ph + `)`

	rows, err := a.host.DBQuery(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("kanban: get accessible queues: %w", err)
	}
	for _, row := range rows {
		set[toInt64(row["id"])] = true
	}
	return set, nil
}

// canAccessQueue reports whether the caller may see or act on tickets in the
// given queue. Admins bypass queue scoping (the platform injects _is_admin
// from membership of the group named "admin").
func (a *AccessService) canAccessQueue(ctx context.Context, rc reqCtx, queueID int64) (bool, error) {
	if rc.IsAdmin {
		return true, nil
	}
	set, err := a.accessibleQueueIDs(ctx, rc.UserID)
	if err != nil {
		return false, err
	}
	return set[queueID], nil
}

// boardAllowed reports whether the caller may use the board at all: an
// all-queues board (queue_id NULL) is always allowed for authenticated
// callers (its contents are scoped per-query to their queues); a per-queue
// board requires access to that queue.
func (a *AccessService) boardAllowed(ctx context.Context, rc reqCtx, boardQueueID *int64) (bool, error) {
	if boardQueueID == nil {
		return true, nil
	}
	return a.canAccessQueue(ctx, rc, *boardQueueID)
}
