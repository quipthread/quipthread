package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	_ "modernc.org/sqlite" // Register the SQLite driver used by disposable test stores.
)

// The application initializes tenant databases through the Atlas runner. This
// constructor exists solely for package and integration tests that need a
// disposable schema without launching the pinned Atlas binary.
//
//go:embed migrations/*.sql
var testMigrationFiles embed.FS

func NewSQLiteStoreForTest(path string) (*SQLiteStore, error) {
	store, err := NewSQLiteStore(path)
	if err != nil {
		return nil, err
	}
	store.db.SetMaxOpenConns(1)
	var initialized int
	if err := store.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sites'`,
	).Scan(&initialized); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("inspect test schema: %w", err)
	}
	if initialized != 0 {
		return store, nil
	}
	entries, err := fs.ReadDir(testMigrationFiles, "migrations")
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("read test migrations: %w", err)
	}
	for _, entry := range entries {
		contents, err := fs.ReadFile(testMigrationFiles, "migrations/"+entry.Name())
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("read test migration %s: %w", entry.Name(), err)
		}
		up := strings.SplitN(string(contents), "-- +goose Down", 2)[0]
		if _, err := store.db.ExecContext(context.Background(), up); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("apply test migration %s: %w", entry.Name(), err)
		}
	}
	return store, nil
}
