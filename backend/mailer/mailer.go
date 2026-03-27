package mailer

import (
	"fmt"
	"net/smtp"
	"strings"

	"github.com/quipthread/quipthread/config"
)

// SendTransactional sends a single HTML email via SMTP.
// Returns nil immediately if SMTP is not configured.
func SendTransactional(cfg *config.Config, to, subject, body string) error {
	if cfg.SMTPHost == "" || cfg.SMTPFrom == "" {
		return nil
	}

	msg := buildMessage(cfg.SMTPFrom, to, subject, body)

	addr := fmt.Sprintf("%s:%s", cfg.SMTPHost, cfg.SMTPPort)
	var auth smtp.Auth
	if cfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)
	}
	return smtp.SendMail(addr, auth, cfg.SMTPFrom, []string{to}, []byte(msg))
}

func buildMessage(from, to, subject, htmlBody string) string {
	var b strings.Builder
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "From: %s\r\n", from)       //nolint:errcheck // strings.Builder.Write never fails
	fmt.Fprintf(&b, "To: %s\r\n", to)           //nolint:errcheck // strings.Builder.Write never fails
	fmt.Fprintf(&b, "Subject: %s\r\n", subject) //nolint:errcheck // strings.Builder.Write never fails
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return b.String()
}

// emailShell wraps content in the shared email chrome (header, footer, fonts).
func emailShell(body string) string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Syne:wght@700&display=swap" rel="stylesheet">
<style>
  body { font-family: system-ui, sans-serif; background: #F7F4EF; color: #1A1714; margin: 0; padding: 24px; }
  .container { max-width: 560px; margin: 0 auto; background: #EEEBE4; border-radius: 8px; overflow: hidden; }
  .header { background: #1A1A1A; padding: 24px 32px; }
  .header h1 { margin: 0; font-family: 'Syne', system-ui, sans-serif; font-size: 1.125rem; font-weight: 700; color: #E8E3DC; letter-spacing: 0.01em; }
  .body { padding: 32px; }
  .body p { margin: 0 0 1em; line-height: 1.6; }
  .btn { display: inline-block; padding: 12px 28px; background: #E07F32; color: #fff !important; border-radius: 6px; font-weight: 600; text-decoration: none; font-size: 0.9375rem; }
  .footer { padding: 16px 32px; font-size: 0.75rem; color: #8A8480; border-top: 1px solid #D9D4CB; }
  .url { font-size: 0.8125rem; color: #8A8480; word-break: break-all; margin-top: 1em; }
  .tips { margin: 1.25em 0; padding: 0; list-style: none; }
  .tips li { padding: 0.75rem 1rem; background: white; border: 1px solid #D9D4CB; border-radius: 6px; margin-bottom: 0.5rem; font-size: 0.9rem; line-height: 1.5; }
  .tips li strong { display: block; color: #1A1714; margin-bottom: 0.2em; }
</style>
</head>
<body>
<div class="container">
  <div class="header"><h1>Quipthread</h1></div>
  <div class="body">` + body + `</div>
  <div class="footer">Quipthread &mdash; quipthread.com</div>
</div>
</body>
</html>`
}

// VerificationEmailBody returns the HTML body for an email verification message.
func VerificationEmailBody(displayName, verifyURL string) string {
	body := fmt.Sprintf(`
    <p>Hi %s,</p>
    <p>Please verify your email address to activate your Quipthread account.</p>
    <p><a class="btn" href="%s">Verify Email Address</a></p>
    <p class="url">Or copy this link: %s</p>
    <p>This link expires in 24 hours. If you didn't create a Quipthread account, you can safely ignore this email.</p>
`, displayName, verifyURL, verifyURL)
	return emailShell(body)
}

// PasswordResetEmailBody returns the HTML body for a password reset message.
func PasswordResetEmailBody(displayName, resetURL string) string {
	body := fmt.Sprintf(`
    <p>Hi %s,</p>
    <p>We received a request to reset your Quipthread password. Click the button below to choose a new one.</p>
    <p><a class="btn" href="%s">Reset Password</a></p>
    <p class="url">Or copy this link: %s</p>
    <p>This link expires in 1 hour. If you didn't request a password reset, you can safely ignore this email.</p>
`, displayName, resetURL, resetURL)
	return emailShell(body)
}

// WelcomeEmailBody returns the HTML body for the post-verification welcome message.
func WelcomeEmailBody(displayName string) string {
	body := fmt.Sprintf(`
    <p>Hi %s,</p>
    <p>Thanks for joining Quipthread &mdash; your account is verified and ready to go.</p>
    <p>Here are a few things to do first:</p>
    <ul class="tips">
      <li><strong>Add your first site</strong>Give your site a name and domain. This creates a dedicated moderation queue and generates your embed code.</li>
      <li><strong>Install the widget</strong>Paste a single &lt;script&gt; tag on any page. Your onboarding wizard shows the exact snippet for your framework.</li>
      <li><strong>Set up notifications</strong>Get notified by email, Slack, Discord, or Telegram when new comments arrive. Configure this under your site settings.</li>
      <li><strong>Pick a theme</strong>Choose from 17 built-in themes or use CSS variables to match your site&#39;s branding exactly.</li>
    </ul>
    <p>If you run into anything, reply to this email &mdash; we read every message.</p>
    <p>Welcome aboard,<br>The Quipthread Team</p>
`, displayName)
	return emailShell(body)
}
