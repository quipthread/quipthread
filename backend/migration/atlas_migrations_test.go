package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"ariga.io/atlas/sql/migrate"
	_ "modernc.org/sqlite"
)

func TestAtlasRunnerSeparatesTargetFromArgumentsAndCleansTempFiles(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "record")
	binaryPath := fakeAtlasBinary(t, recordPath, "success")
	checksum := fileSHA256(t, binaryPath)
	workRoot := t.TempDir()
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: checksum, TempRoot: workRoot})
	if err != nil {
		t.Fatal(err)
	}
	const token = "token-secret-+/="
	outcome, err := runner.Apply(context.Background(), Target{AccountID: "account-a", TargetURL: "libsql://tenant.example.test", AuthToken: token})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ExpectedVersion != "00011" {
		t.Fatalf("expected migration version = %q", outcome.ExpectedVersion)
	}
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "url=libsql://tenant.example.test?authToken="+url.QueryEscape(token)) {
		t.Fatalf("child environment did not receive authenticated target: %q", redactMigrationTestOutput(text, token))
	}
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "args=") {
			continue
		}
		args := strings.TrimPrefix(line, "args=")
		if strings.Contains(args, "tenant.example.test") || strings.Contains(args, token) {
			t.Fatalf("authenticated target leaked into argv: %q", redactMigrationTestOutput(args, token))
		}
	}
	if strings.Contains(text, "format =") || !strings.Contains(text, "format=goose") {
		t.Fatalf("Goose format was not confined to the directory URL: %q", redactMigrationTestOutput(text, token))
	}
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runner temp root entries after success = %v", entries)
	}
}

func TestAtlasRunnerPinnedCLIAppliesAllSQLiteMigrationsAndIsIdempotent(t *testing.T) {
	binaryPath := os.Getenv("ATLAS_TEST_ATLAS_PATH")
	if binaryPath == "" {
		binaryPath = AtlasCLIPath
	}
	if runtime.GOOS != "linux" && os.Getenv("ATLAS_TEST_ATLAS_PATH") == "" {
		t.Skip("the pinned Atlas CLI contract runs against the Linux release binary")
	}
	if _, err := os.Stat(binaryPath); errors.Is(err, os.ErrNotExist) {
		t.Skipf("pinned Atlas CLI is not installed at %s", binaryPath)
	} else if err != nil {
		t.Fatal(err)
	}
	checksum, err := pinnedChecksum("linux", runtime.GOARCH)
	if err != nil {
		t.Skipf("no pinned Atlas CLI for %s: %v", runtime.GOARCH, err)
	}
	workRoot := t.TempDir()
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: checksum, TempRoot: workRoot})
	if err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(t.TempDir(), "contract.db")
	first, err := runner.Apply(context.Background(), Target{AccountID: "pinned-cli-contract", TargetURL: "sqlite://" + databasePath})
	if err != nil {
		t.Fatalf("first pinned Atlas apply failed: outcome=%#v err=%v", first, err)
	}
	if first.Applied != 11 || first.ExpectedVersion != "00011" {
		t.Fatalf("first apply outcome=%#v, want eleven migrations through 00011", first)
	}
	second, err := runner.Apply(context.Background(), Target{AccountID: "pinned-cli-contract", TargetURL: "sqlite://" + databasePath})
	if err != nil {
		t.Fatalf("second pinned Atlas apply failed: outcome=%#v err=%v", second, err)
	}
	if second.Applied != 0 || second.ExpectedVersion != "00011" {
		t.Fatalf("second apply outcome=%#v, want no-op at 00011", second)
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT version FROM atlas_schema_revisions ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var revisions []string
	for rows.Next() {
		var revision string
		if err := rows.Scan(&revision); err != nil {
			t.Fatal(err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantRevisions := []string{"00001", "00002", "00003", "00004", "00005", "00006", "00007", "00008", "00009", "00010", "00011"}
	if strings.Join(revisions, ",") != strings.Join(wantRevisions, ",") {
		t.Fatalf("revision IDs = %v, want %v", revisions, wantRevisions)
	}

	wantTables := []string{
		"sites", "users", "user_identities", "comments", "approval_tokens", "email_tokens",
		"subscriptions", "blocked_terms", "comment_votes", "comment_flags", "notification_outbox",
		"notification_outbox_channels", "notification_outbox_comments",
	}
	for _, table := range wantTables {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("application table %q is missing", table)
		}
	}
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runner temp root entries after pinned contract = %v", entries)
	}
}

func TestAtlasRunnerDoesNotStartAChildForPreCancelledContext(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "record")
	binaryPath := fakeAtlasBinary(t, recordPath, "success")
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: fileSHA256(t, binaryPath), TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runner.Apply(ctx, Target{AccountID: "pre-cancelled", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"})
	if result.Kind != OutcomeConfigurationFailed || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled outcome=%#v err=%v", result, err)
	}
	if _, err := os.Stat(recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-cancelled runner started child, stat error=%v", err)
	}
}

func TestAtlasRunnerPreservesCancellationBeforeMigrationApply(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "record")
	binaryPath := fakeAtlasBinary(t, recordPath, "version-cancel")
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: fileSHA256(t, binaryPath), TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, err := runner.Apply(ctx, Target{AccountID: "version-cancel", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"})
	if result.Kind != OutcomeConfigurationFailed || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pre-migration cancellation outcome=%#v err=%v", result, err)
	}
	entries, err := os.ReadDir(runner.tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runner temp root entries after pre-migration cancellation = %v", entries)
	}
}

func TestAtlasRunnerDoesNotLeakCredentialsInRejectedErrors(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "record")
	binaryPath := fakeAtlasBinary(t, recordPath, "reject")
	token := "rejected-secret-+/="
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: fileSHA256(t, binaryPath), TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Apply(context.Background(), Target{AccountID: "rejected", TargetURL: "libsql://tenant.example.test", AuthToken: token})
	if result.Kind != OutcomeMigrationRejected || err == nil {
		t.Fatalf("rejected outcome=%#v err=%v", result, err)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "tenant.example.test") {
		t.Fatalf("migration error leaked target credentials: %q", err)
	}
	entries, err := os.ReadDir(runner.tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runner temp root entries after rejection = %v", entries)
	}
}

