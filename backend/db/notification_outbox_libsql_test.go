package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/quipthread/quipthread/migration"
	"github.com/quipthread/quipthread/models"
)

const liveLibSQLOutboxConfig = "QUIPTHREAD_LIVE_LIBSQL_TEST_DSN, QUIPTHREAD_LIVE_LIBSQL_TEST_AUTH_TOKEN, QUIPTHREAD_LIVE_LIBSQL_TEST_DATABASE, and QUIPTHREAD_LIVE_LIBSQL_TEST_NONPROD=1"

func TestLiveLibSQLNotificationOutboxClaimContract(t *testing.T) {
	dsn := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_DSN")
	rawDSN := dsn
	authToken := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_AUTH_TOKEN")
	databaseID := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_DATABASE")
	nonProd := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_NONPROD")
	if nonProd != "1" {
		t.Skip("skipping live libSQL/Turso contract test: opt in with " + liveLibSQLOutboxConfig)
	}
	if strings.TrimSpace(dsn) == "" || strings.TrimSpace(authToken) == "" || strings.TrimSpace(databaseID) == "" {
		t.Fatalf("live libSQL/Turso contract test opted in but requires %s", liveLibSQLOutboxConfig)
	}
	dsn, err := validatedLiveLibSQLDSN(dsn, authToken, databaseID)
	if err != nil {
		t.Fatalf("live libSQL/Turso contract test opted in with invalid configuration: %v; required configuration: %s", err, liveLibSQLOutboxConfig)
	}
	runner, err := migration.NewRunner(migration.Config{})
	if err != nil {
		t.Fatalf("configure reviewed Atlas live migration runner: %s", redactLiveContractError(err, rawDSN, authToken))
	}
	firstApply, err := runner.Apply(context.Background(), migration.Target{
		AccountID: "live-libsql-contract",
		TargetURL: dsn,
		AuthToken: authToken,
	})
	if err != nil {
		t.Fatalf("apply Atlas migrations to fresh non-production target: %s", redactLiveContractError(err, rawDSN, authToken))
	}
	if firstApply.Applied != 9 || firstApply.ExpectedVersion != "00009" {
		t.Fatalf("first Atlas apply outcome = %#v, want nine migrations through revision 00009", firstApply)
	}
	secondApply, err := runner.Apply(context.Background(), migration.Target{
		AccountID: "live-libsql-contract",
		TargetURL: dsn,
		AuthToken: authToken,
	})
	if err != nil {
		t.Fatalf("second Atlas apply failed: %s", redactLiveContractError(err, rawDSN, authToken))
	}
	if secondApply.Applied != 0 || secondApply.ExpectedVersion != "00009" {
		t.Fatalf("second Atlas apply outcome = %#v, want no-op at revision 00009", secondApply)
	}

	first, err := NewLibSQLStoreWithAuthToken(dsn, authToken)
	if err != nil {
		t.Fatalf("open configured non-production libSQL test database: %s", redactLiveContractError(err, rawDSN, authToken))
	}
	defer first.Close() //nolint:errcheck,gosec // test cleanup
	second, err := NewLibSQLStoreWithAuthToken(dsn, authToken)
	if err != nil {
		t.Fatalf("open second configured non-production libSQL store: %s", redactLiveContractError(err, rawDSN, authToken))
	}
	defer second.Close() //nolint:errcheck,gosec // test cleanup
	var existingRows int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox`).Scan(&existingRows); err != nil {
		t.Fatalf("verify dedicated live test database: %s", redactLiveContractError(err, rawDSN, authToken))
	}
	if existingRows != 0 {
		t.Fatalf("refusing live contract test database %q because it contains %d pre-existing outbox rows; use a fresh dedicated database", databaseID, existingRows)
	}
	assertLiveLibSQLLatestSchema(t, first)
	assertLiveAtlasRevision(t, first, rawDSN, authToken)
	if err := first.Close(); err != nil {
		t.Fatalf("close live store before reopen: %v", err)
	}
	first, err = NewLibSQLStoreWithAuthToken(dsn, authToken)
	if err != nil {
		t.Fatalf("reopen configured non-production libSQL test database: %s", redactLiveContractError(err, rawDSN, authToken))
	}
	defer first.Close() //nolint:errcheck,gosec // test cleanup
	assertLiveLibSQLLatestSchema(t, first)
	inventory, err := OpenLibSQLLegacyDigestInventory(rawDSN, authToken)
	if err != nil {
		t.Fatalf("open live read-only legacy inventory: %v", err)
	}
	defer inventory.Close() //nolint:errcheck,gosec // test cleanup
	legacyRows, err := inventory.ListUninitializedSiteDigestsContext(context.Background())
	if err != nil {
		t.Fatalf("read live legacy inventory: %v", err)
	}
	if len(legacyRows) != 0 {
		t.Fatalf("fresh live legacy inventory rows = %d, want 0", len(legacyRows))
	}

	key := "live-contract-" + uuid.NewString()
	item := outboxItem("live-test-site", "live-contract", key, time.Now().UTC())
	if err := first.EnqueueNotificationOutbox(item); err != nil {
		t.Fatalf("enqueue live contract item: %v", err)
	}
	operationSiteID := "live-operation-site-" + uuid.NewString()
	operationInterval := 60
	if err := first.CreateSite(&models.Site{ID: operationSiteID, OwnerID: "live-test-owner", Domain: operationSiteID + ".example", NotifyInterval: &operationInterval}); err != nil {
		t.Fatalf("create live operation site: %v", err)
	}
	// CreateSite intentionally persists only the base site fields. Persist the
	// interval explicitly so the operation exercises the configured-window path.
	if err := first.UpdateSite(&models.Site{ID: operationSiteID, Theme: "auto", NotifyInterval: &operationInterval}); err != nil {
		t.Fatalf("persist live operation site interval: %v", err)
	}
	for _, userID := range []string{"live-test-user-1", "live-test-user-2"} {
		if err := first.UpsertUser(&models.User{ID: userID, DisplayName: userID, Role: "commenter"}); err != nil {
			t.Fatalf("seed live operation user %q: %v", userID, err)
		}
	}
	// Keep this digest unavailable to the independent claim/recovery assertions
	// below. Those assertions operate on the separate live-contract row.
	operationCreated := time.Now().UTC().Add(time.Hour)
	operationComments := []*models.Comment{
		{ID: uuid.NewString(), SiteID: operationSiteID, PageID: "/live", UserID: "live-test-user-1", Content: "live operation one", Status: "pending", CreatedAt: operationCreated},
		{ID: uuid.NewString(), SiteID: operationSiteID, PageID: "/live", UserID: "live-test-user-2", Content: "live operation two", Status: "pending", CreatedAt: operationCreated},
	}
	operationStores := []*LibSQLStore{first, second}
	operationErrors := make(chan error, len(operationStores))
	var operationWG sync.WaitGroup
	for i, candidate := range operationStores {
		operationWG.Add(1)
		go func(candidate *LibSQLStore, comment *models.Comment) {
			defer operationWG.Done()
			operationErrors <- candidate.CreatePendingCommentWithNotification(comment)
		}(candidate, operationComments[i])
	}
	operationWG.Wait()
	close(operationErrors)
	for operationErr := range operationErrors {
		if operationErr != nil {
			t.Fatalf("concurrent live atomic comment create: %v", operationErr)
		}
	}
	operationWindowStart, _, err := siteDigestWindow(operationCreated, int64(operationInterval))
	if err != nil {
		t.Fatalf("derive live atomic comment window: %v", err)
	}
	operationKey := fmt.Sprintf("site-digest:%d:%d", operationInterval, operationWindowStart)
	operationOutbox, err := first.GetNotificationOutboxByKey(operationSiteID, NotificationOutboxKindSiteDigest, operationKey)
	if err != nil {
		t.Fatalf("read live atomic comment digest: %v", err)
	}
	if operationOutbox == nil || operationOutbox.Status != models.NotificationOutboxPending {
		t.Fatalf("live atomic comment digest = %#v, want pending row", operationOutbox)
	}
	var operationCommentCount int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE site_id = ?`, operationSiteID).Scan(&operationCommentCount); err != nil {
		t.Fatalf("count live atomic comments: %v", err)
	}
	if operationCommentCount != len(operationComments) {
		t.Fatalf("live atomic comments = %d, want %d", operationCommentCount, len(operationComments))
	}
	now := time.Now().UTC()
	type result struct {
		store  *LibSQLStore
		items  []*models.NotificationOutbox
		worker string
		err    error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, candidate := range []*LibSQLStore{first, second} {
		wg.Add(1)
		go func(candidate *LibSQLStore) {
			defer wg.Done()
			worker := "live-worker-" + uuid.NewString()
			claimed, claimErr := candidate.ClaimNotificationOutbox(worker, now, time.Minute, 1)
			results <- result{store: candidate, items: claimed, worker: worker, err: claimErr}
		}(candidate)
	}
	wg.Wait()
	close(results)

	var winner result
	winners := 0
	var winnerItem *models.NotificationOutbox
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent live claim: %v", result.err)
		}
		if len(result.items) > 1 {
			t.Fatalf("live claimer %s claimed %d rows with limit 1", result.worker, len(result.items))
		}
		for _, claimed := range result.items {
			if claimed.ID != item.ID {
				t.Fatalf("live claimer %s claimed row %q, want unique test row %q", result.worker, claimed.ID, item.ID)
			}
		}
		if len(result.items) == 1 {
			winner = result
			winnerItem = result.items[0]
			winners++
		}
	}
	if winners != 1 || winnerItem == nil || winnerItem.LeaseUntil == nil {
		t.Fatalf("concurrent live claim winners = %d, want exactly 1", winners)
	}

	recoveryStore := first
	if winner.store == first {
		recoveryStore = second
	}
	recoveryNow := winnerItem.LeaseUntil.Add(time.Second)
	recovered, err := recoveryStore.ClaimNotificationOutbox("live-recovery-worker", recoveryNow, time.Minute, 1)
	if err != nil {
		t.Fatalf("expired live lease recovery: %v", err)
	}
	if len(recovered) != 1 || recovered[0].ID != item.ID || recovered[0].Attempts != winnerItem.Attempts+1 {
		t.Fatalf("expired live lease recovery = %#v, want test row %q at generation %d", recovered, item.ID, winnerItem.Attempts+1)
	}
	if err := recoveryStore.AckNotificationOutbox(recovered[0].ID, "live-recovery-worker", recovered[0].Attempts, recoveryNow); err != nil {
		t.Fatalf("ack recovered live lease: %v", err)
	}

	if _, err := first.EnsureNotificationOutboxChannels(operationOutbox.ID, []string{"email", "slack"}); err != nil {
		t.Fatalf("create live digest channels: %v", err)
	}
	digestClaimNow := time.Now().UTC().Add(2 * time.Hour)
	digestClaim, err := first.ClaimNotificationOutbox("live-digest-worker", digestClaimNow, time.Hour, 1)
	if err != nil || len(digestClaim) != 1 || digestClaim[0].ID != operationOutbox.ID {
		t.Fatalf("claim live digest parent: %v, %#v", err, digestClaim)
	}
	digest, err := first.MaterializeClaimedSiteDigest(context.Background(), operationOutbox.ID, "live-digest-worker", digestClaim[0].Attempts, digestClaimNow)
	if err != nil || digest == nil || len(digest.Comments) != len(operationComments) {
		t.Fatalf("materialize live digest: %v, %#v", err, digest)
	}
	digestChannels, err := first.ClaimNotificationOutboxChannels(operationOutbox.ID, "live-digest-channel-worker", digestClaimNow, time.Minute, 10)
	if err != nil || len(digestChannels) != 2 {
		t.Fatalf("claim live digest channels: %v, %#v", err, digestChannels)
	}
	for _, channel := range digestChannels {
		if err := first.AckNotificationOutboxChannel(channel.ID, "live-digest-channel-worker", channel.Attempts, digestClaimNow); err != nil {
			t.Fatalf("ack live digest channel: %v", err)
		}
	}
	if err := first.FinalizeClaimedSiteDigest(context.Background(), operationOutbox.ID, "live-digest-worker", digestClaim[0].Attempts, digestClaimNow); err != nil {
		t.Fatalf("finalize live digest: %v", err)
	}

	channelParent := outboxItem("live-channel-site", "site_digest", "live-channel-parent-"+uuid.NewString(), time.Now().UTC())
	if err := first.EnqueueNotificationOutbox(channelParent); err != nil {
		t.Fatalf("enqueue live channel parent: %v", err)
	}
	if _, err := first.EnsureNotificationOutboxChannels(channelParent.ID, []string{"email", "slack", "webhook"}); err != nil {
		t.Fatalf("create live channel rows: %v", err)
	}
	channelNow := time.Now().UTC().Add(time.Second)
	type channelResult struct {
		items  []*models.NotificationOutboxChannel
		worker string
		err    error
	}
	channelResults := make(chan channelResult, 2)
	var channelWG sync.WaitGroup
	for i, candidate := range []*LibSQLStore{first, second} {
		channelWG.Add(1)
		go func(candidate *LibSQLStore, owner string) {
			defer channelWG.Done()
			claimed, claimErr := candidate.ClaimNotificationOutboxChannels(channelParent.ID, owner, channelNow, time.Minute, 10)
			channelResults <- channelResult{items: claimed, worker: owner, err: claimErr}
		}(candidate, "live-channel-worker-"+uuid.NewString()+fmt.Sprintf("-%d", i))
	}
	channelWG.Wait()
	close(channelResults)
	seenChannels := make(map[string]bool)
	for result := range channelResults {
		if result.err != nil {
			t.Fatalf("concurrent live channel claim: %v", result.err)
		}
		for _, channel := range result.items {
			if seenChannels[channel.ID] {
				t.Fatalf("live channel %q claimed twice", channel.ID)
			}
			seenChannels[channel.ID] = true
		}
	}
	if len(seenChannels) != 3 {
		t.Fatalf("live independent channel claims = %d, want 3", len(seenChannels))
	}
	if _, err := first.db.Exec(`INSERT INTO notification_outbox_channels (id, outbox_id, channel, available_at, created_at, updated_at) VALUES (?, ?, 'email', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, "live-orphan-"+uuid.NewString(), "live-missing-parent"); err == nil {
		t.Fatal("live libSQL orphan channel insert unexpectedly succeeded")
	}
	cascadeParent := outboxItem("live-channel-site", "site_digest", "live-channel-cascade-"+uuid.NewString(), time.Now().UTC())
	if err := first.EnqueueNotificationOutbox(cascadeParent); err != nil {
		t.Fatalf("enqueue live cascade parent: %v", err)
	}
	if _, err := first.EnsureNotificationOutboxChannels(cascadeParent.ID, []string{"email"}); err != nil {
		t.Fatalf("create live cascade channel: %v", err)
	}
	if _, err := second.db.Exec(`DELETE FROM notification_outbox WHERE id = ?`, cascadeParent.ID); err != nil {
		t.Fatalf("delete live cascade parent: %v", err)
	}
	var cascadeChildren int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM notification_outbox_channels WHERE outbox_id = ?`, cascadeParent.ID).Scan(&cascadeChildren); err != nil {
		t.Fatalf("count live cascade children: %v", err)
	}
	if cascadeChildren != 0 {
		t.Fatalf("live cascade left %d child rows", cascadeChildren)
	}

	fencingParent := outboxItem("live-channel-site", "site_digest", "live-channel-fencing-"+uuid.NewString(), time.Now().UTC())
	if err := first.EnqueueNotificationOutbox(fencingParent); err != nil {
		t.Fatalf("enqueue live channel fencing parent: %v", err)
	}
	fencingRows, err := first.EnsureNotificationOutboxChannels(fencingParent.ID, []string{"email"})
	if err != nil {
		t.Fatalf("create live fencing channel: %v", err)
	}
	fencingNow := time.Now().UTC().Add(time.Second)
	initialChannelClaim, err := first.ClaimNotificationOutboxChannels(fencingParent.ID, "live-same-owner", fencingNow, time.Minute, 1)
	if err != nil || len(initialChannelClaim) != 1 || initialChannelClaim[0].LeaseUntil == nil {
		t.Fatalf("initial live channel fencing claim: %v, %#v", err, initialChannelClaim)
	}
	initialChannelGeneration := initialChannelClaim[0].Attempts
	channelExpiry := *initialChannelClaim[0].LeaseUntil
	for _, boundary := range []time.Time{channelExpiry, channelExpiry.Add(time.Nanosecond)} {
		if err := first.AckNotificationOutboxChannel(fencingRows[0].ID, "live-same-owner", initialChannelGeneration, boundary); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
			t.Fatalf("live channel ack at expiry = %v", err)
		}
		if err := first.RetryNotificationOutboxChannel(fencingRows[0].ID, "live-same-owner", initialChannelGeneration, boundary.Add(time.Minute), boundary, "expired"); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
			t.Fatalf("live channel retry at expiry = %v", err)
		}
	}
	reclaimedChannels, err := second.ClaimNotificationOutboxChannels(fencingParent.ID, "live-same-owner", channelExpiry.Add(time.Nanosecond), time.Minute, 1)
	if err != nil || len(reclaimedChannels) != 1 || reclaimedChannels[0].Attempts != initialChannelGeneration+1 {
		t.Fatalf("live same-owner channel reclaim: %v, %#v", err, reclaimedChannels)
	}
	staleNow := channelExpiry.Add(2 * time.Second)
	if err := first.AckNotificationOutboxChannel(fencingRows[0].ID, "live-same-owner", initialChannelGeneration, staleNow); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
		t.Fatalf("live stale channel ack = %v", err)
	}
	if err := first.RetryNotificationOutboxChannel(fencingRows[0].ID, "live-same-owner", initialChannelGeneration, staleNow.Add(time.Minute), staleNow, "stale"); !errors.Is(err, ErrNotificationOutboxChannelNotOwned) {
		t.Fatalf("live stale channel retry = %v", err)
	}
	if err := second.AckNotificationOutboxChannel(reclaimedChannels[0].ID, "live-same-owner", reclaimedChannels[0].Attempts, reclaimedChannels[0].LeaseUntil.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("ack current live channel generation: %v", err)
	}
}

