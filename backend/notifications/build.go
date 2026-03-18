package notifications

import (
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
)

// Build constructs a MultiNotifier from whatever channels are enabled in cfg.
// ownerEmailFn resolves a site owner's email address from their user ID; it is
// used by the SMTP and email-API notifiers.
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

	if cfg.EmailProvider != "" && cfg.EmailAPIKey != "" {
		notifiers = append(notifiers, NewEmailAPINotifier(cfg, ownerEmail))
	}

	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		notifiers = append(notifiers, NewTelegramNotifier(cfg))
	}

	if cfg.SlackWebhookURL != "" {
		notifiers = append(notifiers, NewSlackNotifier(cfg))
	}

	if cfg.DiscordWebhookURL != "" {
		notifiers = append(notifiers, NewDiscordNotifier(cfg))
	}

	if cfg.WebhookURL != "" {
		notifiers = append(notifiers, NewWebhookNotifier(cfg))
	}

	return NewMultiNotifier(notifiers...)
}
