package db

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "github.com/tursodatabase/libsql-client-go/libsql" // register libsql driver for Turso
)

type LibSQLStore struct{ sqlStore }

func NewLibSQLStore(dsn string) (*LibSQLStore, error) {
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open libsql: %w", err)
	}

	store := &LibSQLStore{sqlStore{db: db, dialect: goose.DialectTurso}}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := store.ensureColumns(); err != nil {
		return nil, fmt.Errorf("ensure columns: %w", err)
	}

	return store, nil
}

func (s *LibSQLStore) Close() error { return s.db.Close() }
