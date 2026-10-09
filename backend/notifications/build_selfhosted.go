//go:build selfhosted

package notifications

import (
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
)

// Build constructs a MultiNotifier with SMTP only for selfhosted builds.
// Slack, Discord, Telegram, and webhook channels are cloud-only.
func Build(cfg *config.Config, store db.Store) *MultiNotifier {
	ownerEmail := func(ownerID string) string {
		u, err := store.GetUser(ownerID)
		if err != nil || u == nil {
			return ""
		}
		return u.Email
	}

	var notifiers []Notifier

	if cfg.SMTPHost != "" {
		notifiers = append(notifiers, NewSMTPNotifier(cfg, ownerEmail))
	}

	return NewMultiNotifier(notifiers...)
}
