//go:build cloud

package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	cloudpkg "github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/mailer"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/session"
)

// InvitationsHandler manages team member invitations (Business+ cloud feature).
type InvitationsHandler struct {
	cloudStore cloudpkg.Store
	cfg        *config.Config
}

func NewInvitationsHandler(cfg *config.Config, cs cloudpkg.Store) *InvitationsHandler {
	return &InvitationsHandler{cloudStore: cs, cfg: cfg}
}

// RegisterInvitationRoutes wires up invitation management routes. Owner-only
// API routes sit behind RequireOwner + RequirePlan("business"). The accept page
// is public (the invite token is the authentication factor).
func RegisterInvitationRoutes(r chi.Router, cfg *config.Config, store db.Store, cs cloudpkg.Store) {
	h := NewInvitationsHandler(cfg, cs)

	// Accept page — public, no session required.
	r.Get("/accept-invite", h.AcceptPage)
	r.Post("/accept-invite", h.AcceptAction)

	// Owner-only management routes.
	r.With(middleware.RequireOwner(cfg.JWTSecret)).Group(func(r chi.Router) {
		r.Use(middleware.RequirePlan(store, cfg, "business"))
		r.Get("/api/admin/invitations", h.List)
		r.Post("/api/admin/invitations", h.Create)
		r.Delete("/api/admin/invitations/{id}", h.Delete)
	})
}

// List returns all team members (pending and accepted) for the calling account.
func (h *InvitationsHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(session.UserKey).(*session.Claims)

	members, err := h.cloudStore.ListTeamMembers(claims.AccountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load team members")
		return
	}
	if members == nil {
		members = []*cloudpkg.TeamMember{}
	}
	writeJSONOK(w, map[string]any{"members": members})
}

// Create sends an invitation email to the provided address.
func (h *InvitationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(session.UserKey).(*session.Claims)

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		writeJSONError(w, http.StatusBadRequest, "invalid email address")
		return
	}

	// Prevent inviting the account owner's own email.
	ownerAcc, err := h.cloudStore.GetAccountByID(claims.AccountID)
	if err != nil || ownerAcc == nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if strings.EqualFold(ownerAcc.Email, req.Email) {
		writeJSONError(w, http.StatusBadRequest, "cannot invite the account owner")
		return
	}

	// Idempotent: if already invited, re-send the email.
	existing, _ := h.cloudStore.GetTeamMemberByEmail(claims.AccountID, req.Email)
	var member *cloudpkg.TeamMember
	if existing != nil {
		member = existing
	} else {
		member = &cloudpkg.TeamMember{
			ID:          uuid.New().String(),
			AccountID:   claims.AccountID,
			Email:       req.Email,
			Role:        "admin",
			InviteToken: uuid.New().String(),
			InvitedAt:   time.Now().UTC(),
		}
		if err := h.cloudStore.CreateTeamMember(member); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to create invitation")
			return
		}
	}

	acceptURL := fmt.Sprintf("%s/accept-invite?token=%s", h.cfg.BaseURL, member.InviteToken)
	body := mailer.InviteEmailBody(ownerAcc.Email, acceptURL)
	if err := mailer.SendTransactional(h.cfg, req.Email, "You've been invited to Quipthread", body); err != nil {
		log.Printf("InvitationsHandler.Create: failed to send invite to %s: %v", req.Email, err)
	}

	writeJSONOK(w, member)
}

