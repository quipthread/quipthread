package migration

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"ariga.io/atlas/atlasexec"
)

//go:embed assets/* atlas_migrations.sha256 atlas_cli.sha256
var migrationAssets embed.FS

const (
	AtlasCLIVersion  = "1.3.0"
	AtlasCLIPath     = "/usr/local/bin/atlas"
	AtlasDatabaseURL = "ATLAS_DATABASE_URL"

	OutcomeSucceeded           OutcomeKind = "succeeded"
	OutcomeConfigurationFailed OutcomeKind = "configuration_failure"
	OutcomeBinaryFailed        OutcomeKind = "binary_failure"
	OutcomeMigrationRejected   OutcomeKind = "migration_rejected"
	OutcomeMigrationUnknown    OutcomeKind = "migration_outcome_unknown"
)

type OutcomeKind string

type Outcome struct {
	Kind            OutcomeKind
	Applied         int
	ExpectedVersion string
}

type OutcomeError struct {
	Kind  OutcomeKind
	cause error
}

func (e *OutcomeError) Error() string {
	switch e.Kind {
	case OutcomeConfigurationFailed:
		return "migration configuration failed"
	case OutcomeBinaryFailed:
		return "migration binary verification failed"
	case OutcomeMigrationRejected:
		return "migration was rejected"
	case OutcomeMigrationUnknown:
		return "migration outcome is unknown"
	default:
		return "migration failed"
	}
}

func (e *OutcomeError) Unwrap() error { return e.cause }

type Config struct {
	BinaryPath string
	Version    string
	Checksum   string
	TempRoot   string
	LeaseWait  time.Duration
}

// Target identifies one account's database. AuthToken is deliberately not
// used to construct command-line arguments or the Atlas configuration file;
// it is only placed in the child process environment by Apply.
type Target struct {
	AccountID string
	TargetURL string
	AuthToken string
}

// Request is retained as an alias for callers of the initial runner API.
// Apply's public contract is Target, so new callers should use Target.
type Request = Target

type Runner struct {
	binaryPath string
	version    string
	checksum   string
	tempRoot   string
	leaseWait  time.Duration
}

type accountLease struct {
	token chan struct{}
	refs  int
}

// accountLocks is intentionally package-wide: creating a second Runner in the
// same process must not allow two migrations for one account to overlap.
var accountLocks = struct {
	sync.Mutex
	locks map[string]*accountLease
}{locks: make(map[string]*accountLease)}

