package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quipthread/quipthread/cloud/approvaltoken"
	"github.com/quipthread/quipthread/models"
)

type approvalLocatorFixture struct {
	rows       map[string]*models.ApprovalTokenLocator
	createCall int
	failAt     int
	getBlock   <-chan struct{}
	getStarted chan<- struct{}
}

func newApprovalLocatorFixture() *approvalLocatorFixture {
	return &approvalLocatorFixture{rows: make(map[string]*models.ApprovalTokenLocator)}
}

func (f *approvalLocatorFixture) GetApprovalTokenContext(ctx context.Context, tokenHash string) (*models.ApprovalTokenLocator, error) {
	if f.getStarted != nil {
		select {
		case f.getStarted <- struct{}{}:
		default:
		}
	}
	if f.getBlock != nil {
		select {
		case <-f.getBlock:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	row := f.rows[tokenHash]
	if row == nil {
		return nil, nil
	}
	copy := *row
	return &copy, nil
}

func (f *approvalLocatorFixture) CreateApprovalTokenContext(_ context.Context, locator *models.ApprovalTokenLocator) error {
	f.createCall++
	if f.failAt != 0 && f.createCall == f.failAt {
		return errors.New("simulated control-plane failure")
	}
	if _, exists := f.rows[locator.TokenHash]; exists {
		return errors.New("simulated locator conflict")
	}
	copy := *locator
	f.rows[locator.TokenHash] = &copy
	return nil
}

func materializedApprovalDigest(t *testing.T, store Store, siteID, accountID string, commentIDs ...string) (*models.SiteDigestMaterialization, time.Time) {
	t.Helper()
	seedDigestSite(t, store, siteID, nil)
	base := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for _, commentID := range commentIDs {
		if err := store.CreatePendingCommentWithNotification(pendingDigestComment(siteID, commentID, base.Add(time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	now := base.Add(10 * time.Minute)
	parentID := outboxIDForComment(t, store, commentIDs[0])
	claimed, err := store.ClaimNotificationOutbox("approval-worker-"+accountID, now, time.Hour, 1)
	if err != nil || len(claimed) != 1 || claimed[0].ID != parentID {
		t.Fatalf("claim digest: %v, %#v", err, claimed)
	}
	materialized, err := store.MaterializeClaimedSiteDigest(context.Background(), parentID, claimed[0].LeaseOwner, claimed[0].Attempts, now)
	if err != nil {
		t.Fatalf("materialize digest: %v", err)
	}
	return materialized, now
}

func approvalTokenOptions(accountID string, now, expiresAt time.Time, locators ApprovalTokenLocatorStore, generate func() (string, error)) SiteDigestApprovalTokenOptions {
	return SiteDigestApprovalTokenOptions{
		AccountID:     accountID,
		ExpiresAt:     expiresAt,
		Now:           now,
		HMACKey:       "0123456789abcdef0123456789abcdef",
		LocatorStore:  locators,
		GenerateToken: generate,
	}
}

func TestEnsureMaterializedSiteDigestApprovalTokensIsIdempotentAcrossRestart(t *testing.T) {
	path := t.TempDir() + "/tenant.db"
	store, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	materialized, now := materializedApprovalDigest(t, store, "site-1", "account-1", "comment-1")
	expiresAt := now.Add(24 * time.Hour)
	locators := newApprovalLocatorFixture()
	generateCalls := 0
	generate := func() (string, error) {
		generateCalls++
		return "opaque-approval-token-1", nil
	}
	first, err := store.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), materialized, approvalTokenOptions("account-1", now, expiresAt, locators, generate))
	if err != nil || len(first) != 1 {
		t.Fatalf("first ensure: %v, %#v", err, first)
	}
	if first[0].Token != "opaque-approval-token-1" || generateCalls != 1 || len(locators.rows) != 1 {
		t.Fatalf("first token state: %#v calls=%d locators=%d", first, generateCalls, len(locators.rows))
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close() //nolint:errcheck,gosec // test cleanup
	second, err := reopened.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), materialized, approvalTokenOptions("account-1", now, expiresAt, locators, func() (string, error) {
		return "replacement-token-must-not-be-generated", errors.New("generator must not run on retry")
	}))
	if err != nil || len(second) != 1 || second[0].Token != first[0].Token {
		t.Fatalf("restart ensure: %v, %#v", err, second)
	}
	if generateCalls != 1 || len(locators.rows) != 1 {
		t.Fatalf("retry regenerated or duplicated state: calls=%d locators=%d", generateCalls, len(locators.rows))
	}
}

func TestEnsureMaterializedSiteDigestApprovalTokensReusesUnexpiredTokenAfterReclaim(t *testing.T) {
	store := newPersistentOutboxStore(t)
	firstMaterialized, firstNow := materializedApprovalDigest(t, store, "site-1", "account-1", "comment-1")
	firstExpiry := firstNow.Add(24 * time.Hour)
	locators := newApprovalLocatorFixture()
	locators.failAt = 1
	if _, err := store.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), firstMaterialized, approvalTokenOptions("account-1", firstNow, firstExpiry, locators, func() (string, error) {
		return "opaque-reclaimed-token", nil
	})); !errors.Is(err, ErrDigestApprovalLocatorUnavailable) {
		t.Fatalf("initial locator failure = %v", err)
	}

	secondNow := firstNow.Add(2 * time.Hour)
	claimed, err := store.ClaimNotificationOutbox("replacement-owner", secondNow, time.Hour, 1)
	if err != nil || len(claimed) != 1 || claimed[0].LeaseOwner != "replacement-owner" || claimed[0].Attempts != 2 {
		t.Fatalf("reclaimed parent = %v, %#v", err, claimed)
	}
	secondMaterialized, err := store.MaterializeClaimedSiteDigest(context.Background(), claimed[0].ID, "replacement-owner", claimed[0].Attempts, secondNow)
	if err != nil {
		t.Fatalf("materialize reclaimed parent: %v", err)
	}
	second, err := store.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), secondMaterialized, approvalTokenOptions("account-1", secondNow, secondNow.Add(24*time.Hour), locators, func() (string, error) {
		return "replacement-token-must-not-be-generated", errors.New("generator must not run for an unexpired token")
	}))
	if err != nil || len(second) != 1 || second[0].Token != "opaque-reclaimed-token" || !second[0].ExpiresAt.Equal(firstExpiry) {
		t.Fatalf("reclaimed ensure = %v, %#v", err, second)
	}
	if len(locators.rows) != 1 {
		t.Fatalf("reclaimed locator count = %d, want 1", len(locators.rows))
	}
	for _, locator := range locators.rows {
		if !locator.ExpiresAt.Equal(firstExpiry) || locator.AccountID != "account-1" || locator.SiteID != "site-1" {
			t.Fatalf("reclaimed locator association = %#v", locator)
		}
	}
}

