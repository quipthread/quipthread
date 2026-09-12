package migration

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func historicalDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "historical.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	fixture, err := os.ReadFile("testdata/historical_goose_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(t.Context(), string(fixture)); err != nil {
		t.Fatal(err)
	}
	return conn, "sqlite://" + path
}

func TestHistoricalGooseBaselineRestoresMissingColumns(t *testing.T) {
	// Given the production schema metadata, with synthetic existing records.
	conn, target := historicalDatabase(t)
	if _, err := conn.ExecContext(t.Context(), `INSERT INTO users(id,display_name) VALUES ('old-user','Existing'); INSERT INTO sites(id,owner_id,domain) VALUES ('old-site','old-user','example.invalid'); INSERT INTO comments(id,site_id,page_id,user_id,content) VALUES ('old-comment','old-site','/','old-user','Preserve me'); INSERT INTO blocked_terms(term) VALUES ('existing')`); err != nil {
		t.Fatal(err)
	}
	// When startup adopts the historical schema on successive boots.
	for range 2 {
		version, err := legacyBaseline(t.Context(), target)
		if err != nil || version != "00001" {
			t.Fatalf("baseline=%q error=%v", version, err)
		}
	}
	// Then missing baseline fields default to zero and existing values survive.
	var content string
	var upvotes, flags, shadow, regex int
	if err := conn.QueryRowContext(t.Context(), `SELECT c.content,c.upvotes,c.flags,u.shadow_banned,b.is_regex FROM comments c JOIN users u ON c.user_id=u.id CROSS JOIN blocked_terms b`).Scan(&content, &upvotes, &flags, &shadow, &regex); err != nil {
		t.Fatal(err)
	}
	if content != "Preserve me" || upvotes != 0 || flags != 0 || shadow != 0 || regex != 0 {
		t.Fatal("historical records changed")
	}
}

func TestHistoricalGooseBaselineRejectsDriftBeforeWriting(t *testing.T) {
	for _, statement := range []string{
		`ALTER TABLE users ADD COLUMN unexpected TEXT`,
		`ALTER TABLE users ADD COLUMN unexpected TEXT GENERATED ALWAYS AS (email) VIRTUAL`,
		`ALTER TABLE users DROP COLUMN email; ALTER TABLE users ADD COLUMN email TEXT GENERATED ALWAYS AS (display_name) VIRTUAL`,
		`ALTER TABLE users ADD COLUMN shadow_banned TEXT NOT NULL DEFAULT '0'`,
		`CREATE INDEX unexpected ON users(email)`,
		`UPDATE goose_db_version SET version_id=9 WHERE version_id=1`,
	} {
		t.Run(strings.Fields(statement)[0]+statement, func(t *testing.T) {
			// Given a historical database with unrecognized schema or history.
			conn, target := historicalDatabase(t)
			if _, err := conn.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
			// When startup attempts adoption.
			if _, err := legacyBaseline(t.Context(), target); err == nil {
				t.Fatal("accepted drift")
			}
			// Then no compatibility columns have been written.
			var count int
			if err := conn.QueryRowContext(t.Context(), `SELECT count(*) FROM pragma_table_info('comments') WHERE name IN ('upvotes','flags')`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("wrote compatibility columns before validating drift")
			}
		})
	}
}

func TestHistoricalGooseDatabaseUpgradesWithAtlas(t *testing.T) {
	binary := os.Getenv("ATLAS_TEST_ATLAS_PATH")
	if binary == "" {
		t.Skip("requires pinned Linux Atlas integration binary")
	}
	checksum, err := pinnedChecksum("linux", runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(Config{BinaryPath: binary, Checksum: checksum})
	if err != nil {
		t.Fatal(err)
	}
	// Given the historical production DDL and a synthetic existing user.
	conn, targetURL := historicalDatabase(t)
	if _, err := conn.ExecContext(t.Context(), `INSERT INTO users(id,display_name) VALUES ('existing','Preserve me')`); err != nil {
		t.Fatal(err)
	}
	// When the real Atlas runner upgrades the database twice.
	for boot := range 2 {
		outcome, err := runner.ApplyExisting(t.Context(), Target{AccountID: "historical", TargetURL: targetURL})
		if err != nil {
			t.Fatalf("boot %d: %+v %v", boot, outcome, err)
		}
		if boot == 1 && outcome.Applied != 0 {
			t.Fatalf("second boot reapplied %d migrations", outcome.Applied)
		}
	}
	// Then the existing user has both baseline and new migration fields intact.
	var name string
	var shadow, dashboard, embed int
	if err := conn.QueryRowContext(t.Context(), `SELECT display_name,shadow_banned,dashboard_session_generation,embed_session_generation FROM users WHERE id='existing'`).Scan(&name, &shadow, &dashboard, &embed); err != nil {
		t.Fatal(err)
	}
	if name != "Preserve me" || shadow != 0 || dashboard != 0 || embed != 0 {
		t.Fatal("historical user changed")
	}
}
