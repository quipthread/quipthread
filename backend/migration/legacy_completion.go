package migration

import (
	"context"
	"database/sql"
	"errors"
)

// These fields postdate the released pre-Atlas schemas. Only their absence
// is compatible; columns with different definitions are rejected.
var historicalColumns = map[string]string{
	"blocked_terms.is_regex": "ALTER TABLE blocked_terms ADD COLUMN is_regex INTEGER NOT NULL DEFAULT 0",
	"comments.flags":         "ALTER TABLE comments ADD COLUMN flags INTEGER NOT NULL DEFAULT 0",
	"comments.upvotes":       "ALTER TABLE comments ADD COLUMN upvotes INTEGER NOT NULL DEFAULT 0",
	"sites.notify_interval":  "ALTER TABLE sites ADD COLUMN notify_interval INTEGER",
	"users.shadow_banned":    "ALTER TABLE users ADD COLUMN shadow_banned INTEGER NOT NULL DEFAULT 0",
}

func completeHistoricalBaseline(ctx context.Context, tx *sql.Tx, actual, expected []schemaColumn, expectedDB *sql.DB) error {
	var statements []string
	present := make(map[string]bool)
	for _, column := range actual {
		present[column.table] = true
	}
	missing := make(map[string]bool)
	for _, table := range []string{"comment_flags", "comment_votes"} {
		if present[table] {
			continue
		}
		var definition string
		if err := expectedDB.QueryRowContext(ctx, `SELECT sql FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&definition); err != nil {
			return err
		}
		missing[table] = true
		statements = append(statements, definition)
	}
	next := 0
	for _, column := range expected {
		if missing[column.table] {
			continue
		}
		if next < len(actual) && actual[next] == column {
			next++
			continue
		}
		statement, supported := historicalColumns[column.table+"."+column.name]
		if !supported {
			return errors.New("database does not match the shipped legacy schema")
		}
		statements = append(statements, statement)
	}
	if next != len(actual) {
		return errors.New("database does not match the shipped legacy schema")
	}
	// Validation completes before any DDL, and the caller holds one transaction
	// across schema inspection and all additive changes.
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
