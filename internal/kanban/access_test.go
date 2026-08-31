package kanban

import (
	"context"
	"strings"
	"testing"
)

// accessFake is a hostQuerier that scripts rows for the two query shapes
// AccessService issues: effective-group UNION and queue-by-group IN. It also
// records every query so tests can assert on DB round-trips.
type accessFake struct {
	groups  []int64         // group ids granted to the user (rw or ro — both match)
	queues  map[int64]int64 // queue id -> group id
	queries []string
}

func (a *accessFake) DBQuery(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	a.queries = append(a.queries, query)
	q := strings.ToLower(query)
	switch {
	case strings.Contains(q, "group_user"), strings.Contains(q, "role_user"):
		rows := make([]map[string]any, 0, len(a.groups))
		for _, g := range a.groups {
			rows = append(rows, map[string]any{"group_id": g})
		}
		return rows, nil
	case strings.Contains(q, "from queue q"):
		rows := make([]map[string]any, 0, len(a.queues))
		for id, gid := range a.queues {
			if containsInt64(a.groups, gid) {
				rows = append(rows, map[string]any{"id": id})
			}
		}
		return rows, nil
	}
	return nil, nil
}

func (a *accessFake) DBExec(ctx context.Context, query string, args ...any) (int64, error) {
	return 0, nil
}

func (a *accessFake) Log(ctx context.Context, level, message string, fields map[string]any) {}

func containsInt64(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestCanAccessQueueAdminBypass(t *testing.T) {
	// Admin must bypass queue scoping without any DB round-trip.
	f := &accessFake{queues: map[int64]int64{}}
	a := NewAccessService(f, dialectMySQL)
	ok, err := a.canAccessQueue(context.Background(), reqCtx{UserID: 1, IsAdmin: true}, 999)
	if err != nil || !ok {
		t.Fatalf("admin bypass: ok=%v err=%v", ok, err)
	}
	if len(f.queries) != 0 {
		t.Fatalf("admin must not query the DB, got %d queries: %v", len(f.queries), f.queries)
	}
}

func TestCanAccessQueueForeignForbidden(t *testing.T) {
	// User holds ro on group 1, which owns queue 1. Queue 2 is foreign.
	f := &accessFake{groups: []int64{1}, queues: map[int64]int64{1: 1, 2: 2}}
	a := NewAccessService(f, dialectMySQL)

	if ok, _ := a.canAccessQueue(context.Background(), reqCtx{UserID: 7}, 1); !ok {
		t.Fatal("queue 1 must be accessible")
	}
	if ok, _ := a.canAccessQueue(context.Background(), reqCtx{UserID: 7}, 2); ok {
		t.Fatal("queue 2 must NOT be accessible")
	}
}

func TestAccessibleQueueIDsEmptyGroups(t *testing.T) {
	f := &accessFake{}
	a := NewAccessService(f, dialectMySQL)
	set, err := a.accessibleQueueIDs(context.Background(), 7)
	if err != nil {
		t.Fatalf("accessibleQueueIDs: %v", err)
	}
	if len(set) != 0 {
		t.Fatalf("no groups -> no queues, got %v", set)
	}
}

func TestBoardAllowedAllQueuesScope(t *testing.T) {
	// A NULL-queue board is always allowed; contents are scoped per-query.
	f := &accessFake{queues: map[int64]int64{}}
	a := NewAccessService(f, dialectMySQL)
	ok, err := a.boardAllowed(context.Background(), reqCtx{UserID: 7}, nil)
	if err != nil || !ok {
		t.Fatalf("all-queues board must be allowed: ok=%v err=%v", ok, err)
	}

	qid := int64(2)
	ok, _ = a.boardAllowed(context.Background(), reqCtx{UserID: 7}, &qid)
	if ok {
		t.Fatal("per-queue board for a foreign queue must be forbidden")
	}
}

func TestQuoteIdentDialects(t *testing.T) {
	if got := dialectPostgres.quoteIdent("groups"); got != `"groups"` {
		t.Fatalf("postgres quote = %q", got)
	}
	if got := dialectMySQL.quoteIdent("groups"); got != "`groups`" {
		t.Fatalf("mysql quote = %q", got)
	}
}