func TestEnsureMaterializedSiteDigestApprovalTokensRetriesPartialLocatorFailure(t *testing.T) {
	store := newPersistentOutboxStore(t)
	materialized, now := materializedApprovalDigest(t, store, "site-1", "account-1", "comment-1", "comment-2")
	expiresAt := now.Add(24 * time.Hour)
	locators := newApprovalLocatorFixture()
	locators.failAt = 2
	generated := 0
	generate := func() (string, error) {
		generated++
		return fmt.Sprintf("opaque-approval-token-%d", generated), nil
	}
	if _, err := store.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), materialized, approvalTokenOptions("account-1", now, expiresAt, locators, generate)); !errors.Is(err, ErrDigestApprovalLocatorUnavailable) {
		t.Fatalf("partial locator failure = %v", err)
	} else if strings.Contains(err.Error(), "opaque-approval-token") || strings.Contains(err.Error(), "comment-") || strings.Contains(err.Error(), "content") {
		t.Fatalf("partial locator error leaked tenant data: %v", err)
	}
	if generated != 2 || len(locators.rows) != 1 {
		t.Fatalf("partial ensure state: generated=%d locators=%d", generated, len(locators.rows))
	}
	second, err := store.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), materialized, approvalTokenOptions("account-1", now, expiresAt, locators, func() (string, error) {
		return "replacement-token-must-not-be-generated", errors.New("generator must not run on retry")
	}))
	if err != nil || len(second) != 2 {
		t.Fatalf("retry after partial locator failure: %v, %#v", err, second)
	}
	if len(locators.rows) != 2 {
		t.Fatalf("retry locator count = %d, want 2", len(locators.rows))
	}
}

