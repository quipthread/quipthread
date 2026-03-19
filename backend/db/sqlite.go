package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register modernc/sqlite driver
)

type SQLiteStore struct{ sqlStore }

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil { //nolint:noctx // DB layer; full context threading deferred
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil { //nolint:noctx // DB layer; full context threading deferred
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	store := &SQLiteStore{sqlStore{db: db}}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return store, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }
