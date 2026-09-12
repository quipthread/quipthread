package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/cloud/approvaltoken"
	"github.com/quipthread/quipthread/db"
)

// approvalUnavailable writes the deterministic 503 used while the approval
// routes are gated off in managed cloud mode (missing control-plane store or
// missing/invalid approval-token HMAC key). It follows the same response
// convention as the gated SSO routes (see admin_sso.go): JSON
// feature_unavailable carrying the chi request id.
//
// These disabled handlers receive no store reference at all: in cloud mode the
// approval tokens/comments live per-tenant, and without valid dependencies the
// routes must fail closed rather than resolve content through the global
// constructor store.
func approvalUnavailable(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusServiceUnavailable, "feature_unavailable")
}

// HandleApprovalPageUnavailable is the fail-closed replacement for
// GET /approve/{token} in managed cloud mode. Storeless by construction.
func HandleApprovalPageUnavailable(w http.ResponseWriter, r *http.Request) {
	approvalUnavailable(w, r)
}

// HandleApprovalActionUnavailable is the fail-closed replacement for
// POST /approve/{token} in managed cloud mode. Storeless by construction.
func HandleApprovalActionUnavailable(w http.ResponseWriter, r *http.Request) {
	approvalUnavailable(w, r)
}

// HandleApprovalPage renders the styled approval page for a given token.
// GET /approve/{token}
func HandleApprovalPage(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")

		at, err := store.GetApprovalToken(token)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if at == nil {
			http.Error(w, "token not found or already used", http.StatusNotFound)
			return
		}
		if time.Now().After(at.ExpiresAt) {
			http.Error(w, "approval link has expired", http.StatusGone)
			return
		}

		comment, err := store.GetComment(at.CommentID)
		if err != nil || comment == nil {
			http.Error(w, "comment not found", http.StatusNotFound)
			return
		}

		site, _ := store.GetSite(comment.SiteID)

		author := comment.AuthorName
		if author == "" {
			author = "Anonymous"
		}
		domain := ""
		if site != nil {
			domain = site.Domain
		}

		pageLabel := comment.PageTitle
		if pageLabel == "" {
			pageLabel = comment.PageURL
		}
		if pageLabel == "" {
			pageLabel = comment.PageID
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, approvalPageHTML, //nolint:errcheck,gosec // ResponseWriter.Write errors not actionable; HTML is pre-escaped by html.EscapeString above
			html.EscapeString(domain),
			html.EscapeString(author),
			html.EscapeString(pageLabel),
			html.EscapeString(comment.PageURL),
			comment.CreatedAt.Format("Jan 2, 2006 at 3:04 PM UTC"),
			comment.Content, // raw HTML from the editor — already sanitized on save
			token,
		)
	}
}

// HandleApprovalAction processes POST /approve/{token} with JSON body
// {"action":"approve"|"reject"}.
func HandleApprovalAction(store db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")

		at, err := store.GetApprovalToken(token)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if at == nil {
			jsonError(w, "token not found or already used", http.StatusNotFound)
			return
		}
		if time.Now().After(at.ExpiresAt) {
			jsonError(w, "approval link has expired", http.StatusGone)
			return
		}

		var body struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Action != "approve" && body.Action != "reject") {
			jsonError(w, `action must be "approve" or "reject"`, http.StatusBadRequest)
			return
		}

		comment, err := store.GetComment(at.CommentID)
		if err != nil || comment == nil {
			jsonError(w, "comment not found", http.StatusNotFound)
			return
		}

		switch body.Action {
		case "approve":
			comment.Status = "approved"
		case "reject":
			comment.Status = "rejected"
		}

		if err := store.UpdateComment(comment); err != nil {
			jsonError(w, "failed to update comment", http.StatusInternalServerError)
			return
		}

		// Single-use: delete the token so the link can't be replayed.
		_ = store.DeleteApprovalToken(token)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck,gosec // error response; connection may already be broken
			"ok":     true,
			"status": comment.Status,
		})
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck,gosec // error response; connection may already be broken
}

// --- Cloud approval flow -----------------------------------------------------
//
// The managed-cloud approval flow never touches the global/master content
// store. The central control plane holds only a hashed routing locator
// (cloud.ApprovalToken: account/site/expiry, keyed by the HMAC-SHA-256 hash of
// the raw bearer token); the raw token and its comment association live solely
// in the locator account's tenant store. Raw tokens and the HMAC key are never
// logged — failures are correlated by token hash only.

