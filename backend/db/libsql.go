package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
)

type LibSQLStore struct{ sqlStore }

// NewLibSQLStoreWithAuthToken opens a remote-only Turso store while keeping
// the token out of the persisted/configured URL.
func NewLibSQLStoreWithAuthToken(dsn, authToken string) (*LibSQLStore, error) {
	return NewLibSQLStoreWithAuthTokenContext(context.Background(), dsn, authToken)
}

// NewLibSQLStoreWithAuthTokenContext is the strict remote opener. The target
// URL must be query-free; authToken is supplied separately. Cancellation is
// checked before native setup, but already-started native connects and SQL
// operations are non-interruptible; direct callers may remain blocked until
// they return. A late-completing open is closed rather than retained.
func NewLibSQLStoreWithAuthTokenContext(ctx context.Context, dsn, authToken string) (*LibSQLStore, error) {
	if ctx == nil {
		return nil, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalizedDSN, err := libSQLDriverDSNStrict(dsn, authToken)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("libsql", normalizedDSN)
	if err != nil {
		return nil, errors.New("open remote libsql failed")
	}

	return &LibSQLStore{sqlStore{db: db}}, nil
}

func libSQLDriverDSN(dsn, authToken string) (string, error) {
	return libSQLDriverDSNPolicy(dsn, authToken, false)
}

func libSQLDriverDSNStrict(dsn, authToken string) (string, error) {
	return libSQLDriverDSNPolicy(dsn, authToken, true)
}

func libSQLDriverDSNPolicy(dsn, authToken string, rejectQuery bool) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid libsql URL")
	}
	if (u.Scheme != "libsql" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid libsql URL")
	}
	if u.User != nil || u.Fragment != "" {
		return "", fmt.Errorf("invalid libsql URL")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("invalid libsql URL")
	}
	if rejectQuery && u.RawQuery != "" {
		return "", fmt.Errorf("invalid libsql URL")
	}
	queryToken := query.Get("authToken")
	for key := range query {
		if key != "authToken" {
			return "", fmt.Errorf("invalid libsql URL")
		}
	}
	if authToken == "" {
		authToken = queryToken
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("invalid libsql URL: path is not supported")
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	if authToken != "" {
		// go-libsql consumes the credential as a driver option encoded in its
		// private connection string. It is never returned in an error or log.
		q := url.Values{}
		q.Set("authToken", authToken)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func (s *LibSQLStore) Close() error { return s.db.Close() }
