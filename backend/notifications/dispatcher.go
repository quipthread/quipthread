package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

const (
	defaultNotifyInterval = 5 * time.Minute
	tickInterval          = time.Minute
)

// StartDispatcher runs a background loop that checks for pending comments and
// dispatches notification batches per site. It respects ctx cancellation for
// clean shutdown. Call it in a goroutine from main.
func StartDispatcher(ctx context.Context, store db.Store, notifier Notifier, cfg *config.Config) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	// Run once immediately on startup, then on each tick.
	runDispatch(ctx, store, notifier, cfg)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runDispatch(ctx, store, notifier, cfg)
		}
	}
}

func runDispatch(ctx context.Context, store db.Store, notifier Notifier, cfg *config.Config) {
	sites, err := store.ListSites()
	if err != nil {
		slog.Error("notifications dispatcher: list sites", "error", err)
		return
	}

	for _, site := range sites {
		if err := ctx.Err(); err != nil {
			return
		}

		interval := siteInterval(site, cfg)

		// Determine the cutoff: only consider pending comments created after
		// the last notification. This prevents re-notifying for the same
		// comments on every subsequent dispatch cycle.
		var since time.Time
		if site.LastNotifiedAt != nil {
			since = *site.LastNotifiedAt
		}

		count, err := store.CountPendingComments(site.ID, since)
		if err != nil {
			slog.Error("notifications dispatcher: count pending", "site", site.ID, "error", err)
			continue
		}
		if count == 0 {
			continue
		}

		if !shouldNotify(site, count, cfg.NotifyBatchSize, interval) {
			continue
		}

		comments, err := store.ListPendingComments(site.ID, since)
		if err != nil {
			slog.Error("notifications dispatcher: list pending", "site", site.ID, "error", err)
			continue
		}
		if len(comments) == 0 {
			continue
		}

		approveURLs, rejectURLs, err := generateTokenURLs(store, comments, cfg.BaseURL)
		if err != nil {
			slog.Error("notifications dispatcher: generate tokens", "site", site.ID, "error", err)
			continue
		}

		batch := Batch{
			Site:        site,
			Comments:    comments,
			ApproveURLs: approveURLs,
			RejectURLs:  rejectURLs,
		}

		if err := notifier.NotifyBatch(ctx, batch); err != nil {
			slog.Error("notifications dispatcher: notify", "site", site.ID, "error", err)
			continue
		}

		if err := store.UpdateSiteLastNotifiedAt(site.ID, time.Now()); err != nil {
			slog.Error("notifications dispatcher: update last_notified_at", "site", site.ID, "error", err)
		}
	}
}

// siteInterval returns the notification dispatch interval for a site.
// In non-cloud (self-hosted) builds, always returns the 5-min default.
// In cloud builds, uses site.NotifyInterval if set; otherwise the default.
func siteInterval(site *models.Site, cfg *config.Config) time.Duration {
	if !cfg.CloudMode || site.NotifyInterval == nil {
		return defaultNotifyInterval
	}
	return time.Duration(*site.NotifyInterval) * time.Second
}

func shouldNotify(site *models.Site, count, batchSize int, interval time.Duration) bool {
	if count >= batchSize {
		return true
	}
	if site.LastNotifiedAt == nil {
		return true
	}
	return time.Since(*site.LastNotifiedAt) >= interval
}

func generateTokenURLs(
	store db.Store,
	comments []*models.Comment,
	baseURL string,
) (approveURLs, rejectURLs map[string]string, err error) {
	approveURLs = make(map[string]string, len(comments))
	rejectURLs = make(map[string]string, len(comments))

	for _, c := range comments {
		token := uuid.NewString()
		at := &models.ApprovalToken{
			Token:     token,
			CommentID: c.ID,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		if err := store.CreateApprovalToken(at); err != nil {
			return nil, nil, fmt.Errorf("create approval token for comment %s: %w", c.ID, err)
		}
		approveURLs[c.ID] = fmt.Sprintf("%s/approve/%s?action=approve", baseURL, token)
		rejectURLs[c.ID] = fmt.Sprintf("%s/approve/%s?action=reject", baseURL, token)
	}
	return approveURLs, rejectURLs, nil
}