// ApprovalTenantStoreResolver opens the tenant db.Store owned by the given
// control-plane account. It is the narrow, injectable seam through which the
// cloud approval controller reaches tenant content: production wiring resolves
// the locator account's store through the shared bounded tenant-store cache,
// and tests substitute fakes. The returned release func must be called exactly
// once when the caller is finished with the store.
type ApprovalTenantStoreResolver func(accountID string) (store db.Store, release func(), err error)

// ApprovalCloudDeps carries the collaborators of the cloud approval controller.
// All fields are required; registration (see registerApprovalRoutes) mounts the
// cloud handlers only when every dependency is valid, so the handlers themselves
// fail closed via terminal errors rather than silent fallbacks.
type ApprovalCloudDeps struct {
	// CloudStore is the central control-plane store. Only hashed locators and
	// site-registry entries are read from it; raw tokens never reach it.
	CloudStore cloud.Store
	// HMACKey is the dedicated approval-token HMAC secret. The raw token is
	// hashed with it before any central lookup; the key itself is never logged
	// or persisted.
	HMACKey string
	// ResolveTenant opens the locator account's tenant store.
	ResolveTenant ApprovalTenantStoreResolver
	// Now returns the decision time; overridable for deterministic tests.
	Now func() time.Time
}

// Uniform rejection wording, identical to the self-hosted responses, so a
// rejected cloud approval link leaks no more than a self-hosted one already
// does: unknown, consumed, and registry-inconsistent locators all answer with
// the same 404, and only true expiry is distinguishable as 410.
const (
	approvalMsgNotFound = "token not found or already used"
	approvalMsgExpired  = "approval link has expired"
)

// approvalReject is a terminal caller-safe rejection: an HTTP status plus the
// exact response message. It never carries registry/topology details.
type approvalReject struct {
	status int
	msg    string
}

func (e *approvalReject) Error() string { return e.msg }

func approvalRejectNotFound() *approvalReject {
	return &approvalReject{http.StatusNotFound, approvalMsgNotFound}
}

func approvalRejectExpired() *approvalReject {
	return &approvalReject{http.StatusGone, approvalMsgExpired}
}

func approvalRejectInternal() *approvalReject {
	return &approvalReject{http.StatusInternalServerError, "internal error"}
}

func approvalRejectService() *approvalReject {
	return &approvalReject{http.StatusServiceUnavailable, "service unavailable"}
}

// resolveCloudApproval performs the shared pre-tenant validation for both the
// GET page and the POST action:
//
//  1. The raw path token is HMAC-hashed and looked up in the central locator
//     table. Unknown/consumed locators reject with the uniform 404.
//  2. Locator expiry is enforced against the central record.
//  3. The locator's site must have an active registration owned by the same
//     account the locator names (stale, inactive, or mismatched registry
//     entries reject with the uniform 404 — no registry-state leakage).
//  4. Only the locator account's tenant store is opened, via the injectable
//     resolver seam, and the site must actually exist in that tenant store.
//
// On success it returns the locator (whose TokenHash field identifies the
// central row), the tenant store, and the release func that must be deferred
// by the caller. The raw token is never logged anywhere in this path.
func (d ApprovalCloudDeps) resolveCloudApproval(rawToken string) (*cloud.ApprovalToken, db.Store, func(), *approvalReject) {
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}

	tokenHash := approvaltoken.Hash(d.HMACKey, rawToken)

	loc, err := d.CloudStore.GetApprovalToken(tokenHash)
	if err != nil {
		slog.ErrorContext(context.Background(), "approval locator lookup failed", "token_hash", tokenHash, "error", err)
		return nil, nil, nil, approvalRejectInternal()
	}
	if loc == nil {
		return nil, nil, nil, approvalRejectNotFound()
	}
	if !loc.ExpiresAt.After(now()) {
		return nil, nil, nil, approvalRejectExpired()
	}

	entry, err := d.CloudStore.GetActiveSiteRegistration(loc.SiteID)
	if err != nil {
		slog.ErrorContext(context.Background(), "approval site registry lookup failed", "token_hash", tokenHash, "error", err)
		return nil, nil, nil, approvalRejectInternal()
	}
	if entry == nil || entry.AccountID != loc.AccountID {
		// Stale, inactive, or account-mismatched registration: indistinguishable
		// from an unknown token on purpose.
		return nil, nil, nil, approvalRejectNotFound()
	}

	store, release, err := d.ResolveTenant(loc.AccountID)
	if err != nil {
		slog.ErrorContext(context.Background(), "approval tenant store open failed", "token_hash", tokenHash, "error", err)
		return nil, nil, nil, approvalRejectService()
	}

	site, err := store.GetSite(loc.SiteID)
	if err != nil {
		release()
		slog.ErrorContext(context.Background(), "approval tenant site lookup failed", "token_hash", tokenHash, "error", err)
		return nil, nil, nil, approvalRejectService()
	}
	if site == nil {
		// A registry entry without tenant rows is a consistency gap we must not
		// paper over with a global-store fallback.
		release()
		return nil, nil, nil, approvalRejectNotFound()
	}

	return loc, store, release, nil
}

