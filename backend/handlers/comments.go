package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/sanitize"
	"github.com/quipthread/quipthread/session"
)

const dedupWindow = 5 * time.Minute

type CommentsHandler struct {
	store               db.Store
	config              *config.Config
	spamChecker         *middleware.SpamChecker
	blockedTermsChecker *middleware.BlockedTermsChecker
}

func NewCommentsHandler(store db.Store, cfg *config.Config) *CommentsHandler {
	return &CommentsHandler{
		store:               store,
		config:              cfg,
		spamChecker:         middleware.NewSpamChecker(cfg),
		blockedTermsChecker: middleware.NewBlockedTermsChecker(store),
	}
}

func (h *CommentsHandler) db(r *http.Request) db.Store {
	if s, ok := db.StoreFromContext(r.Context()); ok {
		return s
	}
	return h.store
}

// GET /api/comments?siteId=&pageId=&page=1&limit=10&sort=newest|oldest|top
func (h *CommentsHandler) List(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	q := r.URL.Query()
	siteID := q.Get("siteId")
	pageID := q.Get("pageId")
	if siteID == "" || pageID == "" {
		writeError(w, r, http.StatusBadRequest, "siteId and pageId are required")
		return
	}

	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 10)
	if limit > 100 {
		limit = 100
	}

	sort := q.Get("sort")
	switch sort {
	case "oldest", "top":
		// valid
	default:
		sort = "newest"
	}

	// Populate user_voted when a valid session is present.
	var userID string
	if claims := claimsFromContext(r); claims != nil {
		userID = claims.Sub
	}

	comments, total, err := store.ListComments(siteID, pageID, sort, userID, page, limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to list comments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"comments": comments,
		"total":    total,
		"page":     page,
		"limit":    limit,
	})
}

// POST /api/comments/:id/vote — toggles an upvote; requires auth.
func (h *CommentsHandler) Vote(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromContext(r)
	if claims == nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	comment, err := store.GetComment(id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	if comment == nil {
		writeError(w, r, http.StatusNotFound, "comment not found")
		return
	}

	upvotes, voted, err := store.ToggleVote(id, claims.Sub)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to record vote")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"upvotes":    upvotes,
		"user_voted": voted,
	})
}

// POST /api/comments/:id/flag — toggles a flag; requires auth.
func (h *CommentsHandler) Flag(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromContext(r)
	if claims == nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	comment, err := store.GetComment(id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	if comment == nil {
		writeError(w, r, http.StatusNotFound, "comment not found")
		return
	}
	if comment.UserID == claims.Sub {
		writeError(w, r, http.StatusForbidden, "cannot flag your own comment")
		return
	}

	_, flagged, err := store.ToggleFlag(id, claims.Sub)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to record flag")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_flagged": flagged,
	})
}

type createCommentRequest struct {
	SiteID         string `json:"site_id"`
	PageID         string `json:"page_id"`
	PageURL        string `json:"page_url"`
	PageTitle      string `json:"page_title"`
	ParentID       string `json:"parent_id"`
	Content        string `json:"content"`
	TurnstileToken string `json:"turnstile_token"`
}

// POST /api/comments
func (h *CommentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromContext(r)
	if claims == nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SiteID == "" || req.PageID == "" || req.Content == "" {
		writeError(w, r, http.StatusBadRequest, "site_id, page_id, and content are required")
		return
	}

	req.Content = sanitize.CommentHTML(req.Content)
	if req.Content == "" {
		writeError(w, r, http.StatusBadRequest, "comment content is empty after sanitization")
		return
	}

	if req.PageID == "__preview__" {
		writeError(w, r, http.StatusForbidden, "preview_mode")
		return
	}

	// Prefer account-level Turnstile secret key; fall back to global env var.
	turnstileSecret := h.config.TurnstileSecretKey
	if accountSecret, err := func() (string, error) {
		_, s, err := store.GetTurnstileKeys()
		return s, err
	}(); err == nil && accountSecret != "" {
		turnstileSecret = accountSecret
	}
	if turnstileSecret != "" {
		ip := middleware.RealIP(r)
		ok, err := verifyTurnstile(r.Context(), turnstileSecret, req.TurnstileToken, ip)
		if err != nil || !ok {
			writeError(w, r, http.StatusForbidden, "bot check failed")
			return
		}
	}

	// Run heuristic spam detection and blocked terms check. Both auto-reject
	// and persist so admins have an audit trail; the response mirrors the
	// pending flow so the author sees "awaiting approval" rather than an error.
	isHeuristicSpam, _ := h.spamChecker.IsSpam(req.Content)
	isBlocked, _ := h.blockedTermsChecker.ContainsBlockedTerm(req.Content)
	if isHeuristicSpam || isBlocked {
		comment := &models.Comment{
			SiteID:    req.SiteID,
			PageID:    req.PageID,
			PageURL:   req.PageURL,
			PageTitle: req.PageTitle,
			ParentID:  req.ParentID,
			UserID:    claims.Sub,
			Content:   req.Content,
			Status:    "rejected",
		}
		_ = store.CreateComment(comment)
		writeJSON(w, http.StatusCreated, comment)
		return
	}

	// Duplicate detection: if identical content was posted by the same user to
	// the same page within the dedup window, return the existing comment silently.
	if existing, err := store.FindDuplicateComment(claims.Sub, req.PageID, req.Content, time.Now().Add(-dedupWindow)); err == nil && existing != nil {
		writeJSON(w, http.StatusCreated, existing)
		return
	}

	// Shadow-banned users: always store as approved so they see their comment,
	// but ListComments filters them out for all other users.
	var status string
	if u, err := store.GetUser(claims.Sub); err == nil && u != nil && u.ShadowBanned {
		status = "approved"
	} else {
		// Auto-approve if user has previously approved comments on this site.
		status = "pending"
		count, err := store.CountApprovedCommentsByUser(claims.Sub, req.SiteID)
		if err == nil && count > 0 {
			status = "approved"
		}
	}

	comment := &models.Comment{
		SiteID:    req.SiteID,
		PageID:    req.PageID,
		PageURL:   req.PageURL,
		PageTitle: req.PageTitle,
		ParentID:  req.ParentID,
		UserID:    claims.Sub,
		Content:   req.Content,
		Status:    status,
	}

	if err := store.CreateComment(comment); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to create comment")
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

// DELETE /api/comments/:id
func (h *CommentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromContext(r)
	if claims == nil {
		writeError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	comment, err := store.GetComment(id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	if comment == nil {
		writeError(w, r, http.StatusNotFound, "comment not found")
		return
	}
	if comment.UserID != claims.Sub && claims.Role != "admin" {
		writeError(w, r, http.StatusForbidden, "forbidden")
		return
	}

	if err := store.DeleteComment(id); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to delete comment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ----------------------------------------------------------------

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func claimsFromContext(r *http.Request) *session.Claims {
	v := r.Context().Value(session.UserKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*session.Claims)
	return c
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck,gosec // error response; connection may already be broken
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	reqID := chimiddleware.GetReqID(r.Context())
	if status >= 500 {
		slog.ErrorContext(r.Context(), "request error", "request_id", reqID, "status", status, "error", msg, "path", r.URL.Path)
	}
	writeJSON(w, status, map[string]string{"error": msg, "request_id": reqID})
}
