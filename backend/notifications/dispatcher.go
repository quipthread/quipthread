package notifications

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

const dispatchInterval = 5 * time.Minute

// StartDispatcher runs a background loop that checks for pending comments and
// dispatches notification batches. It respects ctx cancellation for clean
// shutdown. Call it in a goroutine from main.
func StartDispatcher(ctx context.Context, store db.Store, notifier Notifier, cfg *config.Config) {
	ticker := time.NewTicker(dispatchInterval)
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
		log.Printf("notifications dispatcher: list sites: %v", err)
		return
	}

	cooldown := time.Duration(cfg.NotifyCooldownHrs) * time.Hour

	for _, site := range sites {
		if err := ctx.Err(); err != nil {
			return
		}

		count, err := store.CountPendingComments(site.ID)
		if err != nil {
			log.Printf("notifications dispatcher: count pending for site %s: %v", site.ID, err)
			continue
		}
		if count == 0 {
			continue
		}

		if !shouldNotify(site, count, cfg.NotifyBatchSize, cooldown) {
			continue
		}

		comments, err := store.ListPendingComments(site.ID)
		if err != nil {
			log.Printf("notifications dispatcher: list pending for site %s: %v", site.ID, err)
			continue
		}
		if len(comments) == 0 {
			continue
		}

		approveURLs, rejectURLs, err := generateTokenURLs(ctx, store, comments, cfg.BaseURL)
		if err != nil {
			log.Printf("notifications dispatcher: generate tokens for site %s: %v", site.ID, err)
			continue
		}

		batch := Batch{
			Site:        site,
			Comments:    comments,
			ApproveURLs: approveURLs,
			RejectURLs:  rejectURLs,
		}

		if err := notifier.NotifyBatch(ctx, batch); err != nil {
			log.Printf("notifications dispatcher: notify site %s: %v", site.ID, err)
			continue
		}

		if err := store.UpdateSiteLastNotifiedAt(site.ID, time.Now()); err != nil {
			log.Printf("notifications dispatcher: update last_notified_at for site %s: %v", site.ID, err)
		}
	}
}

func shouldNotify(site *models.Site, count, batchSize int, cooldown time.Duration) bool {
	if count >= batchSize {
		return true
	}
	if site.LastNotifiedAt == nil {
		return true
	}
	return time.Since(*site.LastNotifiedAt) >= cooldown
}

func generateTokenURLs(
	ctx context.Context,
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