// Delete removes a team member invitation (pending or accepted).
func (h *InvitationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(session.UserKey).(*session.Claims)
	memberID := chi.URLParam(r, "id")

	if err := h.cloudStore.DeleteTeamMember(claims.AccountID, memberID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to remove team member")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AcceptPage renders the invitation accept page. The token in the query param
// is the authentication factor — no session is required to view the page.
func (h *InvitationsHandler) AcceptPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		renderInvitePage(w, "Invalid link", "This invitation link is missing a token.", false, "")
		return
	}

	member, err := h.cloudStore.GetTeamMemberByToken(token)
	if err != nil || member == nil {
		renderInvitePage(w, "Link not found", "This invitation link is invalid or has already been used.", false, "")
		return
	}

	if member.Accepted {
		// Already accepted — if logged in redirect to dashboard, else show sign-in.
		if cookie, err := r.Cookie(session.CookieName); err == nil {
			if _, err := session.Parse(h.cfg.JWTSecret, cookie.Value); err == nil {
				http.Redirect(w, r, h.cfg.BaseURL+"/dashboard/", http.StatusFound)
				return
			}
		}
		renderInvitePage(w, "Already accepted", "This invitation has been accepted. Please sign in to access the dashboard.", true, token)
		return
	}

	// Render accept form.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	escapedToken := html.EscapeString(token)
	escapedEmail := html.EscapeString(member.Email)
	fmt.Fprintf(w, invitePageHTML, escapedEmail, escapedToken) //nolint:errcheck,gosec // ResponseWriter.Write errors not actionable
}

// AcceptAction handles the POST when a user clicks "Accept Invitation".
func (h *InvitationsHandler) AcceptAction(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeJSONError(w, http.StatusBadRequest, "missing token")
		return
	}

	member, err := h.cloudStore.GetTeamMemberByToken(token)
	if err != nil || member == nil {
		writeJSONError(w, http.StatusNotFound, "invitation not found")
		return
	}

	if !member.Accepted {
		if err := h.cloudStore.AcceptTeamMember(token); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to accept invitation")
			return
		}
	}

	writeJSONOK(w, map[string]bool{"accepted": true})
}

// --- HTML templates ----------------------------------------------------------

func renderInvitePage(w http.ResponseWriter, title, message string, showSignIn bool, _ string) {
	signInLink := ""
	if showSignIn {
		signInLink = `<p style="margin-top:1.5rem"><a href="/login" style="color:#E07F32;font-weight:600;">Sign in to Quipthread</a></p>`
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, inviteStatusPageHTML, //nolint:errcheck,gosec // ResponseWriter.Write errors not actionable
		html.EscapeString(title),
		html.EscapeString(message),
		signInLink,
	)
}

const inviteStatusPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Quipthread — Invitation</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  :root { --ink:#0F0F0F; --surface:#1A1A1A; --border:#2A2A2A; --text:#E8E3DC; --muted:#8A8480; --amber:#E07F32; --paper:#F7F4EF; --p-surf:#EEEBE4; --p-bord:#D9D4CB; --p-text:#1A1714; --p-muted:#7A7570; }
  @media (prefers-color-scheme:light) { body { --bg:var(--paper);--surf:var(--p-surf);--bord:var(--p-bord);--fg:var(--p-text);--faded:var(--p-muted); } }
  @media (prefers-color-scheme:dark)  { body { --bg:var(--ink);--surf:var(--surface);--bord:var(--border);--fg:var(--text);--faded:var(--muted); } }
  body { font-family:system-ui,-apple-system,sans-serif;background:var(--bg,var(--paper));color:var(--fg,var(--p-text));min-height:100vh;display:flex;align-items:center;justify-content:center;padding:2rem 1rem; }
  .card { width:100%%;max-width:480px;background:var(--surf,var(--p-surf));border:1px solid var(--bord,var(--p-bord));border-radius:10px;overflow:hidden; }
  .card-header { padding:1.25rem 1.5rem;border-bottom:1px solid var(--bord,var(--p-bord));display:flex;align-items:center;gap:.5rem; }
  .logo { font-size:.875rem;font-weight:700;color:var(--amber);letter-spacing:.02em;text-transform:uppercase; }
  .sep { color:var(--faded,var(--p-muted)); }
  .card-title { font-size:.875rem;color:var(--faded,var(--p-muted)); }
  .body { padding:2rem 1.5rem; }
  .body p { font-size:.9375rem;color:var(--faded,var(--p-muted));line-height:1.6; }
</style>
</head>
<body>
<div class="card">
  <div class="card-header"><span class="logo">Quipthread</span><span class="sep">/</span><span class="card-title">Team Invitation</span></div>
  <div class="body">
    <p><strong style="color:inherit">%s</strong></p>
    <p style="margin-top:.75rem">%s</p>
    %s
  </div>
</div>
</body>
</html>`

const invitePageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Quipthread — Accept Invitation</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  :root { --ink:#0F0F0F; --surface:#1A1A1A; --border:#2A2A2A; --text:#E8E3DC; --muted:#8A8480; --amber:#E07F32; --amber-h:#F0A06A; --paper:#F7F4EF; --p-surf:#EEEBE4; --p-bord:#D9D4CB; --p-text:#1A1714; --p-muted:#7A7570; }
  @media (prefers-color-scheme:light) { body { --bg:var(--paper);--surf:var(--p-surf);--bord:var(--p-bord);--fg:var(--p-text);--faded:var(--p-muted); } }
  @media (prefers-color-scheme:dark)  { body { --bg:var(--ink);--surf:var(--surface);--bord:var(--border);--fg:var(--text);--faded:var(--muted); } }
  body { font-family:system-ui,-apple-system,sans-serif;background:var(--bg,var(--paper));color:var(--fg,var(--p-text));min-height:100vh;display:flex;align-items:center;justify-content:center;padding:2rem 1rem; }
  .card { width:100%%;max-width:480px;background:var(--surf,var(--p-surf));border:1px solid var(--bord,var(--p-bord));border-radius:10px;overflow:hidden; }
  .card-header { padding:1.25rem 1.5rem;border-bottom:1px solid var(--bord,var(--p-bord));display:flex;align-items:center;gap:.5rem; }
  .logo { font-size:.875rem;font-weight:700;color:var(--amber);letter-spacing:.02em;text-transform:uppercase; }
  .sep { color:var(--faded,var(--p-muted)); }
  .card-title { font-size:.875rem;color:var(--faded,var(--p-muted)); }
  .body { padding:2rem 1.5rem; }
  .body p { font-size:.9375rem;color:var(--faded,var(--p-muted));line-height:1.6;margin-bottom:1rem; }
  .email-chip { display:inline-block;padding:.25em .75em;background:var(--bord,var(--p-bord));border-radius:9999px;font-size:.875rem;font-weight:600;color:var(--fg,var(--p-text)); }
  .btn { margin-top:.5rem;width:100%%;padding:.75rem;background:var(--amber);color:#fff;border:none;border-radius:6px;font-size:.9375rem;font-weight:600;cursor:pointer;transition:background .15s; }
  .btn:hover:not(:disabled) { background:var(--amber-h); }
  .btn:disabled { opacity:.5;cursor:default; }
  .msg { margin-top:1rem;font-size:.875rem;text-align:center;color:var(--faded,var(--p-muted)); }
  .msg.error { color:#c0392b; }
</style>
</head>
<body>
<div class="card">
  <div class="card-header"><span class="logo">Quipthread</span><span class="sep">/</span><span class="card-title">Team Invitation</span></div>
  <div class="body">
    <p>You've been invited to join a Quipthread workspace as a team member.</p>
    <p>This invitation was sent to <span class="email-chip">%s</span>.</p>
    <p>Accepting will grant you access to the team's moderation dashboard. You'll need to sign in or create an account with this email address.</p>
    <button class="btn" id="btn" onclick="accept()">Accept Invitation</button>
    <div class="msg" id="msg"></div>
  </div>
</div>
<script>
async function accept() {
  const btn = document.getElementById('btn');
  const msg = document.getElementById('msg');
  btn.disabled = true;
  msg.className = 'msg';
  msg.textContent = '';
  try {
    const res = await fetch('/accept-invite?token=%s', { method: 'POST' });
    const data = await res.json();
    if (res.ok && data.accepted) {
      btn.style.display = 'none';
      msg.className = 'msg';
      msg.innerHTML = 'Invitation accepted! <a href="/login" style="color:#E07F32;font-weight:600;">Sign in to continue.</a>';
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

// writeJSONOK writes a 200 JSON response.
func writeJSONOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(v) //nolint:errcheck,gosec // response write; connection may already be broken
}

// writeJSONError writes an error JSON response.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck,gosec // response write; connection may already be broken
}
