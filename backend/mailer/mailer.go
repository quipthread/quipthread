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
	b.WriteString(fmt.Sprintf("From: %s\r\n", from))
	b.WriteString(fmt.Sprintf("To: %s\r\n", to))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return b.String()
}

// VerificationEmailBody returns the HTML body for an email verification message.
func VerificationEmailBody(displayName, verifyURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8">
<style>
  body { font-family: system-ui, sans-serif; background: #F7F4EF; color: #1A1714; margin: 0; padding: 24px; }
  .container { max-width: 560px; margin: 0 auto; background: #EEEBE4; border-radius: 8px; overflow: hidden; }
  .header { background: #1A1A1A; color: #E8E3DC; padding: 24px 32px; }
  .header h1 { margin: 0; font-size: 1rem; font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; color: #E07F32; }
  .body { padding: 32px; }
  .body p { margin: 0 0 1em; line-height: 1.6; }
  .btn { display: inline-block; padding: 12px 28px; background: #E07F32; color: #fff; border-radius: 6px; font-weight: 600; text-decoration: none; font-size: 0.9375rem; }
  .btn:hover { background: #F0A06A; }
  .footer { padding: 16px 32px; font-size: 0.75rem; color: #8A8480; border-top: 1px solid #D9D4CB; }
  .url { font-size: 0.8125rem; color: #8A8480; word-break: break-all; margin-top: 1em; }
</style>
</head>
<body>
<div class="container">
  <div class="header"><h1>Quipthread</h1></div>
  <div class="body">
    <p>Hi %s,</p>
    <p>Please verify your email address to activate your account.</p>
    <p><a class="btn" href="%s">Verify Email Address</a></p>
    <p class="url">Or copy this link: %s</p>
    <p>This link expires in 24 hours. If you didn't create a Quipthread account, you can safely ignore this email.</p>
  </div>
  <div class="footer">Sent by Quipthread.</div>
</div>
</body>
</html>`, displayName, verifyURL, verifyURL)
}

// PasswordResetEmailBody returns the HTML body for a password reset message.
func PasswordResetEmailBody(displayName, resetURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8">
<style>
  body { font-family: system-ui, sans-serif; background: #F7F4EF; color: #1A1714; margin: 0; padding: 24px; }
  .container { max-width: 560px; margin: 0 auto; background: #EEEBE4; border-radius: 8px; overflow: hidden; }
  .header { background: #1A1A1A; color: #E8E3DC; padding: 24px 32px; }
  .header h1 { margin: 0; font-size: 1rem; font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; color: #E07F32; }
  .body { padding: 32px; }
  .body p { margin: 0 0 1em; line-height: 1.6; }
  .btn { display: inline-block; padding: 12px 28px; background: #E07F32; color: #fff; border-radius: 6px; font-weight: 600; text-decoration: none; font-size: 0.9375rem; }
  .btn:hover { background: #F0A06A; }
  .footer { padding: 16px 32px; font-size: 0.75rem; color: #8A8480; border-top: 1px solid #D9D4CB; }
  .url { font-size: 0.8125rem; color: #8A8480; word-break: break-all; margin-top: 1em; }
</style>
</head>
<body>
<div class="container">
  <div class="header"><h1>Quipthread</h1></div>
  <div class="body">
    <p>Hi %s,</p>
    <p>We received a request to reset your password. Click the button below to choose a new one.</p>
    <p><a class="btn" href="%s">Reset Password</a></p>
    <p class="url">Or copy this link: %s</p>
    <p>This link expires in 1 hour. If you didn't request a password reset, you can safely ignore this email.</p>
  </div>
  <div class="footer">Sent by Quipthread.</div>
</div>
</body>
</html>`, displayName, resetURL, resetURL)
}
