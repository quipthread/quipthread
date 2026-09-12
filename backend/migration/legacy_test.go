package migration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExistingDatabaseUpgradePreservesLegacyData(t *testing.T) {
	binary := os.Getenv("ATLAS_TEST_ATLAS_PATH")
	if binary == "" {
		if runtime.GOOS != "linux" {
			t.Skip("pinned Atlas integration runs on Linux")
		}
		binary = AtlasCLIPath
	}
	checksum, err := pinnedChecksum("linux", runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(Config{BinaryPath: binary, Checksum: checksum})
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"unversioned baseline", "Goose baseline", "ALTER-appended column"} {
		t.Run(variant, func(t *testing.T) {
			// Given the schema shipped before Atlas, with real records.
			path := filepath.Join(t.TempDir(), "legacy.db")
			conn, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			baseline, err := migrationAssets.ReadFile("assets/00001_baseline.sql")
			if err != nil {
				t.Fatal(err)
			}
			if variant == "ALTER-appended column" {
				baseline = []byte(strings.Replace(string(baseline), "    notify_interval  INTEGER,\n", "", 1))
			}
			if _, err := conn.ExecContext(t.Context(), strings.SplitN(string(baseline), "-- +goose Down", 2)[0]); err != nil {
				t.Fatal(err)
			}
			if variant == "ALTER-appended column" {
				if _, err := conn.ExecContext(t.Context(), `ALTER TABLE sites ADD COLUMN notify_interval INTEGER`); err != nil {
					t.Fatal(err)
				}
			}
			if variant == "Goose baseline" {
				if _, err := conn.ExecContext(t.Context(), `CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY AUTOINCREMENT, version_id INTEGER NOT NULL, is_applied INTEGER NOT NULL, tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP); INSERT INTO goose_db_version (version_id,is_applied) VALUES (0,1),(1,1)`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := conn.ExecContext(t.Context(), `INSERT INTO users(id,display_name,email) VALUES ('old-user','Existing user','old@example.invalid'); INSERT INTO sites(id,owner_id,domain) VALUES ('old-site','old-user','example.invalid'); INSERT INTO comments(id,site_id,page_id,user_id,content) VALUES ('old-comment','old-site','/','old-user','Preserve this comment')`); err != nil {
				t.Fatal(err)
			}
			// When the existing database is upgraded twice, as on successive boots.
			target := Target{AccountID: "legacy", TargetURL: "sqlite://" + path}
			for boot := range 2 {
				outcome, err := runner.ApplyExisting(context.Background(), target)
				if err != nil {
					t.Fatalf("boot %d: outcome=%+v error=%v", boot, outcome, err)
				}
				if boot == 1 && outcome.Applied != 0 {
					t.Fatalf("second boot reapplied %d migrations", outcome.Applied)
				}
			}
			// Then existing data remains accessible through the new schema.
			var content, email string
			var dashboardGeneration, embedGeneration int
			if err := conn.QueryRowContext(t.Context(), `SELECT c.content,u.email,u.dashboard_session_generation,u.embed_session_generation FROM comments c JOIN users u ON u.id=c.user_id WHERE c.id='old-comment'`).Scan(&content, &email, &dashboardGeneration, &embedGeneration); err != nil {
				t.Fatal(err)
			}
			if content != "Preserve this comment" || email != "old@example.invalid" || dashboardGeneration != 0 || embedGeneration != 0 {
				t.Fatal("legacy records changed")
			}
		})
	}
}

func TestExistingDatabaseUpgradeRejectsUnrecognizedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unrelated.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(t.Context(), `CREATE TABLE unrelated (value TEXT); INSERT INTO unrelated VALUES ('preserve')`); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(t.TempDir(), "record")
	binary := fakeAtlasBinary(t, record, "success")
	runner, err := NewRunner(Config{BinaryPath: binary, Checksum: fileSHA256(t, binary)})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := runner.ApplyExisting(t.Context(), Target{AccountID: "unknown", TargetURL: "sqlite://" + path})
	if err == nil || outcome.Kind != OutcomeMigrationRejected {
		t.Fatalf("unrecognized schema accepted: %+v %v", outcome, err)
	}
	var value string
	if err := conn.QueryRowContext(t.Context(), `SELECT value FROM unrelated`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "preserve" {
		t.Fatal("unrecognized schema was modified")
	}
}

func TestExistingDatabaseUpgradeRejectsUnknownSchemaObjects(t *testing.T) {
	for _, definition := range []string{
		`CREATE VIEW unexpected AS SELECT id FROM users`,
		`CREATE TRIGGER unexpected AFTER INSERT ON users BEGIN DELETE FROM users; END`,
		`CREATE INDEX unexpected ON users(email)`,
	} {
		t.Run(strings.Fields(definition)[1], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.db")
			conn, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			baseline, err := migrationAssets.ReadFile("assets/00001_baseline.sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := conn.ExecContext(t.Context(), string(baseline)); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.ExecContext(t.Context(), definition); err != nil {
				t.Fatal(err)
			}
			binary := fakeAtlasBinary(t, filepath.Join(t.TempDir(), "record"), "success")
			runner, err := NewRunner(Config{BinaryPath: binary, Checksum: fileSHA256(t, binary)})
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := runner.ApplyExisting(t.Context(), Target{AccountID: "unknown-object", TargetURL: "sqlite://" + path})
			if err == nil || outcome.Kind != OutcomeMigrationRejected {
				t.Fatalf("unrecognized object accepted: %+v %v", outcome, err)
			}
		})
	}
}
