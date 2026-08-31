// Package kanban implements the GoatFlow Kanban board plugin.
//
// schema.go manages the kanban database schema: dialect detection and
// versioned migrations run from InitWithHost against the host database via
// HostAPI.DBExec. DDL is dialect-aware (MySQL/MariaDB and PostgreSQL) because
// the platform supports both and ConvertPlaceholders only rewrites ?
// placeholders, not DDL keywords such as AUTO_INCREMENT vs BIGINT GENERATED.
package kanban

import (
	"context"
	"fmt"
	"strings"
)

// schemaVersion tracks the kanban schema revision. Bump when adding a
// migration entry below. The host persists applied versions in
// gk_kanban_schema_version.
const schemaVersion = 1

// dialect is the detected SQL dialect: "mysql" (MariaDB included) or "postgres".
type dialect string

const (
	dialectMySQL    dialect = "mysql"
	dialectPostgres dialect = "postgres"
	dialectUnknown  dialect = "unknown"
)

// detectDialect probes the host database by selecting from a table that
// exists on every GoatFlow install (gk_organisation). A PostgreSQL-only cast
// disambiguates from MySQL/MariaDB; if the probe can't be parsed at all we
// fall back to a literal cast. We can't use internal/platform/database from a
// plugin (platform boundary), and HostAPI exposes no dialect accessor, so this
// is the cheapest reliable signal.
func detectDialect(ctx context.Context, host hostQuerier) dialect {
	if _, err := host.DBQuery(ctx, "SELECT 1 AS v FROM gk_organisation LIMIT 1"); err == nil {
		if _, err := host.DBQuery(ctx, "SELECT 1::int AS v FROM gk_organisation LIMIT 1"); err == nil {
			return dialectPostgres
		}
		return dialectMySQL
	}
	if _, err := host.DBQuery(ctx, "SELECT 1::int AS v"); err == nil {
		return dialectPostgres
	}
	if _, err := host.DBQuery(ctx, "SELECT 1 AS v"); err == nil {
		return dialectMySQL
	}
	return dialectUnknown
}

// quoteIdent quotes an identifier for the active dialect. `groups` is a
// reserved word in PostgreSQL and must be double-quoted there; MySQL accepts
// backticks (and unquoted `groups` too, but we quote uniformly).
func (d dialect) quoteIdent(name string) string {
	if d == dialectPostgres {
		return `"` + name + `"`
	}
	return "`" + name + "`"
}

// hostQuerier is the subset of plugin.HostAPI schema code needs. Declared
// locally so tests can pass a fake and so the rest of the package can compose
// on it without importing the plugin package here.
type hostQuerier interface {
	DBQuery(ctx context.Context, query string, args ...any) ([]map[string]any, error)
	DBExec(ctx context.Context, query string, args ...any) (int64, error)
	Log(ctx context.Context, level, message string, fields map[string]any)
}

// migrateSchema brings the kanban tables to schemaVersion. Idempotent: it
// records each applied version in gk_kanban_schema_version and skips
// already-applied ones.
func migrateSchema(ctx context.Context, host hostQuerier, d dialect) error {
	if d == dialectUnknown {
		return fmt.Errorf("kanban: could not detect database dialect")
	}

	// Version tracking table. Keep the DDL portable: INTEGER PRIMARY KEY
	// auto-increments on MySQL when it's the PK, and becomes a plain int PK
	// on Postgres (rows are inserted with explicit version numbers, so
	// auto-increment is not required).
	if _, err := host.DBExec(ctx, versionTableDDL); err != nil {
		return fmt.Errorf("kanban: create schema_version table: %w", err)
	}

	current, err := currentSchemaVersion(ctx, host)
	if err != nil {
		return fmt.Errorf("kanban: read schema version: %w", err)
	}
	if current >= schemaVersion {
		host.Log(ctx, "info", fmt.Sprintf("kanban schema up to date (v%d)", current), nil)
		return nil
	}

	for v := current + 1; v <= schemaVersion; v++ {
		stmts, ok := migrations[v]
		if !ok {
			return fmt.Errorf("kanban: no migration for version %d", v)
		}
		host.Log(ctx, "info", fmt.Sprintf("kanban applying schema migration v%d", v), nil)
		for i, stmt := range stmts {
			sqlText := d.render(stmt)
			if sqlText == "" {
				continue // dialect-specific statement (empty for this dialect)
			}
			if _, err := host.DBExec(ctx, sqlText); err != nil {
				return fmt.Errorf("kanban: migration v%d statement %d: %w", v, i+1, err)
			}
		}
		if _, err := host.DBExec(ctx,
			"INSERT INTO gk_kanban_schema_version (version) VALUES (?)", v); err != nil {
			return fmt.Errorf("kanban: record migration v%d: %w", v, err)
		}
		host.Log(ctx, "info", fmt.Sprintf("kanban schema migration v%d applied", v), nil)
	}
	return nil
}

