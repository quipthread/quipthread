package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // register the SQLite driver for local tenant stores

	"github.com/quipthread/quipthread/models"
)

// LegacyDigestInventoryStore is the read-only capability used by the default
// operator inventory. It has no mutation methods by construction.
type LegacyDigestInventoryStore interface {
	ListUninitializedSiteDigestsContext(ctx context.Context) ([]*models.NotificationOutbox, error)
	Close() error
}

// LegacyDigestStore is the explicitly opened mutation capability for one
// tenant. Tenant identity is resolved by the cloud control plane before this
// capability is constructed.
type LegacyDigestStore interface {
	LegacyDigestInventoryStore
	GetNotificationOutbox(id string) (*models.NotificationOutbox, error)
	ClaimLegacySiteDigestContext(ctx context.Context, outboxID, owner string, now time.Time, leaseFor time.Duration) (*models.NotificationOutbox, error)
	ReconcileLegacySiteDigestContext(ctx context.Context, outboxID, owner string, generation int, commentIDs []string, now time.Time) error
	RetireLegacySiteDigestContext(ctx context.Context, outboxID, owner string, generation int, now time.Time) error
}

type legacyDigestSQLStore struct{ sqlStore }

func (s *legacyDigestSQLStore) Close() error { return s.db.Close() }

func openLegacyDigestSQLStore(db *sql.DB) *legacyDigestSQLStore {
	return &legacyDigestSQLStore{sqlStore: sqlStore{db: db}}
}

// OpenSQLiteLegacyDigestInventory opens an existing local tenant database in
// SQLite read-only mode. It performs no migrations, schema repair, or file
// creation.
func OpenSQLiteLegacyDigestInventory(path string) (LegacyDigestInventoryStore, error) {
	db, err := openSQLiteLegacyDigest(path, true)
	if err != nil {
		return nil, err
	}
	return openLegacyDigestSQLStore(db), nil
}

// OpenSQLiteLegacyDigestStore opens an existing local tenant database for an
// explicit operator mutation. It performs no migrations or schema repair.
func OpenSQLiteLegacyDigestStore(path string) (LegacyDigestStore, error) {
	db, err := openSQLiteLegacyDigest(path, false)
	if err != nil {
		return nil, err
	}
	return openLegacyDigestSQLStore(db), nil
}

func openSQLiteLegacyDigest(path string, readOnly bool) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("legacy digest sqlite path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("legacy digest sqlite db not accessible: %w", err)
	}
	dsn := path
	if readOnly {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve legacy digest sqlite path: %w", err)
		}
		resolved, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat legacy digest sqlite db: %w", err)
		}
		if !resolved.Mode().IsRegular() {
			return nil, errors.New("legacy digest sqlite path is not a regular file")
		}
		dsn = (&url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro"}).String()
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open legacy digest sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure legacy digest sqlite: %w", err)
	}
	return db, nil
}

// OpenLibSQLLegacyDigestInventory opens a tenant libSQL database for the
// default read-only inventory. The auth token is passed out-of-band.
func OpenLibSQLLegacyDigestInventory(dbURL, authToken string) (LegacyDigestInventoryStore, error) {
	return openLibSQLLegacyDigest(dbURL, authToken)
}

// OpenLibSQLLegacyDigestStore opens a tenant libSQL database for an explicit
// mutation. The auth token is passed out-of-band and never appended to dbURL.
func OpenLibSQLLegacyDigestStore(dbURL, authToken string) (LegacyDigestStore, error) {
	return openLibSQLLegacyDigest(dbURL, authToken)
}

func openLibSQLLegacyDigest(dbURL, authToken string) (*legacyDigestSQLStore, error) {
	if authToken == "" {
		return nil, errors.New("legacy digest libsql auth token is required")
	}
	u, err := url.Parse(dbURL)
	if err != nil || (u.Scheme != "libsql" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("legacy digest libsql URL is invalid")
	}
	dsn, err := libSQLDriverDSN(dbURL, authToken)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, errors.New("open legacy digest libsql failed")
	}
	db.SetMaxOpenConns(1)
	return openLegacyDigestSQLStore(db), nil
}