func TestEnsureMaterializedSiteDigestApprovalTokensHonorsLocatorContextCancellation(t *testing.T) {
	store := newPersistentOutboxStore(t)
	materialized, now := materializedApprovalDigest(t, store, "site-1", "account-1", "comment-1")
	locators := newApprovalLocatorFixture()
	started := make(chan struct{}, 1)
	locators.getStarted = started
	locators.getBlock = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := store.EnsureMaterializedSiteDigestApprovalTokens(ctx, materialized, approvalTokenOptions("account-1", now, now.Add(24*time.Hour), locators, func() (string, error) {
			return "opaque-cancellation-token", nil
		}))
		result <- err
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("locator lookup did not start")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled locator lookup = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled locator lookup did not return")
	}
}

func TestEnsureMaterializedSiteDigestApprovalTokensRejectsLocatorMismatch(t *testing.T) {
	store := newPersistentOutboxStore(t)
	materialized, now := materializedApprovalDigest(t, store, "site-1", "account-1", "comment-1")
	expiresAt := now.Add(24 * time.Hour)
	const rawToken = "opaque-approval-token-1"
	locators := newApprovalLocatorFixture()
	hash := approvaltoken.Hash("0123456789abcdef0123456789abcdef", rawToken)
	locators.rows[hash] = &models.ApprovalTokenLocator{
		TokenHash: hash,
		AccountID: "wrong-account",
		SiteID:    "site-1",
		ExpiresAt: expiresAt,
	}
	_, err := store.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), materialized, approvalTokenOptions("account-1", now, expiresAt, locators, func() (string, error) {
		return rawToken, nil
	}))
	if !errors.Is(err, ErrDigestApprovalLocatorConflict) {
		t.Fatalf("locator mismatch = %v", err)
	}
	if strings.Contains(err.Error(), rawToken) || strings.Contains(err.Error(), "comment-1") {
		t.Fatalf("mismatch error leaked tenant data: %v", err)
	}
}

func TestEnsureMaterializedSiteDigestApprovalTokensIsTenantLocal(t *testing.T) {
	firstStore := newPersistentOutboxStore(t)
	secondStore := newPersistentOutboxStore(t)
	firstMaterialized, now := materializedApprovalDigest(t, firstStore, "same-site", "account-a", "same-comment")
	secondMaterialized, _ := materializedApprovalDigest(t, secondStore, "same-site", "account-b", "same-comment")
	expiresAt := now.Add(24 * time.Hour)
	locators := newApprovalLocatorFixture()
	first, err := firstStore.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), firstMaterialized, approvalTokenOptions("account-a", now, expiresAt, locators, func() (string, error) {
		return "opaque-tenant-a-token", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondStore.EnsureMaterializedSiteDigestApprovalTokens(context.Background(), secondMaterialized, approvalTokenOptions("account-b", now, expiresAt, locators, func() (string, error) {
		return "opaque-tenant-b-token", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || len(locators.rows) != 2 {
		t.Fatalf("colliding tenant IDs: first=%#v second=%#v locators=%d", first, second, len(locators.rows))
	}
	for _, locator := range locators.rows {
		encoded, err := json.Marshal(locator)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		for _, forbidden := range []string{"same-comment", "opaque-tenant-a-token", "opaque-tenant-b-token", "comment_id", "content"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("control-plane locator leaked %q: %s", forbidden, text)
			}
		}
	}
}
