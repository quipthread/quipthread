package notifications

import (
	"context"
	"log"
)

// MultiNotifier fans a batch out to all configured notifiers.
// Errors from individual notifiers are logged but do not abort delivery to
// the remaining ones.
type MultiNotifier struct {
	notifiers []Notifier
}

func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

func (m *MultiNotifier) NotifyBatch(ctx context.Context, b Batch) error {
	for _, n := range m.notifiers {
		if err := n.NotifyBatch(ctx, b); err != nil {
			log.Printf("notifications: notifier %T error: %v", n, err)
		}
	}
	return nil
}