func TestAtlasRunnerChecksBinaryAndVersion(t *testing.T) {
	binaryPath := fakeAtlasBinary(t, filepath.Join(t.TempDir(), "record"), "success")
	if _, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: strings.Repeat("0", sha256.Size*2)}); err != nil {
		t.Fatal("constructor should defer binary checksum verification until Apply")
	}
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: strings.Repeat("0", sha256.Size*2)})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Apply(context.Background(), Request{AccountID: "account-a", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"})
	if result.Kind != OutcomeBinaryFailed || err == nil {
		t.Fatalf("bad checksum outcome=%#v err=%v", result, err)
	}
	runner, err = NewRunner(Config{BinaryPath: binaryPath, Version: "9.9.9", Checksum: fileSHA256(t, binaryPath)})
	if err != nil {
		t.Fatal(err)
	}
	result, err = runner.Apply(context.Background(), Request{AccountID: "account-a", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"})
	if result.Kind != OutcomeBinaryFailed || err == nil {
		t.Fatalf("bad version outcome=%#v err=%v", result, err)
	}
}

func TestAtlasRunnerSerializesSameAccountInvocations(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "record")
	binaryPath := fakeAtlasBinary(t, recordPath, "serialize")
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: fileSHA256(t, binaryPath), TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, runErr := runner.Apply(context.Background(), Request{AccountID: "same-account", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"})
			results <- runErr
		}()
	}
	wg.Wait()
	close(results)
	for runErr := range results {
		if runErr != nil {
			t.Fatal(runErr)
		}
	}
}

func TestAtlasRunnerCancellationWhileWaitingDoesNotStartChild(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), "record")
	binaryPath := fakeAtlasBinary(t, recordPath, "cancel")
	runner, err := NewRunner(Config{BinaryPath: binaryPath, Version: AtlasCLIVersion, Checksum: fileSHA256(t, binaryPath), TempRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan struct{})
	firstCtx, firstCancel := context.WithCancel(t.Context())
	go func() {
		defer close(firstDone)
		_, _ = runner.Apply(firstCtx, Target{AccountID: "wait-account", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"})
	}()
	t.Cleanup(func() {
		firstCancel()
		select {
		case <-firstDone:
		case <-time.After(5 * time.Second):
			t.Error("first migration child did not stop during cleanup")
		}
	})
	for {
		if _, err := os.Stat(recordPath); err == nil {
			break
		}
		select {
		case <-firstDone:
			t.Fatal("first migration returned before the lease wait")
		default:
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, err := runner.Apply(ctx, Target{AccountID: "wait-account", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"})
	if result.Kind != OutcomeConfigurationFailed || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait cancellation outcome=%#v err=%v", result, err)
	}
	if err := os.Remove(recordPath); err != nil {
		t.Fatal(err)
	}
}

func TestAtlasRunnerRejectsUnsafeTargetsAndVerifiesMigrationChecksums(t *testing.T) {
	runner, err := NewRunner(Config{BinaryPath: "/missing/atlas", Version: AtlasCLIVersion, Checksum: strings.Repeat("0", sha256.Size*2)})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []Request{
		{AccountID: "../account", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"},
		{AccountID: "account with spaces", TargetURL: "libsql://tenant.example.test", AuthToken: "secret"},
		{AccountID: "a", TargetURL: "libsql://tenant.example.test?authToken=leak", AuthToken: "secret"},
		{AccountID: "a", TargetURL: "libsql://tenant.example.test/path", AuthToken: "secret"},
		{AccountID: "a", TargetURL: "libsql://tenant.example.test", AuthToken: ""},
	} {
		result, err := runner.Apply(context.Background(), target)
		if result.Kind != OutcomeConfigurationFailed || err == nil {
			t.Fatalf("unsafe target outcome=%#v err=%v", result, err)
		}
	}
	if err := verifyMigrationIntegrity(); err != nil {
		t.Fatal(err)
	}
}

func TestAtlasMigrationIntegrityArtifactMatchesAtlasSum(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "migrations")
	if err := materializeMigrations(dir); err != nil {
		t.Fatal(err)
	}
	local, err := migrate.NewLocalDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := local.Checksum()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "atlas.sum"))
	if err != nil {
		t.Fatal(err)
	}
	var expected migrate.HashFile
	if err := expected.UnmarshalText(data); err != nil {
		t.Fatalf("parse committed atlas.sum: %v", err)
	}
	if actual.Sum() != expected.Sum() || len(actual) != 11 || len(expected) != len(actual) {
		t.Fatalf("Atlas migration checksum mismatch: actual=%s expected=%s", actual.Sum(), expected.Sum())
	}
}

func fakeAtlasBinary(t *testing.T, recordPath, mode string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "atlas")
	script := "#!/bin/sh\n" +
		"printf 'args=%s\\nurl=%s\\n' \"$*\" \"${ATLAS_DATABASE_URL-}\" >> " + shellQuote(recordPath) + "\n" +
		"if [ \"$ATLAS_FAKE_MODE\" = version-cancel ] && [ \"$1\" = version ]; then exec sleep 10; fi\n" +
		"if [ \"$1\" = version ]; then printf 'atlas version v" + AtlasCLIVersion + "\\n'; exit 0; fi\n" +
		"if [ ! -f \"$PWD/atlas.hcl\" ] || [ ! -f \"$PWD/migrations/atlas.sum\" ] || [ ! -f \"$PWD/migrations/00001_baseline.sql\" ]; then exit 1; fi\n" +
		"if [ \"$ATLAS_DATABASE_URL\" = 'libsql://cancel.example.test?authToken=secret' ]; then exec sleep 10; fi\n" +
		"if [ \"$1\" = migrate ] && [ \"$2\" = apply ] && [ \"$ATLAS_FAKE_MODE\" = reject ]; then printf 'secret rejected target=libsql://tenant.example.test\\n' >&2; exit 1; fi\n" +
		"if [ \"$1\" = migrate ] && [ \"$2\" = apply ] && [ \"$ATLAS_FAKE_MODE\" = serialize ]; then if [ -e " + shellQuote(recordPath+".marker") + " ]; then exit 1; fi; touch " + shellQuote(recordPath+".marker") + "; sleep 0.1; rm -f " + shellQuote(recordPath+".marker") + "; fi\n" +
		"if [ \"$1\" = migrate ] && [ \"$2\" = apply ]; then printf '{\"Applied\":[],\"Current\":\"00009\",\"Target\":\"00009\"}\\n'; exit 0; fi\n" +
		"exit 1\n"
	if mode == "cancel" {
		script = strings.Replace(script, "if [ \"$ATLAS_DATABASE_URL\" = 'libsql://cancel.example.test?authToken=secret' ]", "if [ \"$2\" = apply ]", 1)
	}
	if mode == "reject" || mode == "serialize" || mode == "version-cancel" {
		script = "#!/bin/sh\nATLAS_FAKE_MODE=" + shellQuote(mode) + "\n" + strings.TrimPrefix(script, "#!/bin/sh\n")
	}
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func redactMigrationTestOutput(output string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			output = strings.ReplaceAll(output, secret, "[REDACTED]")
			output = strings.ReplaceAll(output, url.QueryEscape(secret), "[REDACTED]")
		}
	}
	return output
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
