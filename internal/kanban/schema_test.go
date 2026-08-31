package kanban

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// schemaFake records every executed statement and tracks applied versions so
// migrateSchema can be run twice to prove idempotency.
type schemaFake struct {
	executed []string
	applied  map[int]bool
	queryErr bool
}

func (s *schemaFake) DBQuery(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	if s.queryErr {
		return nil, fmt.Errorf("db down")
	}
	q := strings.ToLower(query)
	if strings.Contains(q, "max(version)") {
		maxV := 0
		for v := range s.applied {
			if v > maxV {
				maxV = v
			}
		}
		return []map[string]any{{"v": maxV}}, nil
	}
	return []map[string]any{{"v": 1}}, nil
}

func (s *schemaFake) DBExec(ctx context.Context, query string, args ...any) (int64, error) {
	s.executed = append(s.executed, query)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(query)), "insert into gk_kanban_schema_version") && len(args) > 0 {
		s.applied[toInt(args[0])] = true
	}
	return 1, nil
}

func (s *schemaFake) Log(ctx context.Context, level, message string, fields map[string]any) {}

// TestMigrateSchemaAppliesV1 runs a fresh migration and checks the statement
// mix per dialect: MySQL keeps inline INDEX + InnoDB; Postgres must use
// separate CREATE INDEX statements (inline INDEX is invalid there).
func TestMigrateSchemaAppliesV1(t *testing.T) {
	for name, d := range map[string]dialect{"mysql": dialectMySQL, "postgres": dialectPostgres} {
		t.Run(name, func(t *testing.T) {
			f := &schemaFake{applied: map[int]bool{}}
			if err := migrateSchema(context.Background(), f, d); err != nil {
				t.Fatalf("migrateSchema: %v", err)
			}

			var createTable, createIndex int
			for _, stmt := range f.executed {
				lower := strings.ToLower(stmt)
				if strings.Contains(lower, "create table") {
					createTable++
					if d == dialectPostgres && strings.Contains(strings.ToUpper(stmt), "INDEX ") && !strings.HasPrefix(strings.TrimSpace(stmt), "CREATE INDEX") {
						t.Errorf("postgres CREATE TABLE with inline INDEX (invalid DDL):\n%s", stmt)
					}
				}
				if strings.HasPrefix(lower, "create index") {
					createIndex++
				}
			}
			if createTable != 4 { // version table + 3 kanban tables
				t.Fatalf("create table count = %d, want 4", createTable)
			}
			if d == dialectPostgres && createIndex != 3 {
				t.Fatalf("postgres separate CREATE INDEX count = %d, want 3", createIndex)
			}
			if !f.applied[1] {
				t.Fatal("version 1 not recorded")
			}
		})
	}
}

// TestMigrateSchemaIdempotent: a second run must execute zero DDL beyond the
// version-table IF NOT EXISTS and the version read.
func TestMigrateSchemaIdempotent(t *testing.T) {
	f := &schemaFake{applied: map[int]bool{}}
	if err := migrateSchema(context.Background(), f, dialectMySQL); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	first := len(f.executed)
	if first < 5 {
		t.Fatalf("first run executed only %d statements", first)
	}

	if err := migrateSchema(context.Background(), f, dialectMySQL); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	for _, stmt := range f.executed[first:] {
		lower := strings.ToLower(stmt)
		if strings.Contains(lower, "create table") && !strings.Contains(lower, "gk_kanban_schema_version") {
			t.Errorf("idempotency violated, re-executed: %s", stmt)
		}
		if strings.HasPrefix(lower, "create index") || strings.HasPrefix(lower, "insert into gk_kanban_schema_version") {
			t.Errorf("idempotency violated, re-executed: %s", stmt)
		}
	}
}

// TestMigrateSchemaUnknownDialect must fail fast rather than guess.
func TestMigrateSchemaUnknownDialect(t *testing.T) {
	f := &schemaFake{applied: map[int]bool{}}
	if err := migrateSchema(context.Background(), f, dialectUnknown); err == nil {
		t.Fatal("unknown dialect must error")
	}
	if len(f.executed) != 0 {
		t.Fatalf("no DDL may run before dialect check, got %d", len(f.executed))
	}
}

// TestMigrateSchemaDBError propagates host errors.
func TestMigrateSchemaDBError(t *testing.T) {
	f := &schemaFake{applied: map[int]bool{}, queryErr: true}
	if err := migrateSchema(context.Background(), f, dialectMySQL); err == nil {
		t.Fatal("DB error must propagate")
	}
}

// TestV1DDLRendersBothDialects renders every v1 template for both dialects and
// checks the MySQL/Postgres marker split: AUTO_INCREMENT + ENGINE=InnoDB only
// on MySQL, BIGINT GENERATED ... IDENTITY only on Postgres.
func TestV1DDLRendersBothDialects(t *testing.T) {
	stmts, ok := migrations[1]
	if !ok || len(stmts) == 0 {
		t.Fatal("no v1 migration")
	}
	for i, st := range stmts {
		my, pg := dialectMySQL.render(st), dialectPostgres.render(st)
		if my == "" && pg == "" {
			t.Fatalf("statement %d: empty in both dialects", i)
		}
		if strings.Contains(strings.ToLower(my), "generated always as identity") {
			t.Errorf("statement %d: mysql got postgres identity DDL", i)
		}
		if strings.Contains(strings.ToLower(pg), "auto_increment") || strings.Contains(strings.ToLower(pg), "engine=innodb") {
			t.Errorf("statement %d: postgres got mysql-only DDL:\n%s", i, pg)
		}
		if my != "" && strings.Contains(strings.ToLower(my), "create table") && !strings.Contains(strings.ToLower(my), "auto_increment") {
			// gk_kanban_ticket has no id column; it legitimately lacks AUTO_INCREMENT.
			if !strings.Contains(strings.ToLower(my), "gk_kanban_ticket") {
				t.Errorf("statement %d: mysql table DDL without AUTO_INCREMENT:\n%s", i, my)
			}
		}
	}
}

// TestInPlaceholdersEmpty guards the NULL fallback used by IN (...) clauses.
func TestInPlaceholdersEmpty(t *testing.T) {
	ph, args := inPlaceholders(nil)
	if ph != "NULL" || len(args) != 0 {
		t.Fatalf("empty = %q %v, want NULL and no args", ph, args)
	}
	ph, args = inPlaceholders([]int64{1, 2})
	if ph != "?,?" || len(args) != 2 {
		t.Fatalf("two = %q %v", ph, args)
	}
}