// emitApprovalReject writes the rejection in the response style of the calling
// route: plain text for the server-rendered GET page, JSON for the POST action.
func emitApprovalReject(w http.ResponseWriter, rej *approvalReject, asJSON bool) {
	if asJSON {
		jsonError(w, rej.msg, rej.status)
		return
	}
	http.Error(w, rej.msg, rej.status)
}

// HandleCloudApprovalPage renders the styled approval page for a raw path
// token in managed cloud mode. GET /approve/{token}. It reads the tenant raw
// token and comment without consuming: the link stays usable until a POST
// action atomically consumes it.
func HandleCloudApprovalPage(d ApprovalCloudDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawToken := chi.URLParam(r, "token")

		loc, store, release, rej := d.resolveCloudApproval(rawToken)
		if rej != nil {
			emitApprovalReject(w, rej, false)
			return
		}
		defer release()

		// Tenant-owned raw token association. The central locator only routed
		// us here; the tenant store is the authority on the link itself.
		tt, err := store.GetApprovalToken(rawToken)
		if err != nil {
			slog.ErrorContext(r.Context(), "approval tenant token lookup failed", "token_hash", loc.TokenHash, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if tt == nil {
			http.Error(w, approvalMsgNotFound, http.StatusNotFound)
			return
		}
		if !tt.ExpiresAt.After(d.Now()) {
			http.Error(w, approvalMsgExpired, http.StatusGone)
			return
		}

		comment, err := store.GetComment(tt.CommentID)
		if err != nil || comment == nil {
			http.Error(w, "comment not found", http.StatusNotFound)
			return
		}

		// The verified tenant site was fetched during resolution; use its
		// domain for the page header exactly as the self-hosted renderer does.
		site, _ := store.GetSite(comment.SiteID)

		author := comment.AuthorName
		if author == "" {
			author = "Anonymous"
		}
		domain := ""
		if site != nil {
			domain = site.Domain
		}

		pageLabel := comment.PageTitle
		if pageLabel == "" {
			pageLabel = comment.PageURL
		}
		if pageLabel == "" {
			pageLabel = comment.PageID
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, approvalPageHTML, //nolint:errcheck,gosec // ResponseWriter.Write errors not actionable; HTML is pre-escaped by html.EscapeString above
			html.EscapeString(domain),
			html.EscapeString(author),
			html.EscapeString(pageLabel),
			html.EscapeString(comment.PageURL),
			comment.CreatedAt.Format("Jan 2, 2006 at 3:04 PM UTC"),
			comment.Content, // raw HTML from the editor — already sanitized on save
			rawToken,
		)
	}
}

// HandleCloudApprovalAction processes POST /approve/{token} with the existing
// JSON body {"action":"approve"|"reject"}. The tenant store's
// ConsumeApprovalToken is the atomic single-use primitive: it validates the
// raw token, applies the final comment status, and deletes the token in one
// transaction, so exactly one concurrent caller wins. After a successful
// tenant commit, the central locator is deleted best-effort. A locator-delete
// failure never replays the tenant mutation: the delete runs after the
// consume committed and is logged, not retried. This is deliberately not a
// distributed transaction — the worst case is a consumed link whose central
// locator lingers until the expiry sweep removes it.
func HandleCloudApprovalAction(d ApprovalCloudDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawToken := chi.URLParam(r, "token")

		loc, store, release, rej := d.resolveCloudApproval(rawToken)
		if rej != nil {
			emitApprovalReject(w, rej, true)
			return
		}
		defer release()

		// Existing JSON action behavior: parse and validate the body exactly
		// as the self-hosted action handler does.
		var body struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Action != "approve" && body.Action != "reject") {
			jsonError(w, `action must be "approve" or "reject"`, http.StatusBadRequest)
			return
		}

		// Map the action to the final comment status, exactly as the
		// self-hosted action handler does, before the atomic consume.
		status := body.Action
		switch body.Action {
		case "approve":
			status = "approved"
		case "reject":
			status = "rejected"
		}

		comment, err := store.ConsumeApprovalToken(rawToken, status, d.Now())
		switch {
		case errors.Is(err, db.ErrApprovalTokenNotFound):
			jsonError(w, approvalMsgNotFound, http.StatusNotFound)
			return
		case errors.Is(err, db.ErrApprovalTokenExpired):
			jsonError(w, approvalMsgExpired, http.StatusGone)
			return
		case errors.Is(err, db.ErrApprovalCommentNotFound):
			jsonError(w, "comment not found", http.StatusNotFound)
			return
		case err != nil:
			slog.ErrorContext(r.Context(), "approval tenant consume failed", "token_hash", loc.TokenHash, "error", err)
			jsonError(w, "failed to update comment", http.StatusInternalServerError)
			return
		}

		// Best-effort central cleanup after the tenant mutation committed. A
		// failure is logged with the token hash only and must never trigger a
		// second tenant mutation.
		if derr := d.CloudStore.DeleteApprovalToken(loc.TokenHash); derr != nil {
			slog.WarnContext(r.Context(), "approval locator cleanup failed", "token_hash", loc.TokenHash, "error", derr)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck,gosec // error response; connection may already be broken
			"ok":     true,
			"status": comment.Status,
		})
	}
}

