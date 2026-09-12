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

// BuildChannelSender constructs the explicit self-hosted channel boundary.
// Self-hosted builds retain their existing SMTP-only provider policy.
func BuildChannelSender(cfg *config.Config, store db.Store) *NamedChannelSender {
	ownerEmail := func(ownerID string) string {
		if store == nil {
			return ""
		}
		u, err := store.GetUser(ownerID)
		if err != nil || u == nil {
			return ""
		}
		return u.Email
	}

	notifiers := make(map[string]Notifier)
	if cfg.SMTPHost != "" {
		notifiers[ChannelEmail] = NewSMTPNotifier(cfg, ownerEmail)
	}
	return NewNamedChannelSender(notifiers)
}

// BuildNamedChannelSender is an explicit-name alias for BuildChannelSender.
func BuildNamedChannelSender(cfg *config.Config, store db.Store) *NamedChannelSender {
	return BuildChannelSender(cfg, store)
}
