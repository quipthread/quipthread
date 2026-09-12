package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/quipthread/quipthread/cloud/approvaltoken"
	"github.com/quipthread/quipthread/models"
)

var (
	ErrDigestApprovalTokenRequest       = errors.New("site digest approval token: invalid request")
	ErrDigestApprovalTokenConflict      = errors.New("site digest approval token: association conflict")
	ErrDigestApprovalMaterialization    = errors.New("site digest approval token: materialization is stale")
	ErrDigestApprovalLocatorConflict    = errors.New("site digest approval locator: association conflict")
	ErrDigestApprovalLocatorUnavailable = errors.New("site digest approval locator: unavailable")
)

const approvalTokenRandomBytes = 32

// SiteDigestApprovalTokenOptions supplies the account routing metadata, HMAC
// key, clock, and control-plane sink for one tenant-local ensure operation.
// GenerateToken is optional; the default uses fresh cryptographic randomness.
// Persistence, rather than deterministic secret material, provides stability
// across retries and process restarts.
type SiteDigestApprovalTokenOptions struct {
	AccountID     string
	ExpiresAt     time.Time
	Now           time.Time
	HMACKey       string
	LocatorStore  ApprovalTokenLocatorStore
	GenerateToken func() (string, error)
}

// EnsureMaterializedSiteDigestApprovalTokens idempotently ensures one stable
// tenant-local bearer token and one hash-only control-plane locator for every
// pending comment in a claimed materialized digest. Tenant token rows commit
// before locator publication so a partial control-plane failure can be retried
// without generating replacement bearer tokens.
func (s *sqlStore) EnsureMaterializedSiteDigestApprovalTokens(ctx context.Context, materialized *models.SiteDigestMaterialization, opts SiteDigestApprovalTokenOptions) ([]*models.ApprovalToken, error) {
	if err := validateSiteDigestApprovalTokenRequest(ctx, materialized, opts); err != nil {
		return nil, err
	}
	generator := opts.GenerateToken
	if generator == nil {
		generator = NewOpaqueApprovalToken
	}

	tokens, err := retryNotificationOutboxBusyContextResult(ctx, func() ([]*models.ApprovalToken, error) {
		return s.ensureTenantDigestApprovalTokens(ctx, materialized, opts, generator)
	})
	if err != nil {
		return nil, sanitizeDigestApprovalTokenError(err)
	}

	for _, token := range tokens {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := ensureApprovalTokenLocator(ctx, opts, materialized.Site.ID, token); err != nil {
			return nil, err
		}
	}
	return tokens, nil
}

// NewOpaqueApprovalToken returns a URL-safe random bearer token. Callers that
// need deterministic derivation must inject GenerateToken; no secret or
// derivation domain is hardcoded in this package.
func NewOpaqueApprovalToken() (string, error) {
	raw := make([]byte, approvalTokenRandomBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", ErrDigestApprovalTokenRequest
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validateSiteDigestApprovalTokenRequest(ctx context.Context, materialized *models.SiteDigestMaterialization, opts SiteDigestApprovalTokenOptions) error {
	if ctx == nil || materialized == nil || materialized.Outbox == nil || materialized.Site == nil || opts.LocatorStore == nil || opts.Now.IsZero() || opts.ExpiresAt.IsZero() || !opts.ExpiresAt.After(opts.Now) || strings.TrimSpace(opts.AccountID) != opts.AccountID || opts.AccountID == "" {
		return ErrDigestApprovalTokenRequest
	}
	if materialized.Outbox.ID == "" || materialized.Outbox.LeaseOwner == "" || materialized.Outbox.Attempts <= 0 || materialized.Site.ID == "" || materialized.Site.ID != materialized.Outbox.SiteID {
		return ErrDigestApprovalTokenRequest
	}
	if err := approvaltoken.ValidateKey(opts.HMACKey); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(materialized.Comments))
	for _, comment := range materialized.Comments {
		if comment == nil || comment.ID == "" || comment.SiteID != materialized.Site.ID || comment.Status != "pending" {
			return ErrDigestApprovalMaterialization
		}
		if _, exists := seen[comment.ID]; exists {
			return ErrDigestApprovalMaterialization
		}
		seen[comment.ID] = struct{}{}
	}
	return nil
}

func (s *sqlStore) ensureTenantDigestApprovalTokens(ctx context.Context, materialized *models.SiteDigestMaterialization, opts SiteDigestApprovalTokenOptions, generate func() (string, error)) ([]*models.ApprovalToken, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after successful commit

	parent, spec, err := claimedSiteDigestParentTx(ctx, tx, materialized.Outbox.ID, materialized.Outbox.LeaseOwner, materialized.Outbox.Attempts, opts.Now.UTC())
	if err != nil {
		return nil, err
	}
	if parent.SiteID != materialized.Site.ID {
		return nil, ErrDigestApprovalMaterialization
	}
	var siteExists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM sites WHERE id = ?`, parent.SiteID).Scan(&siteExists); err != nil {
		return nil, err
	}
	currentComments, err := loadSiteDigestCommentsTx(ctx, tx, parent, spec)
	if err != nil {
		return nil, err
	}
	if !sameDigestApprovalComments(materialized.Comments, currentComments) {
		return nil, ErrDigestApprovalMaterialization
	}

	tokens := make([]*models.ApprovalToken, 0, len(currentComments))
	for _, comment := range currentComments {
		var token string
		var expiresAt time.Time
		rows, err := tx.QueryContext(ctx, `SELECT token, expires_at FROM approval_tokens WHERE comment_id = ? ORDER BY token LIMIT 2`, comment.ID)
		if err != nil {
			return nil, err
		}
		count := 0
		for rows.Next() {
			if err := rows.Scan(&token, &expiresAt); err != nil {
				rows.Close() //nolint:errcheck,gosec // cleanup after scan failure
				return nil, err
			}
			count++
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if rowsErr != nil {
			return nil, rowsErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if count > 1 {
			return nil, ErrDigestApprovalTokenConflict
		}
		if count == 1 {
			if !validOpaqueApprovalToken(token) {
				return nil, ErrDigestApprovalTokenConflict
			}
			if expiresAt.After(opts.Now) {
				// A valid unexpired tenant token is stable across retries. Its
				// original expiry, rather than a later retry timestamp, remains
				// the locator expiry.
			} else {
				var generateErr error
				token, generateErr = generate()
				if generateErr != nil || !validOpaqueApprovalToken(token) {
					return nil, ErrDigestApprovalTokenRequest
				}
				expiresAt = opts.ExpiresAt.UTC()
				if _, err := tx.ExecContext(ctx, `DELETE FROM approval_tokens WHERE comment_id = ?`, comment.ID); err != nil {
					return nil, ErrDigestApprovalTokenConflict
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO approval_tokens (token, comment_id, expires_at) VALUES (?, ?, ?)`, token, comment.ID, expiresAt); err != nil {
					return nil, ErrDigestApprovalTokenConflict
				}
			}
		} else {
			var generateErr error
			token, generateErr = generate()
			if generateErr != nil || !validOpaqueApprovalToken(token) {
				return nil, ErrDigestApprovalTokenRequest
			}
			expiresAt = opts.ExpiresAt.UTC()
			if _, err := tx.ExecContext(ctx, `INSERT INTO approval_tokens (token, comment_id, expires_at) VALUES (?, ?, ?)`, token, comment.ID, expiresAt); err != nil {
				return nil, ErrDigestApprovalTokenConflict
			}
		}
		tokens = append(tokens, &models.ApprovalToken{Token: token, CommentID: comment.ID, ExpiresAt: expiresAt})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tokens, nil
}

