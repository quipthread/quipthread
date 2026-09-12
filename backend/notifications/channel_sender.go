package notifications

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// The channel values are the stable names stored in notification outbox
// records. Keep this list closed: callers must not be able to turn an
// arbitrary persisted string into a provider invocation.
const (
	ChannelEmail    = "email"
	ChannelSlack    = "slack"
	ChannelDiscord  = "discord"
	ChannelTelegram = "telegram"
	ChannelWebhook  = "webhook"
)

// ChannelErrorKind is safe to persist. It intentionally contains no provider
// response, URL, recipient, or other request data.
type ChannelErrorKind string

const (
	ChannelErrorInvalidChannel       ChannelErrorKind = "invalid_channel"
	ChannelErrorNotConfigured        ChannelErrorKind = "not_configured"
	ChannelErrorRecipientUnavailable ChannelErrorKind = "recipient_unavailable"
	ChannelErrorProvider             ChannelErrorKind = "provider_failure"
	ChannelErrorInvalidRequest       ChannelErrorKind = "invalid_request"
)

var (
	ErrInvalidChannel        = errors.New("notification channel is invalid")
	ErrChannelNotConfigured  = errors.New("notification channel is not configured")
	ErrRecipientUnavailable  = errors.New("notification recipient is unavailable")
	ErrChannelProvider       = errors.New("notification provider failed")
	ErrChannelInvalidRequest = errors.New("notification request is invalid")
)

// ChannelError is the only provider error exposed by the named sender. Its
// Error method is deliberately made from an allowlisted channel and a fixed
// error kind, so Error() is suitable for persistence.
type ChannelError struct {
	Channel string
	Kind    ChannelErrorKind
}

func (e *ChannelError) Error() string {
	if e == nil {
		return "notification failed"
	}
	kind := e.Kind
	switch kind {
	case ChannelErrorInvalidChannel, ChannelErrorNotConfigured,
		ChannelErrorRecipientUnavailable, ChannelErrorProvider,
		ChannelErrorInvalidRequest:
	default:
		kind = ChannelErrorProvider
	}
	if !isNotificationChannel(e.Channel) {
		return "notification failed: " + string(kind)
	}
	return "notification " + e.Channel + ": " + string(kind)
}

func (e *ChannelError) Is(target error) bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case ChannelErrorInvalidChannel:
		return target == ErrInvalidChannel
	case ChannelErrorNotConfigured:
		return target == ErrChannelNotConfigured
	case ChannelErrorRecipientUnavailable:
		return target == ErrRecipientUnavailable
	case ChannelErrorProvider:
		return target == ErrChannelProvider
	case ChannelErrorInvalidRequest:
		return target == ErrChannelInvalidRequest
	default:
		return false
	}
}

// ChannelSender invokes one, and only one, named notification channel.
type ChannelSender interface {
	Send(ctx context.Context, channel string, b Batch) error
}

// NamedChannelSender is a narrow boundary for callers that select a channel
// from persisted state. It does not send anything until Send is called.
type NamedChannelSender struct {
	notifiers map[string]Notifier
}

// NewNamedChannelSender constructs a sender from channel-specific notifiers.
// The map is copied so callers cannot change routing while a send is running.
func NewNamedChannelSender(notifiers map[string]Notifier) *NamedChannelSender {
	owned := make(map[string]Notifier, len(notifiers))
	for channel, notifier := range notifiers {
		if isNotificationChannel(channel) && notifier != nil {
			owned[channel] = notifier
		}
	}
	return &NamedChannelSender{notifiers: owned}
}

// NewChannelSender is the concise constructor for the ChannelSender boundary.
func NewChannelSender(notifiers map[string]Notifier) *NamedChannelSender {
	return NewNamedChannelSender(notifiers)
}

// Send invokes exactly the selected logical channel. Underlying provider
// errors are reduced to ChannelError before crossing this boundary.
func (s *NamedChannelSender) Send(ctx context.Context, channel string, b Batch) error {
	if !isNotificationChannel(channel) {
		return &ChannelError{Kind: ChannelErrorInvalidChannel}
	}
	if ctx == nil {
		return &ChannelError{Channel: channel, Kind: ChannelErrorInvalidRequest}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateBatch(b); err != nil {
		return &ChannelError{Channel: channel, Kind: ChannelErrorInvalidRequest}
	}
	if s == nil || s.notifiers[channel] == nil {
		return &ChannelError{Channel: channel, Kind: ChannelErrorNotConfigured}
	}

	if err := s.notifiers[channel].NotifyBatch(ctx, b); err != nil {
		return safeChannelError(channel, err)
	}
	return nil
}

// NotifyChannel is an alias for callers that use the existing Notify naming.
func (s *NamedChannelSender) NotifyChannel(ctx context.Context, channel string, b Batch) error {
	return s.Send(ctx, channel, b)
}

func isNotificationChannel(channel string) bool {
	switch channel {
	case ChannelEmail, ChannelSlack, ChannelDiscord, ChannelTelegram, ChannelWebhook:
		return true
	default:
		return false
	}
}

func safeChannelError(channel string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if channel == ChannelEmail && errors.Is(err, ErrRecipientUnavailable) {
		return &ChannelError{Channel: channel, Kind: ChannelErrorRecipientUnavailable}
	}
	if errors.Is(err, ErrChannelNotConfigured) {
		return &ChannelError{Channel: channel, Kind: ChannelErrorNotConfigured}
	}
	return &ChannelError{Channel: channel, Kind: ChannelErrorProvider}
}

func channelError(channel string, kind ChannelErrorKind) error {
	return &ChannelError{Channel: channel, Kind: kind}
}

func validateBatch(b Batch) error {
	if b.Site == nil || strings.TrimSpace(b.Site.ID) == "" || len(b.Comments) == 0 {
		return ErrChannelInvalidRequest
	}

	seen := make(map[string]struct{}, len(b.Comments))
	for _, comment := range b.Comments {
		if comment == nil || strings.TrimSpace(comment.ID) == "" || comment.Status != "pending" || comment.SiteID != b.Site.ID {
			return ErrChannelInvalidRequest
		}
		if _, exists := seen[comment.ID]; exists {
			return ErrChannelInvalidRequest
		}
		seen[comment.ID] = struct{}{}

		approveURL, approveOK := b.ApproveURLs[comment.ID]
		rejectURL, rejectOK := b.RejectURLs[comment.ID]
		if !approveOK || !validApprovalURL(approveURL) || !rejectOK || !validApprovalURL(rejectURL) {
			return ErrChannelInvalidRequest
		}
	}
	return nil
}

func validApprovalURL(raw string) bool {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return false
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}
