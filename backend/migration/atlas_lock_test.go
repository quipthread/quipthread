package migration

import (
	"database/sql"
	"fmt"
	"hash/adler32"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"ariga.io/atlas/sql/sqlite"
)

func TestAtlasRunnerSQLiteLockProtectsOnlyItsDatabase(t *testing.T) {
	binary := os.Getenv("ATLAS_TEST_ATLAS_PATH")
	if binary == "" {
		if runtime.GOOS != "linux" {
			t.Skip("pinned Atlas integration runs on Linux")
		}
		binary = AtlasCLIPath
	}
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Skip("pinned Atlas CLI is not installed")
	}
	checksum, err := pinnedChecksum("linux", runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(Config{BinaryPath: binary, Checksum: checksum})
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "tenant.db")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias.db")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	dirAlias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, dirAlias); err != nil {
		t.Fatal(err)
	}
	lockName := fmt.Sprintf("atlas_migrate_execute_%x", adler32.Checksum([]byte(filepath.Join("file:", path))))
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	driver, err := sqlite.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := driver.Lock(t.Context(), lockName, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unlock(); err != nil {
			t.Error(err)
		}
	})
	for _, tc := range []struct {
		name string
		path string
		want OutcomeKind
	}{
		{"different database", filepath.Join(root, "other.db"), OutcomeSucceeded},
		{"same database", path, OutcomeMigrationRejected},
		{"cleaned path", root + "/./tenant.db", OutcomeMigrationRejected},
		{"file symlink", alias, OutcomeMigrationRejected},
		{"directory symlink", filepath.Join(dirAlias, "tenant.db"), OutcomeMigrationRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome, err := runner.Apply(t.Context(), Target{AccountID: "lock-contract", TargetURL: "sqlite://" + tc.path})
			if outcome.Kind != tc.want || (err == nil) != (tc.want == OutcomeSucceeded) {
				t.Fatalf("outcome=%#v err=%v, want %s", outcome, err, tc.want)
			}
		})
	}
}

func TestSQLiteLockIdentitySurvivesDatabaseCreation(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "new.db")
	before, err := atlasDatabaseURL("sqlite://" + filepath.Join(alias, "new.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	after, err := atlasDatabaseURL("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("creating the database changed its migration lock identity")
	}
}
