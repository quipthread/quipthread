package migration

import (
	"context"
	"database/sql"
	"errors"
)

// These fields were added to the consolidated Goose baseline without ALTERs for
// existing databases. Only their absence is compatible with that shipped schema.
var historicalColumns = map[string]string{
	"blocked_terms.is_regex": "ALTER TABLE blocked_terms ADD COLUMN is_regex INTEGER NOT NULL DEFAULT 0",
	"comments.flags":         "ALTER TABLE comments ADD COLUMN flags INTEGER NOT NULL DEFAULT 0",
	"comments.upvotes":       "ALTER TABLE comments ADD COLUMN upvotes INTEGER NOT NULL DEFAULT 0",
	"users.shadow_banned":    "ALTER TABLE users ADD COLUMN shadow_banned INTEGER NOT NULL DEFAULT 0",
}

func completeHistoricalBaseline(ctx context.Context, tx *sql.Tx, actual, expected []schemaColumn) error {
	var statements []string
	next := 0
	for _, column := range expected {
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