func TestLiveLibSQLStoreContextCancellation(t *testing.T) {
	dsn := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_DSN")
	authToken := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_AUTH_TOKEN")
	databaseID := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_DATABASE")
	if os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_NONPROD") != "1" {
		t.Skip("skipping live libSQL cancellation test: opt in with " + liveLibSQLOutboxConfig)
	}
	if _, err := validatedLiveLibSQLDSN(dsn, authToken, databaseID); err != nil {
		t.Fatalf("invalid live libSQL cancellation configuration: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewLibSQLStoreWithAuthTokenContext(ctx, dsn, authToken)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled live libSQL open error = %v, want context cancellation", err)
	}
}

func assertLiveLibSQLLatestSchema(t *testing.T, store *LibSQLStore) {
	t.Helper()
	for _, table := range []string{"notification_outbox", "notification_outbox_channels", "notification_outbox_comments"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatalf("check live table %q: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("live table %q count = %d, want 1", table, count)
		}
	}
}

func assertLiveAtlasRevision(t *testing.T, store *LibSQLStore, rawDSN, authToken string) {
	t.Helper()
	var revision string
	if err := store.db.QueryRow(`SELECT version FROM atlas_schema_revisions ORDER BY version DESC LIMIT 1`).Scan(&revision); err != nil {
		t.Fatalf("read Atlas live revision: %s", redactLiveContractError(err, rawDSN, authToken))
	}
	if revision != "00009" {
		t.Fatalf("Atlas live revision = %q, want 00009", revision)
	}
}

