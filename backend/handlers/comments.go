package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/session"
)

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

// GET /api/comments?siteId=&pageId=&page=1&limit=10
func (h *CommentsHandler) List(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	siteID := r.URL.Query().Get("siteId")
	pageID := r.URL.Query().Get("pageId")
	if siteID == "" || pageID == "" {
		writeError(w, http.StatusBadRequest, "siteId and pageId are required")
		return
	}

	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 10)
	if limit > 100 {
		limit = 100
	}

	comments, total, err := store.ListComments(siteID, pageID, page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"comments": comments,
		"total":    total,
		"page":     page,
		"limit":    limit,
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
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SiteID == "" || req.PageID == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "site_id, page_id, and content are required")
		return
	}

	if req.PageID == "__preview__" {
		writeError(w, http.StatusForbidden, "preview_mode")
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
			writeError(w, http.StatusForbidden, "bot check failed")
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

	// Auto-approve if user has previously approved comments on this site.
	status := "pending"
	count, err := store.CountApprovedCommentsByUser(claims.Sub, req.SiteID)
	if err == nil && count > 0 {
		status = "approved"
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
		writeError(w, http.StatusInternalServerError, "failed to create comment")
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

// DELETE /api/comments/:id
func (h *CommentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromContext(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	comment, err := store.GetComment(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if comment == nil {
		writeError(w, http.StatusNotFound, "comment not found")
		return
	}
	if comment.UserID != claims.Sub && claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := store.DeleteComment(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete comment")
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

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