func NewRunner(cfg Config) (*Runner, error) {
	if cfg.LeaseWait < 0 {
		return nil, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	path := cfg.BinaryPath
	if path == "" {
		path = AtlasCLIPath
	}
	if !filepath.IsAbs(path) {
		return nil, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	version := strings.TrimPrefix(cfg.Version, "v")
	if version == "" {
		version = AtlasCLIVersion
	}
	if !validVersion(version) {
		return nil, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	checksum := strings.ToLower(cfg.Checksum)
	if checksum == "" {
		var err error
		checksum, err = pinnedChecksum(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return nil, &OutcomeError{Kind: OutcomeConfigurationFailed}
		}
	}
	if !isSHA256(checksum) {
		return nil, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	return &Runner{binaryPath: filepath.Clean(path), version: version, checksum: checksum, tempRoot: cfg.TempRoot, leaseWait: cfg.LeaseWait}, nil
}

func (r *Runner) Apply(ctx context.Context, target Target) (Outcome, error) {
	return r.apply(ctx, target, false)
}

func (r *Runner) ApplyExisting(ctx context.Context, target Target) (Outcome, error) {
	return r.apply(ctx, target, true)
}

func (r *Runner) apply(ctx context.Context, target Target, adoptLegacy bool) (Outcome, error) {
	if ctx == nil || !validAccountID(target.AccountID) {
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	databaseURL, err := normalizeTarget(target.TargetURL, target.AuthToken)
	if err != nil {
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	if err := ctx.Err(); err != nil {
		return configurationOutcome(err)
	}
	if err := r.acquire(ctx, target.AccountID); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return configurationOutcome(ctxErr)
		}
		return configurationOutcome(nil)
	}
	defer r.release(target.AccountID)

	if err := verifyBinary(r.binaryPath, r.checksum); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return configurationOutcome(ctxErr)
		}
		return Outcome{Kind: OutcomeBinaryFailed}, &OutcomeError{Kind: OutcomeBinaryFailed}
	}
	if err := verifyMigrationIntegrity(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return configurationOutcome(ctxErr)
		}
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	expectedVersion, err := latestMigrationVersion()
	if err != nil {
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	workDir, err := os.MkdirTemp(r.tempRoot, "quipthread-atlas-")
	if err != nil {
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	defer os.RemoveAll(workDir)                     //nolint:errcheck // cleanup is best effort after the outcome is recorded
	if err := os.Chmod(workDir, 0700); err != nil { //nolint:gosec // owner execute permission is required to traverse this private directory
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	migrationDir := filepath.Join(workDir, "migrations")
	if err := materializeMigrations(migrationDir); err != nil {
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	configPath := filepath.Join(workDir, "atlas.hcl")
	config := fmt.Sprintf("env \"tenant\" {\n  url = getenv(%q)\n}\n", AtlasDatabaseURL)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return configurationOutcome(ctxErr)
		}
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	if err := ctx.Err(); err != nil {
		return configurationOutcome(err)
	}

	client, err := atlasexec.NewClient(workDir, r.binaryPath)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return configurationOutcome(ctxErr)
		}
		return Outcome{Kind: OutcomeBinaryFailed}, &OutcomeError{Kind: OutcomeBinaryFailed}
	}
	if err := client.SetEnv(atlasexec.Environ{
		AtlasDatabaseURL: databaseURL,
	}); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return configurationOutcome(ctxErr)
		}
		return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed}
	}
	version, err := client.Version(ctx)
	if err != nil || version == nil || version.Version != r.version {
		if ctx.Err() != nil {
			return configurationOutcome(ctx.Err())
		}
		return Outcome{Kind: OutcomeBinaryFailed}, &OutcomeError{Kind: OutcomeBinaryFailed}
	}
	if err := ctx.Err(); err != nil {
		return configurationOutcome(err)
	}
	baseline := ""
	if adoptLegacy {
		baseline, err = legacyBaseline(ctx, databaseURL)
		if err != nil {
			if ctx.Err() != nil {
				return configurationOutcome(ctx.Err())
			}
			return Outcome{Kind: OutcomeMigrationRejected}, &OutcomeError{Kind: OutcomeMigrationRejected}
		}
	}
	result, err := client.MigrateApply(ctx, &atlasexec.MigrateApplyParams{
		ConfigURL:       "file://" + configPath,
		Env:             "tenant",
		DirURL:          "file://" + migrationDir + "?format=goose",
		BaselineVersion: baseline,
	})
	if err != nil {
		if ctx.Err() != nil {
			return unknownOutcome(ctx.Err())
		}
		return Outcome{Kind: OutcomeMigrationRejected}, &OutcomeError{Kind: OutcomeMigrationRejected}
	}
	if result == nil || result.Error != "" {
		return Outcome{Kind: OutcomeMigrationRejected}, &OutcomeError{Kind: OutcomeMigrationRejected}
	}
	return Outcome{Kind: OutcomeSucceeded, Applied: len(result.Applied), ExpectedVersion: expectedVersion}, nil
}

func configurationOutcome(cause error) (Outcome, error) {
	return Outcome{Kind: OutcomeConfigurationFailed}, &OutcomeError{Kind: OutcomeConfigurationFailed, cause: cause}
}

func unknownOutcome(cause error) (Outcome, error) {
	return Outcome{Kind: OutcomeMigrationUnknown}, &OutcomeError{Kind: OutcomeMigrationUnknown, cause: cause}
}

func normalizeTarget(raw, authToken string) (string, error) {
	if raw == "" || strings.IndexFunc(raw, func(r rune) bool { return r <= ' ' || r == '\x7f' }) >= 0 {
		return "", errors.New("invalid migration target")
	}
	if strings.IndexByte(authToken, 0) >= 0 {
		return "", errors.New("invalid migration credential")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("invalid migration target")
	}
	if u.Scheme != "libsql" && u.Scheme != "https" && u.Scheme != "sqlite" {
		return "", errors.New("invalid migration target")
	}
	if u.Scheme == "sqlite" {
		if authToken != "" || u.Host != "" || u.Path == "" {
			return "", errors.New("sqlite migration target cannot have auth")
		}
		return raw, nil
	}
	if u.Host == "" || u.Hostname() == "" || authToken == "" {
		return "", errors.New("remote migration target requires auth")
	}
	if u.Port() != "" {
		if _, err := strconv.ParseUint(u.Port(), 10, 16); err != nil {
			return "", errors.New("invalid migration target")
		}
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("migration target path is not supported")
	}
	u.Path, u.RawPath = "", ""
	q := u.Query()
	q.Set("authToken", authToken)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func validAccountID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for i, char := range []byte(value) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || (i > 0 && (char == '-' || char == '_' || char == '.')) {
			continue
		}
		return false
	}
	return true
}

func validVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}

func verifyBinary(path, expected string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return errors.New("atlas binary is unavailable")
	}
	f, err := os.Open(path) //nolint:gosec // configured executable path is read only to verify its pinned checksum
	if err != nil {
		return errors.New("atlas binary is unavailable")
	}
	defer f.Close() //nolint:errcheck // read-only cleanup
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return errors.New("atlas binary checksum failed")
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		return errors.New("atlas binary checksum mismatch")
	}
	return nil
}

func materializeMigrations(dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	entries, err := migrationAssets.ReadDir("assets")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "atlas.sum" || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := migrationAssets.ReadFile(filepath.Join("assets", entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, entry.Name()), data, 0600); err != nil {
			return err
		}
	}
	data, err := migrationAssets.ReadFile("assets/atlas.sum")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "atlas.sum"), data, 0600); err != nil {
		return err
	}
	return nil
}

func latestMigrationVersion() (string, error) {
	entries, err := migrationAssets.ReadDir("assets")
	if err != nil {
		return "", err
	}
	latest := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version := strings.SplitN(strings.TrimSuffix(entry.Name(), ".sql"), "_", 2)[0]
		if _, err := strconv.ParseUint(version, 10, 64); err != nil {
			return "", errors.New("migration version is invalid")
		}
		if version > latest {
			latest = version
		}
	}
	if latest == "" {
		return "", errors.New("no migrations found")
	}
	return latest, nil
}

func (r *Runner) acquire(ctx context.Context, accountID string) error {
	accountLocks.Lock()
	lease, ok := accountLocks.locks[accountID]
	if !ok {
		lease = &accountLease{token: make(chan struct{}, 1)}
		accountLocks.locks[accountID] = lease
	}
	lease.refs++
	accountLocks.Unlock()
	waitCtx := ctx
	var cancel context.CancelFunc
	if r.leaseWait > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, r.leaseWait)
		defer cancel()
	}
	select {
	case lease.token <- struct{}{}:
		return nil
	case <-waitCtx.Done():
		r.releaseLease(accountID, lease)
		return waitCtx.Err()
	}
}

func (r *Runner) release(accountID string) {
	accountLocks.Lock()
	lease := accountLocks.locks[accountID]
	accountLocks.Unlock()
	<-lease.token
	r.releaseLease(accountID, lease)
}

func (r *Runner) releaseLease(accountID string, lease *accountLease) {
	accountLocks.Lock()
	lease.refs--
	if lease.refs == 0 && accountLocks.locks[accountID] == lease {
		delete(accountLocks.locks, accountID)
	}
	accountLocks.Unlock()
}

func pinnedChecksum(goos, goarch string) (string, error) { //nolint:unparam // OS and architecture vary across build targets, but are constant within each build.
	data, err := migrationAssets.ReadFile("atlas_cli.sha256")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == goos+"/"+goarch+"/atlas-linux-"+goarch+"-v"+AtlasCLIVersion {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no pinned Atlas checksum for %s/%s", goos, goarch)
}

func verifyMigrationIntegrity() error {
	data, err := migrationAssets.ReadFile("atlas_migrations.sha256")
	if err != nil {
		return err
	}
	expected := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || !isSHA256(fields[0]) || !strings.HasSuffix(fields[1], ".sql") {
			return errors.New("migration checksum artifact is invalid")
		}
		expected[fields[1]] = fields[0]
	}
	entries, err := migrationAssets.ReadDir("assets")
	if err != nil {
		return err
	}
	actual := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := migrationAssets.ReadFile(filepath.Join("assets", entry.Name()))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if expected[entry.Name()] != hex.EncodeToString(sum[:]) {
			return errors.New("migration checksum mismatch")
		}
		actual++
	}
	if actual != len(expected) {
		return errors.New("migration checksum artifact is incomplete")
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
