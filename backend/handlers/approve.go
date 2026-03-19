package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/db"
)

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