// currentSchemaVersion reads the highest applied version, or 0 if none.
func currentSchemaVersion(ctx context.Context, host hostQuerier) (int, error) {
	rows, err := host.DBQuery(ctx, "SELECT COALESCE(MAX(version), 0) AS v FROM gk_kanban_schema_version")
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return toInt(rows[0]["v"]), nil
}

// versionTableDDL is the version-tracking table, dialect-neutral.
const versionTableDDL = `CREATE TABLE IF NOT EXISTS gk_kanban_schema_version (
		version INTEGER NOT NULL,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (version)
	)`

// migrations is the ordered set of DDL migrations. Each statement is a
// dialectTemplate rendered with the active dialect before execution. v1
// creates the three board tables. Boards are user-created — nothing is seeded.
var migrations = map[int][]dialectTemplate{
	1: {
		// gk_kanban_board: one row per board. queue_id NULL means an
		// all-queues board (visible to every queue the caller can access);
		// otherwise the board is scoped to that single queue. PostgreSQL has
		// no inline INDEX syntax, so its indexes are separate statements.
		{
			mysql: `CREATE TABLE IF NOT EXISTS gk_kanban_board (
				id INTEGER PRIMARY KEY AUTO_INCREMENT,
				name VARCHAR(255) NOT NULL,
				queue_id INTEGER DEFAULT NULL,
				created_by INTEGER NOT NULL,
				create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				change_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				INDEX idx_kanban_board_queue (queue_id),
				INDEX idx_kanban_board_created_by (created_by)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			postgres: `CREATE TABLE IF NOT EXISTS gk_kanban_board (
				id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				queue_id INTEGER DEFAULT NULL,
				created_by INTEGER NOT NULL,
				create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				change_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)`,
		},
		{mysql: "", postgres: `CREATE INDEX IF NOT EXISTS idx_kanban_board_queue ON gk_kanban_board (queue_id)`},
		{mysql: "", postgres: `CREATE INDEX IF NOT EXISTS idx_kanban_board_created_by ON gk_kanban_board (created_by)`},
		// gk_kanban_ticket: board membership. A ticket appears on a board iff
		// a row exists here; its column is the ticket's current state. The
		// only raw write this plugin performs on tickets is inserting/removing
		// these membership rows — state changes always go through the host
		// ChangeTicketStatus API so core invariants (pending due-time) hold.
		{
			mysql: `CREATE TABLE IF NOT EXISTS gk_kanban_ticket (
				board_id INTEGER NOT NULL,
				ticket_id BIGINT NOT NULL,
				added_by INTEGER NOT NULL,
				added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (board_id, ticket_id),
				INDEX idx_kanban_ticket_board (board_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			postgres: `CREATE TABLE IF NOT EXISTS gk_kanban_ticket (
				board_id INTEGER NOT NULL,
				ticket_id BIGINT NOT NULL,
				added_by INTEGER NOT NULL,
				added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (board_id, ticket_id)
			)`,
		},
		// gk_kanban_column: visible states for a board, in display order.
		// Presence of a row means the state is shown; sort_order is the
		// left-to-right position. Customization is hide/reorder only —
		// columns are always real ticket states.
		{
			mysql: `CREATE TABLE IF NOT EXISTS gk_kanban_column (
				id INTEGER PRIMARY KEY AUTO_INCREMENT,
				board_id INTEGER NOT NULL,
				state_id INTEGER NOT NULL,
				sort_order INTEGER NOT NULL DEFAULT 0,
				UNIQUE KEY uk_kanban_col_board_state (board_id, state_id),
				INDEX idx_kanban_column_board (board_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			postgres: `CREATE TABLE IF NOT EXISTS gk_kanban_column (
				id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
				board_id INTEGER NOT NULL,
				state_id INTEGER NOT NULL,
				sort_order INTEGER NOT NULL DEFAULT 0,
				UNIQUE (board_id, state_id)
			)`,
		},
		{mysql: "", postgres: `CREATE INDEX IF NOT EXISTS idx_kanban_column_board ON gk_kanban_column (board_id)`},
	},
}

// dialectTemplate carries a DDL statement per dialect. Statements that are
// identical across dialects set the same string in both fields.
type dialectTemplate struct {
	mysql    string
	postgres string
}

// render returns the DDL for the active dialect.
func (d dialect) render(t dialectTemplate) string {
	switch d {
	case dialectPostgres:
		return t.postgres
	default:
		return t.mysql
	}
}

// inPlaceholders builds a comma-separated ? list with matching args, e.g.
// ("?,?,?", [1 2 3]). Used for IN (...) clauses over HostAPI (which converts
// ? placeholders per dialect).
func inPlaceholders(values []int64) (string, []any) {
	if len(values) == 0 {
		return "NULL", nil
	}
	ph := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		ph[i] = "?"
		args[i] = v
	}
	return strings.Join(ph, ","), args
}