func sameDigestApprovalComments(expected, current []*models.Comment) bool {
	if len(expected) != len(current) {
		return false
	}
	currentIDs := make(map[string]struct{}, len(current))
	for _, comment := range current {
		currentIDs[comment.ID] = struct{}{}
	}
	for _, comment := range expected {
		if _, ok := currentIDs[comment.ID]; !ok {
			return false
		}
	}
	return true
}

func validOpaqueApprovalToken(token string) bool {
	return strings.TrimSpace(token) == token && token != "" && len(token) <= 512
}

func ensureApprovalTokenLocator(ctx context.Context, opts SiteDigestApprovalTokenOptions, siteID string, token *models.ApprovalToken) error {
	hash := approvaltoken.Hash(opts.HMACKey, token.Token)
	expected := &models.ApprovalTokenLocator{
		TokenHash: hash,
		AccountID: opts.AccountID,
		SiteID:    siteID,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: opts.Now.UTC(),
	}
	actual, err := opts.LocatorStore.GetApprovalTokenContext(ctx, hash)
	if err != nil {
		return sanitizeApprovalLocatorError(err)
	}
	if actual != nil {
		return validateApprovalTokenLocator(expected, actual)
	}
	if err := opts.LocatorStore.CreateApprovalTokenContext(ctx, expected); err == nil {
		return nil
	}
	// A concurrent creator may have won. Re-read and validate the complete
	// association; any absent or mismatched row fails closed.
	actual, err = opts.LocatorStore.GetApprovalTokenContext(ctx, hash)
	if err != nil {
		return sanitizeApprovalLocatorError(err)
	}
	if actual == nil {
		return ErrDigestApprovalLocatorUnavailable
	}
	return validateApprovalTokenLocator(expected, actual)
}

func sanitizeApprovalLocatorError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrDigestApprovalLocatorUnavailable
}

func validateApprovalTokenLocator(expected, actual *models.ApprovalTokenLocator) error {
	if actual.TokenHash != expected.TokenHash || actual.AccountID != expected.AccountID || actual.SiteID != expected.SiteID || !actual.ExpiresAt.Equal(expected.ExpiresAt) {
		return ErrDigestApprovalLocatorConflict
	}
	return nil
}

func sanitizeDigestApprovalTokenError(err error) error {
	switch {
	case errors.Is(err, ErrDigestMembershipUninitialized):
		return ErrDigestMembershipUninitialized
	case errors.Is(err, ErrDigestMembershipMalformed):
		return ErrDigestMembershipMalformed
	case errors.Is(err, ErrDigestAggregateMalformed):
		return ErrDigestAggregateMalformed
	case errors.Is(err, ErrNotificationOutboxNotFound):
		return ErrNotificationOutboxNotFound
	case errors.Is(err, ErrNotificationOutboxNotOwned):
		return ErrNotificationOutboxNotOwned
	case errors.Is(err, ErrInvalidSiteDigestParent):
		return ErrInvalidSiteDigestParent
	case errors.Is(err, ErrDigestApprovalTokenConflict), errors.Is(err, ErrDigestApprovalMaterialization):
		return err
	default:
		return ErrDigestApprovalTokenRequest
	}
}
