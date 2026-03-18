package notifications

import (
	"context"

	"github.com/quipthread/quipthread/models"
)

// Batch is the unit passed to every Notifier implementation.
type Batch struct {
	Site        *models.Site
	Comments    []*models.Comment
	ApproveURLs map[string]string // commentID → approve URL
	RejectURLs  map[string]string // commentID → reject URL
}

// Notifier sends a batch of pending comments via some channel.
type Notifier interface {
	NotifyBatch(ctx context.Context, b Batch) error
}