func redactLiveContractError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return message
}

func TestLiveLibSQLLegacyDigestInventoryContract(t *testing.T) {
	dsn := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_DSN")
	authToken := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_AUTH_TOKEN")
	databaseID := os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_DATABASE")
	if os.Getenv("QUIPTHREAD_LIVE_LIBSQL_TEST_NONPROD") != "1" {
		t.Skip("skipping live legacy digest contract test: opt in with " + liveLibSQLOutboxConfig)
	}
	if strings.TrimSpace(dsn) == "" || strings.TrimSpace(authToken) == "" || strings.TrimSpace(databaseID) == "" {
		t.Fatalf("live legacy digest contract test opted in but requires %s", liveLibSQLOutboxConfig)
	}
	if _, err := validatedLiveLibSQLDSN(dsn, authToken, databaseID); err != nil {
		t.Fatalf("invalid live legacy digest contract configuration: %v", err)
	}
	store, err := OpenLibSQLLegacyDigestInventory(dsn, authToken)
	if err != nil {
		t.Fatalf("open live legacy digest inventory: %v", err)
	}
	defer store.Close() //nolint:errcheck // test cleanup
	if _, err := store.ListUninitializedSiteDigestsContext(context.Background()); err != nil {
		t.Fatalf("list live legacy digest inventory: %v", err)
	}
}

