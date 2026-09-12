package migration

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"slices"
	"strings"

	_ "modernc.org/sqlite" // Register SQLite for the legacy schema preflight.
)

type schemaColumn struct {
	table, name, kind   string
	notNull, primaryKey int
	defaultValue        sql.NullString
}

func legacyBaseline(ctx context.Context, databaseURL string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	driver, dsn := "libsql", databaseURL
	if u.Scheme == "sqlite" {
		driver, dsn = "sqlite", u.Path
	}
	conn, err := sql.Open(driver, dsn)
	if err != nil {
		return "", err
	}
	defer conn.Close() //nolint:errcheck // read-only migration preflight
	conn.SetMaxOpenConns(1)
	var atlasTable, gooseTable, tables int
	if err := conn.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(name='atlas_schema_revisions'),0),coalesce(sum(name='goose_db_version'),0) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables, &atlasTable, &gooseTable); err != nil {
		return "", err
	}
	if tables == 0 {
		return "", nil
	}
	if atlasTable > 0 {
		var revisions int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM atlas_schema_revisions`).Scan(&revisions); err != nil {
			return "", err
		}
		if revisions > 0 {
			return "", nil
		}
	}
	if gooseTable > 0 {
		var version, invalid int
		if err := conn.QueryRowContext(ctx, `SELECT coalesce(max(version_id),0),coalesce(sum(version_id NOT IN (0,1) OR is_applied<>1),0) FROM goose_db_version`).Scan(&version, &invalid); err != nil {
			return "", err
		}
		if version != 1 || invalid != 0 {
			return "", errors.New("unsupported legacy migration history")
		}
	}
	var customObjects int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type IN ('view','trigger') OR (type='index' AND sql IS NOT NULL)`).Scan(&customObjects); err != nil {
		return "", err
	}
	if customObjects != 0 {
		return "", errors.New("unrecognized legacy schema objects")
	}
	actual, err := readSchemaColumns(ctx, conn)
	if err != nil {
		return "", err
	}
	if len(actual) == 0 && gooseTable == 0 {
		return "", nil
	}
	expectedDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return "", err
	}
	defer expectedDB.Close() //nolint:errcheck // disposable schema comparison
	expectedDB.SetMaxOpenConns(1)
	baseline, err := migrationAssets.ReadFile("assets/00001_baseline.sql")
	if err != nil {
		return "", err
	}
	if _, err := expectedDB.ExecContext(ctx, strings.SplitN(string(baseline), "-- +goose Down", 2)[0]); err != nil {
		return "", err
	}
	expected, err := readSchemaColumns(ctx, expectedDB)
	if err != nil {
		return "", err
	}
	if !slices.Equal(actual, expected) {
		return "", errors.New("database does not match the shipped legacy schema")
	}
	return "00001", nil
}

func readSchemaColumns(ctx context.Context, conn *sql.DB) ([]schemaColumn, error) {
	rows, err := conn.QueryContext(ctx, `SELECT m.name,p.name,p.type,p."notnull",p.dflt_value,p.pk FROM sqlite_schema m JOIN pragma_table_info(m.name) p WHERE m.type='table' AND m.name NOT LIKE 'sqlite_%' AND m.name NOT IN ('goose_db_version','atlas_schema_revisions') ORDER BY m.name,p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // read-only schema rows
	var columns []schemaColumn
	for rows.Next() {
		var column schemaColumn
		if err := rows.Scan(&column.table, &column.name, &column.kind, &column.notNull, &column.defaultValue, &column.primaryKey); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}
