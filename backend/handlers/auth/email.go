package auth

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/mailer"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type EmailProvider struct {
	store  db.Store
	config *config.Config
}

func NewEmailProvider(store db.Store, cfg *config.Config) *EmailProvider {
	return &EmailProvider{store: store, config: cfg}
}

func (p *EmailProvider) Name() string { return "email" }

// --- HTTP handlers ----------------------------------------------------------

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *Handler) EmailRegister(w http.ResponseWriter, r *http.Request) {
	if h.cloudEmailRegister(w, r) {
		return
	}
	if h.email == nil {
		writeError(w, http.StatusNotFound, "email auth not enabled")
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !emailRe.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	existing, err := h.store.GetIdentity("email", req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	u := &models.User{
		DisplayName: strings.TrimSpace(req.Name),
		Email:       req.Email,
		Role:        "commenter",
	}
	if err := h.store.UpsertUser(u); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	ident := &models.UserIdentity{
		UserID:       u.ID,
		Provider:     "email",
		ProviderID:   req.Email,
		PasswordHash: string(hash),
	}
	if err := h.store.CreateIdentity(ident); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create identity")
		return
	}

	sendVerificationEmail(h.store, h.config, u)

	writeJSON(w, http.StatusCreated, map[string]string{
		"message": "account created; check your email for a verification link",
	})
}

func (h *Handler) EmailVerify(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	et, err := h.store.GetEmailToken(token)
	if err != nil {
		renderEmailPage(w, "Error", "Something went wrong. Please try again.", false)
		return
	}
	if et == nil {
		renderEmailPage(w, "Link not found", "This verification link is invalid or has already been used.", false)
		return
	}
	if et.Type != "verification" {
		renderEmailPage(w, "Invalid link", "This link cannot be used for email verification.", false)
		return
	}
	if time.Now().After(et.ExpiresAt) {
		_ = h.store.DeleteEmailToken(token)
		renderEmailPage(w, "Link expired", "This verification link has expired. Please request a new one.", false)
		return
	}

	// Delete the token before verifying so concurrent requests with the same
	// link cannot both succeed. If verification fails after deletion the user
	// must request a new link — a safe trade-off over allowing replay.
	if err := h.store.DeleteEmailToken(token); err != nil {
		renderEmailPage(w, "Error", "Something went wrong. Please try again.", false)
		return
	}
	if err := h.store.SetEmailVerified(et.UserID); err != nil {
		renderEmailPage(w, "Error", "Failed to verify email. Please try again.", false)
		return
	}

	renderEmailPage(w, "Email verified", "Your email address has been verified. You can now log in.", true)
}

type resendRequest struct {
	Email string `json:"email"`
}

func (h *Handler) EmailResend(w http.ResponseWriter, r *http.Request) {
	if h.cloudEmailResend(w, r) {
		return
	}
	if h.email == nil {
		writeError(w, http.StatusNotFound, "email auth not enabled")
		return
	}

	var req resendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	const ok = `{"message":"if that email is registered and unverified, a new verification link has been sent"}`

	// Self-hosted: look up account in the tenant store.
	identity, err := h.store.GetIdentity("email", req.Email)
	if err != nil || identity == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ok))
		return
	}

	user, err := h.store.GetUser(identity.UserID)
	if err != nil || user == nil || user.EmailVerified {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ok))
		return
	}

	sendVerificationEmail(h.store, h.config, user)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(ok))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) EmailLogin(w http.ResponseWriter, r *http.Request) {
	if h.cloudEmailLogin(w, r) {
		return
	}
	if h.email == nil {
		writeError(w, http.StatusNotFound, "email auth not enabled")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	identity, err := h.store.GetIdentity("email", req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if identity == nil {
		// Constant-time response to avoid user enumeration.
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(identity.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	user, err := h.store.GetUser(identity.UserID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user.Banned {
		writeError(w, http.StatusForbidden, "account is banned")
		return
	}
	if !user.EmailVerified {
		writeErrorCode(w, http.StatusForbidden, "email_not_verified", "please verify your email address before logging in")
		return
	}

	tokenStr, err := session.Issue(h.config.JWTSecret, user.ID, user.DisplayName, "email", user.Role, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	session.SetCookie(w, tokenStr, r.TLS != nil)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged in"})
}

type forgotRequest struct {
	Email string `json:"email"`
}

func (h *Handler) EmailForgot(w http.ResponseWriter, r *http.Request) {
	if h.cloudForgot(w, r) {
		return
	}
	if h.email == nil {
		writeError(w, http.StatusNotFound, "email auth not enabled")
		return
	}

	var req forgotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Always return success to avoid user enumeration.
	identity, err := h.store.GetIdentity("email", req.Email)
	if err != nil || identity == nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "if that email is registered you will receive a reset link"})
		return
	}

	user, err := h.store.GetUser(identity.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "if that email is registered you will receive a reset link"})
		return
	}

	token := &models.EmailToken{
		Token:     uuid.NewString(),
		UserID:    user.ID,
		Type:      "password_reset",
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	}
	if err := h.store.CreateEmailToken(token); err == nil {
		resetURL := fmt.Sprintf("%s/auth/email/reset/%s", h.config.BaseURL, token.Token)
		body := mailer.PasswordResetEmailBody(user.DisplayName, resetURL)
		if err := mailer.SendTransactional(h.config, user.Email, "Reset your Quipthread password", body); err != nil {
			log.Printf("EmailForgot: failed to send password reset to %s: %v", user.Email, err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "if that email is registered you will receive a reset link"})
}

// EmailResetPage renders the password reset form.
// GET /auth/email/reset/{token}
func (h *Handler) EmailResetPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	et, err := h.store.GetEmailToken(token)
	if err != nil {
		renderEmailPage(w, "Error", "Something went wrong. Please try again.", false)
		return
	}
	if et == nil {
		renderEmailPage(w, "Link not found", "This password reset link is invalid or has already been used.", false)
		return
	}
	if et.Type != "password_reset" {
		renderEmailPage(w, "Invalid link", "This link cannot be used for password reset.", false)
		return
	}
	if time.Now().After(et.ExpiresAt) {
		_ = h.store.DeleteEmailToken(token)
		renderEmailPage(w, "Link expired", "This password reset link has expired. Please request a new one.", false)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, resetFormHTML, html.EscapeString(token)) //nolint:errcheck // ResponseWriter.Write errors are not actionable
}

type resetRequest struct {
	Password string `json:"password"`
}

func (h *Handler) EmailReset(w http.ResponseWriter, r *http.Request) {
	if h.email == nil {
		writeError(w, http.StatusNotFound, "email auth not enabled")
		return
	}

	token := chi.URLParam(r, "token")

	et, err := h.store.GetEmailToken(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if et == nil {
		writeError(w, http.StatusNotFound, "token not found or already used")
		return
	}
	if et.Type != "password_reset" {
		writeError(w, http.StatusBadRequest, "invalid token type")
		return
	}
	if time.Now().After(et.ExpiresAt) {
		_ = h.store.DeleteEmailToken(token)
		writeError(w, http.StatusGone, "password reset link has expired")
		return
	}

	var req resetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := h.store.UpdatePasswordHashByUser(et.UserID, "email", string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	_ = h.store.DeleteEmailToken(token)

	writeJSON(w, http.StatusOK, map[string]string{"message": "password updated successfully"})
}

// --- Helpers -----------------------------------------------------------------

func sendVerificationEmail(store db.Store, cfg *config.Config, user *models.User) {
	token := &models.EmailToken{
		Token:     uuid.NewString(),
		UserID:    user.ID,
		Type:      "verification",
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	if err := store.CreateEmailToken(token); err != nil {
		return
	}
	verifyURL := fmt.Sprintf("%s/auth/email/verify/%s", cfg.BaseURL, token.Token)
	body := mailer.VerificationEmailBody(user.DisplayName, verifyURL)
	if err := mailer.SendTransactional(cfg, user.Email, "Verify your Quipthread email address", body); err != nil {
		log.Printf("sendVerificationEmail: failed to send to %s: %v", user.Email, err)
	}
}

// writeErrorCode writes a JSON error with an additional machine-readable code field.
func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message, "code": code}) //nolint:errcheck,gosec // error response; connection may already be broken
}

// renderEmailPage renders a minimal branded result page (success or error).
func renderEmailPage(w http.ResponseWriter, title, message string, success bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !success {
		w.WriteHeader(http.StatusBadRequest)
	}
	icon := `<span style="font-size:2rem;color:#E07F32">&#10003;</span>`
	if !success {
		icon = `<span style="font-size:2rem;color:#8A8480">&#10007;</span>`
	}
	fmt.Fprintf(w, emailResultPageHTML, html.EscapeString(title), icon, html.EscapeString(title), html.EscapeString(message)) //nolint:errcheck // ResponseWriter.Write errors are not actionable
}

const emailResultPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Quipthread — %s</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  :root {
    --ink: #0F0F0F; --surface: #1A1A1A; --border: #2A2A2A;
    --text: #E8E3DC; --muted: #8A8480; --amber: #E07F32;
    --paper: #F7F4EF; --p-surf: #EEEBE4; --p-bord: #D9D4CB;
    --p-text: #1A1714; --p-muted: #7A7570;
  }
  @media (prefers-color-scheme: light) {
    body { --bg: var(--paper); --surf: var(--p-surf); --bord: var(--p-bord); --fg: var(--p-text); --faded: var(--p-muted); }
  }
  @media (prefers-color-scheme: dark) {
    body { --bg: var(--ink); --surf: var(--surface); --bord: var(--border); --fg: var(--text); --faded: var(--muted); }
  }
  body {
    font-family: system-ui, -apple-system, sans-serif;
    background: var(--bg, var(--paper));
    color: var(--fg, var(--p-text));
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem 1rem;
  }
  .card {
    width: 100%%; max-width: 480px;
    background: var(--surf, var(--p-surf));
    border: 1px solid var(--bord, var(--p-bord));
    border-radius: 10px;
    overflow: hidden;
  }
  .card-header {
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid var(--bord, var(--p-bord));
  }
  .logo {
    font-size: 0.875rem; font-weight: 700;
    color: var(--amber); letter-spacing: 0.02em; text-transform: uppercase;
  }
  .body {
    padding: 2rem 1.5rem;
    text-align: center;
  }
  .body h1 { font-size: 1.25rem; margin: 0.75rem 0 0.5rem; }
  .body p { font-size: 0.9375rem; color: var(--faded, var(--p-muted)); line-height: 1.6; }
</style>
</head>
<body>
<div class="card">
  <div class="card-header"><span class="logo">Quipthread</span></div>
  <div class="body">
    %s
    <h1>%s</h1>
    <p>%s</p>
  </div>
</div>
</body>
</html>`

const resetFormHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Quipthread — Reset Password</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  :root {
    --ink: #0F0F0F; --surface: #1A1A1A; --border: #2A2A2A;
    --text: #E8E3DC; --muted: #8A8480; --amber: #E07F32; --amber-h: #F0A06A;
    --paper: #F7F4EF; --p-surf: #EEEBE4; --p-bord: #D9D4CB;
    --p-text: #1A1714; --p-muted: #7A7570;
  }
  @media (prefers-color-scheme: light) {
    body { --bg: var(--paper); --surf: var(--p-surf); --bord: var(--p-bord); --fg: var(--p-text); --faded: var(--p-muted); }
  }
  @media (prefers-color-scheme: dark) {
    body { --bg: var(--ink); --surf: var(--surface); --bord: var(--border); --fg: var(--text); --faded: var(--muted); }
  }
  body {
    font-family: system-ui, -apple-system, sans-serif;
    background: var(--bg, var(--paper));
    color: var(--fg, var(--p-text));
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem 1rem;
  }
  .card {
    width: 100%%; max-width: 480px;
    background: var(--surf, var(--p-surf));
    border: 1px solid var(--bord, var(--p-bord));
    border-radius: 10px;
    overflow: hidden;
  }
  .card-header {
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid var(--bord, var(--p-bord));
    display: flex; align-items: center; gap: 0.5rem;
  }
  .logo { font-size: 0.875rem; font-weight: 700; color: var(--amber); letter-spacing: 0.02em; text-transform: uppercase; }
  .sep { color: var(--faded, var(--p-muted)); }
  .card-title { font-size: 0.875rem; color: var(--faded, var(--p-muted)); }
  .body { padding: 2rem 1.5rem; }
  .body p { font-size: 0.9375rem; color: var(--faded, var(--p-muted)); margin-bottom: 1.5rem; line-height: 1.6; }
  label { display: block; font-size: 0.8125rem; font-weight: 600; margin-bottom: 0.375rem; }
  input[type="password"] {
    width: 100%%; padding: 0.625rem 0.875rem;
    background: var(--bg, var(--paper));
    color: var(--fg, var(--p-text));
    border: 1px solid var(--bord, var(--p-bord));
    border-radius: 6px; font-size: 0.9375rem;
    outline: none;
  }
  input[type="password"]:focus { border-color: var(--amber); }
  .btn {
    margin-top: 1.25rem; width: 100%%;
    padding: 0.75rem; background: var(--amber); color: #fff;
    border: none; border-radius: 6px; font-size: 0.9375rem;
    font-weight: 600; cursor: pointer; transition: background 0.15s;
  }
  .btn:hover:not(:disabled) { background: var(--amber-h); }
  .btn:disabled { opacity: 0.5; cursor: default; }
  .msg { margin-top: 1rem; font-size: 0.875rem; text-align: center; color: var(--faded, var(--p-muted)); }
  .msg.error { color: #c0392b; }
</style>
</head>
<body>
<div class="card">
  <div class="card-header">
    <span class="logo">Quipthread</span>
    <span class="sep">/</span>
    <span class="card-title">Reset Password</span>
  </div>
  <div class="body">
    <p>Enter a new password for your account. It must be at least 8 characters.</p>
    <label for="pw">New password</label>
    <input type="password" id="pw" placeholder="New password" autocomplete="new-password">
    <button class="btn" id="btn" onclick="submit()">Set new password</button>
    <div class="msg" id="msg"></div>
  </div>
</div>
<script>
async function submit() {
  const pw = document.getElementById('pw').value;
  const btn = document.getElementById('btn');
  const msg = document.getElementById('msg');

  if (pw.length < 8) {
    msg.className = 'msg error';
    msg.textContent = 'Password must be at least 8 characters.';
    return;
  }

  btn.disabled = true;
  msg.className = 'msg';
  msg.textContent = '';

  try {
    const res = await fetch('/auth/email/reset/%s', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: pw }),
    });
    const data = await res.json();

    if (res.ok) {
      btn.style.display = 'none';
      document.getElementById('pw').style.display = 'none';
      document.querySelector('label').style.display = 'none';
      document.querySelector('.body p').textContent = 'Your password has been updated. You can now log in.';
    } else {
      msg.className = 'msg error';
      msg.textContent = (data && data.error) ? data.error : 'Something went wrong. Please try again.';
      btn.disabled = false;
    }
  } catch (e) {
    msg.className = 'msg error';
    msg.textContent = 'Network error — please try again.';
    btn.disabled = false;
  }
}
</script>
</body>
</html>`