// approvalPageHTML is a self-contained server-rendered page styled with
// Quipthread brand colors. Placeholders (in order):
//  1. site domain
//  2. author name
//  3. page label (title or URL or ID)
//  4. page URL (href)
//  5. comment created_at
//  6. comment HTML content
//  7. token (used in fetch URL)
const approvalPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Quipthread — Comment Approval</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

  :root {
    --ink:      #0F0F0F;
    --surface:  #1A1A1A;
    --border:   #2A2A2A;
    --text:     #E8E3DC;
    --muted:    #8A8480;
    --amber:    #E07F32;
    --amber-h:  #F0A06A;
    --amber-bg: #2a1508;
    --paper:    #F7F4EF;
    --p-surf:   #EEEBE4;
    --p-bord:   #D9D4CB;
    --p-text:   #1A1714;
    --p-muted:  #7A7570;
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
    align-items: flex-start;
    justify-content: center;
    padding: 3rem 1rem;
  }

  .card {
    width: 100%%;
    max-width: 600px;
    background: var(--surf, var(--p-surf));
    border: 1px solid var(--bord, var(--p-bord));
    border-radius: 10px;
    overflow: hidden;
  }

  .card-header {
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid var(--bord, var(--p-bord));
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .card-header .logo {
    font-size: 0.875rem;
    font-weight: 700;
    color: var(--amber);
    letter-spacing: 0.02em;
    text-transform: uppercase;
  }
  .card-header .sep { color: var(--faded, var(--p-muted)); }
  .card-header .domain { font-size: 0.875rem; color: var(--faded, var(--p-muted)); }

  .comment-block {
    padding: 1.5rem;
    border-bottom: 1px solid var(--bord, var(--p-bord));
  }

  .comment-meta {
    font-size: 0.8125rem;
    color: var(--faded, var(--p-muted));
    margin-bottom: 0.75rem;
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.5rem;
    align-items: baseline;
  }
  .comment-meta .author { font-weight: 600; color: var(--fg, var(--p-text)); }
  .comment-meta a { color: var(--amber); text-decoration: none; }
  .comment-meta a:hover { color: var(--amber-h); text-decoration: underline; }

  .comment-content {
    font-size: 0.9375rem;
    line-height: 1.65;
  }
  .comment-content p { margin: 0 0 0.75em; }
  .comment-content p:last-child { margin-bottom: 0; }
  .comment-content a { color: var(--amber); }

  .actions {
    padding: 1.25rem 1.5rem;
    display: flex;
    gap: 0.75rem;
    align-items: center;
  }

  .btn {
    padding: 0.625rem 1.25rem;
    border-radius: 6px;
    font-size: 0.9375rem;
    font-weight: 600;
    cursor: pointer;
    border: none;
    transition: opacity 0.15s;
  }
  .btn:disabled { opacity: 0.4; cursor: default; }
  .btn-approve {
    background: var(--amber);
    color: #fff;
  }
  .btn-approve:hover:not(:disabled) { background: var(--amber-h); }
  .btn-reject {
    background: transparent;
    color: var(--faded, var(--p-muted));
    border: 1px solid var(--bord, var(--p-bord));
  }
  .btn-reject:hover:not(:disabled) { color: var(--fg, var(--p-text)); }

  .status-badge {
    display: none;
    font-size: 0.875rem;
    font-weight: 600;
    padding: 0.25rem 0.75rem;
    border-radius: 999px;
  }
  .status-badge.approved { background: rgba(224,127,50,0.15); color: var(--amber); }
  .status-badge.rejected { background: rgba(138,132,128,0.15); color: var(--faded, var(--p-muted)); }

  /* Toast */
  .qt-toast {
    position: fixed; top: 1.5rem; right: 1.5rem; z-index: 9999;
    background: var(--surface, #1A1A1A); color: var(--text, #E8E3DC);
    border: 1px solid #2E2E2E; border-radius: 8px;
    padding: 0.875rem 1.25rem;
    box-shadow: 0 8px 30px rgba(0,0,0,0.4);
    font-size: 0.9375rem;
    animation: qt-slide-in 0.2s ease, qt-fade-out 0.3s ease 3.7s forwards;
  }
  @keyframes qt-slide-in {
    from { transform: translateX(110%%); opacity: 0; }
    to   { transform: none; opacity: 1; }
  }
  @keyframes qt-fade-out {
    to { opacity: 0; transform: translateX(110%%); }
  }
</style>
</head>
<body>
<div class="card">
  <div class="card-header">
    <span class="logo">Quipthread</span>
    <span class="sep">/</span>
    <span class="domain">%s</span>
  </div>

  <div class="comment-block">
    <div class="comment-meta">
      <span class="author">%s</span>
      <span>on</span>
      <a href="%s" target="_blank" rel="noopener noreferrer">%s</a>
      <span>&middot;</span>
      <span>%s</span>
    </div>
    <div class="comment-content">%s</div>
  </div>

  <div class="actions">
    <button class="btn btn-approve" id="btn-approve" onclick="act('approve')">Approve</button>
    <button class="btn btn-reject"  id="btn-reject"  onclick="act('reject')">Reject</button>
    <span class="status-badge" id="status-badge"></span>
  </div>
</div>

<script>
async function act(action) {
  const btnApprove = document.getElementById('btn-approve');
  const btnReject  = document.getElementById('btn-reject');
  const badge      = document.getElementById('status-badge');

  btnApprove.disabled = true;
  btnReject.disabled  = true;

  try {
    const res = await fetch('/approve/%s', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action }),
    });
    const data = await res.json();

    if (res.ok && data.ok) {
      badge.textContent = data.status === 'approved' ? 'Approved' : 'Rejected';
      badge.className = 'status-badge ' + data.status;
      badge.style.display = 'inline-block';
      showToast(data.status === 'approved'
        ? 'Comment approved successfully.'
        : 'Comment rejected.');
    } else {
      showToast((data && data.error) ? data.error : 'Something went wrong.', true);
      btnApprove.disabled = false;
      btnReject.disabled  = false;
    }
  } catch (e) {
    showToast('Network error — please try again.', true);
    btnApprove.disabled = false;
    btnReject.disabled  = false;
  }
}

function showToast(msg, isError) {
  const t = document.createElement('div');
  t.className = 'qt-toast';
  if (isError) t.style.borderColor = '#8A8480';
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), 4100);
}
</script>
</body>
</html>`
