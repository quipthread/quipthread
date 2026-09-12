package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/quipthread/quipthread/models"
)

// seedApprovalFixture creates a site, user, pending comment, and approval
// token, returning the IDs used. siteID must be unique per store.
func seedApprovalFixture(t *testing.T, store Store, siteID, commentID, token string, expiresAt time.Time) {
	t.Helper()
	if err := store.CreateSite(&models.Site{ID: siteID, OwnerID: "o1", Domain: "x.com"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	if err := store.UpsertUser(&models.User{ID: "u1", DisplayName: "Author", Role: "commenter"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := store.CreateComment(&models.Comment{
		ID: commentID, SiteID: siteID, PageID: "/post", UserID: "u1",
		Content: "hello", Status: "pending",
	}); err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	if err := store.CreateApprovalToken(&models.ApprovalToken{
		Token: token, CommentID: commentID, ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatalf("seed approval token: %v", err)
	}
}

// TestApprovalTokenCRUDCompatibility pins the pre-existing self-hosted token
// behavior: raw tokens round-trip with their comment association, unknown
// tokens read as (nil, nil), and deleting an unknown token is a no-op.
func TestApprovalTokenCRUDCompatibility(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	seedApprovalFixture(t, store, "s1", "c1", "tok-1", now.Add(24*time.Hour))

	got, err := store.GetApprovalToken("tok-1")
	if err != nil || got == nil {
		t.Fatalf("GetApprovalToken: got (%v, %v)", got, err)
	}
	if got.Token != "tok-1" || got.CommentID != "c1" {
		t.Errorf("token round-trip mismatch: %+v", got)
	}
	if !got.ExpiresAt.Equal(now.Add(24*time.Hour).Truncate(time.Second)) &&
		!got.ExpiresAt.Equal(now.Add(24*time.Hour)) {
		t.Errorf("expires_at not round-tripped: got %v, want ~%v", got.ExpiresAt, now.Add(24*time.Hour))
	}

	missing, err := store.GetApprovalToken("unknown-token")
	if err != nil || missing != nil {
		t.Errorf("GetApprovalToken(unknown): got (%v, %v), want (nil, nil)", missing, err)
	}

	if err := store.DeleteApprovalToken("tok-1"); err != nil {
		t.Fatalf("DeleteApprovalToken: %v", err)
	}
	// Legacy semantics: deleting an unknown/already-deleted token is a no-op,
	// not an error.
	if err := store.DeleteApprovalToken("tok-1"); err != nil {
		t.Errorf("re-delete: got %v, want nil", err)
	}
	gone, err := store.GetApprovalToken("tok-1")
	if err != nil || gone != nil {
		t.Errorf("token survived delete: got (%v, %v)", gone, err)
	}
}

func TestConsumeApprovalTokenSuccess(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	seedApprovalFixture(t, store, "s1", "c1", "tok-approve", now.Add(24*time.Hour))
	seedApprovalFixture(t, store, "s2", "c2", "tok-reject", now.Add(24*time.Hour))

	comment, err := store.ConsumeApprovalToken("tok-approve", "approved", now)
	if err != nil {
		t.Fatalf("consume approve: %v", err)
	}
	if comment.ID != "c1" || comment.Status != "approved" {
		t.Errorf("returned comment: %+v, want c1/approved", comment)
	}
	persisted, err := store.GetComment("c1")
	if err != nil || persisted == nil {
		t.Fatalf("GetComment after approve: (%v, %v)", persisted, err)
	}
	if persisted.Status != "approved" {
		t.Errorf("status after approve: %q, want approved", persisted.Status)
	}

	comment, err = store.ConsumeApprovalToken("tok-reject", "rejected", now)
	if err != nil {
		t.Fatalf("consume reject: %v", err)
	}
	if comment.Status != "rejected" {
		t.Errorf("status after reject: %q, want rejected", comment.Status)
	}

	// Single use: consumed tokens are gone.
	for _, tok := range []string{"tok-approve", "tok-reject"} {
		got, err := store.GetApprovalToken(tok)
		if err != nil || got != nil {
			t.Errorf("token %s survived consume: got (%v, %v)", tok, got, err)
		}
	}
}

func TestConsumeApprovalTokenNotFound(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.ConsumeApprovalToken("ghost", "approved", time.Now().UTC()); !errors.Is(err, ErrApprovalTokenNotFound) {
		t.Fatalf("consume unknown token: got %v, want ErrApprovalTokenNotFound", err)
	}
}

// TestConsumeApprovalTokenReplay proves replay safety: the second consume of
// the same token fails without touching the comment again.
func TestConsumeApprovalTokenReplay(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	seedApprovalFixture(t, store, "s1", "c1", "tok-1", now.Add(24*time.Hour))

	if _, err := store.ConsumeApprovalToken("tok-1", "approved", now); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := store.ConsumeApprovalToken("tok-1", "rejected", now); !errors.Is(err, ErrApprovalTokenNotFound) {
		t.Fatalf("replay consume: got %v, want ErrApprovalTokenNotFound", err)
	}
	comment, err := store.GetComment("c1")
	if err != nil || comment == nil {
		t.Fatalf("GetComment after replay: (%v, %v)", comment, err)
	}
	if comment.Status != "approved" {
		t.Errorf("status changed by replay: %q, want approved", comment.Status)
	}
}

// TestConsumeApprovalTokenExpired proves expired links are refused without
// consuming the token or mutating the comment.
func TestConsumeApprovalTokenExpired(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	seedApprovalFixture(t, store, "s1", "c1", "tok-1", now.Add(-time.Minute))

	if _, err := store.ConsumeApprovalToken("tok-1", "approved", now); !errors.Is(err, ErrApprovalTokenExpired) {
		t.Fatalf("consume expired: got %v, want ErrApprovalTokenExpired", err)
	}
	// Expired tokens are left in place (matching the legacy GET page behavior
	// that renders "expired" for them); the comment is untouched.
	got, err := store.GetApprovalToken("tok-1")
	if err != nil || got == nil {
		t.Fatalf("expired token removed: got (%v, %v)", got, err)
	}
	comment, err := store.GetComment("c1")
	if err != nil || comment == nil {
		t.Fatalf("GetComment after expired consume: (%v, %v)", comment, err)
	}
	if comment.Status != "pending" {
		t.Errorf("status mutated by expired consume: %q, want pending", comment.Status)
	}
}

func TestConsumeApprovalTokenInvalidStatus(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	seedApprovalFixture(t, store, "s1", "c1", "tok-1", now.Add(24*time.Hour))

	for _, status := range []string{"", "approve", "delete", "approved; DROP"} {
		if _, err := store.ConsumeApprovalToken("tok-1", status, now); !errors.Is(err, ErrInvalidApprovalStatus) {
			t.Errorf("consume with status %q: got %v, want ErrInvalidApprovalStatus", status, err)
		}
	}
	// Rejected invalid requests must not have consumed the token.
	if got, err := store.GetApprovalToken("tok-1"); err != nil || got == nil {
		t.Errorf("token consumed by invalid status: got (%v, %v)", got, err)
	}
}

// TestConsumeApprovalTokenCommentMissing covers the dangling-token edge case:
// if the comment was deleted after the token was issued, the consume fails
// without consuming the token. The schema's FK on approval_tokens.comment_id
// prevents this through the public API, so the comment is removed with FK
// enforcement disabled on the raw handle — mirroring databases that predate
// the constraint or tenants where it is not enforced.
func TestConsumeApprovalTokenCommentMissing(t *testing.T) {
	sqlStore, err := NewSQLiteStoreForTest(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { sqlStore.Close() }) //nolint:errcheck,gosec // deferred store cleanup in test

	now := time.Now().UTC()
	seedApprovalFixture(t, sqlStore, "s1", "c1", "tok-1", now.Add(24*time.Hour))
	if _, err := sqlStore.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable FKs: %v", err)
	}
	if _, err := sqlStore.db.Exec(`DELETE FROM comments WHERE id = 'c1'`); err != nil {
		t.Fatalf("delete comment: %v", err)
	}
	if _, err := sqlStore.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("re-enable FKs: %v", err)
	}

	if _, err := sqlStore.ConsumeApprovalToken("tok-1", "approved", now); !errors.Is(err, ErrApprovalCommentNotFound) {
		t.Fatalf("consume with missing comment: got %v, want ErrApprovalCommentNotFound", err)
	}
	if got, err := sqlStore.GetApprovalToken("tok-1"); err != nil || got == nil {
		t.Errorf("token consumed despite missing comment: got (%v, %v)", got, err)
	}
}

// TestConsumeApprovalTokenConcurrentSingleWinner races two consumes of the
// same raw token with opposite actions on a shared file-backed database.
// Exactly one transaction may commit: the loser must observe the token as
// consumed (or fail on the write lock) and the comment must carry exactly the
// winner's action.
func TestConsumeApprovalTokenConcurrentSingleWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenant.db")
	store, err := NewSQLiteStoreForTest(path)
	if err != nil {
		t.Fatalf("open shared store: %v", err)
	}
	defer store.Close() //nolint:errcheck,gosec // test cleanup
	now := time.Now().UTC()
	seedApprovalFixture(t, store, "s1", "c1", "tok-race", now.Add(24*time.Hour))

	type outcome struct {
		status string
		err    error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for _, status := range []string{"approved", "rejected"} {
		go func(status string) {
			<-start
			_, err := store.ConsumeApprovalToken("tok-race", status, time.Now().UTC())
			results <- outcome{status: status, err: err}
		}(status)
	}
	close(start)

	winners := 0
	var winnerStatus string
	for range 2 {
		res := <-results
		if res.err == nil {
			winners++
			winnerStatus = res.status
		} else if !errors.Is(res.err, ErrApprovalTokenNotFound) {
			// A loser may also fail on SQLite write-lock contention before
			// reaching the token delete; both paths mutate nothing.
			t.Logf("loser failed with %v", res.err)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent consumes: %d winners, want exactly 1", winners)
	}

	comment, err := store.GetComment("c1")
	if err != nil || comment == nil {
		t.Fatalf("GetComment after race: (%v, %v)", comment, err)
	}
	if comment.Status != winnerStatus {
		t.Errorf("comment status = %q, want the single winner's action %q", comment.Status, winnerStatus)
	}
	got, err := store.GetApprovalToken("tok-race")
	if err != nil || got != nil {
		t.Errorf("token survived race: got (%v, %v)", got, err)
	}
}