func validatedLiveLibSQLDSN(raw, authToken, databaseID string) (string, error) {
	if strings.TrimSpace(authToken) == "" {
		return "", fmt.Errorf("test auth token must be non-empty")
	}
	if strings.TrimSpace(databaseID) == "" {
		return "", fmt.Errorf("test database identifier must be non-empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("test DSN must be a URL with a hostname")
	}
	if parsed.Scheme != "libsql" && parsed.Scheme != "https" {
		return "", fmt.Errorf("test DSN must use libsql:// or https://")
	}
	host := strings.ToLower(parsed.Hostname())
	if strings.ToLower(strings.TrimSpace(databaseID)) != host {
		return "", fmt.Errorf("test database identifier %q must exactly match DSN hostname %q", databaseID, host)
	}
	if !strings.HasPrefix(host, "quipthread-phase1-test-") {
		return "", fmt.Errorf("test database identifier %q must name a dedicated quipthread-phase1-test-* database", databaseID)
	}
	if strings.Contains(host, "prod") || strings.Contains(host, "production") {
		return "", fmt.Errorf("refusing a production-looking test DSN host %q", host)
	}
	nonProductionHost := host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		strings.Contains(host, "test") || strings.Contains(host, "staging") || strings.Contains(host, "dev")
	if !nonProductionHost {
		return "", fmt.Errorf("test DSN host %q is not identifiable as non-production", host)
	}
	parsed.RawQuery = ""
	return parsed.String(), nil
}
