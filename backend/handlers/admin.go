package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/models"
)

type AdminHandler struct {
	store  db.Store
	config *config.Config
}

func NewAdminHandler(store db.Store, cfg *config.Config) *AdminHandler {
	return &AdminHandler{store: store, config: cfg}
}

func (h *AdminHandler) db(r *http.Request) db.Store {
	if s, ok := db.StoreFromContext(r.Context()); ok {
		return s
	}
	return h.store
}

// GET /api/admin/comments?status=pending&page=1&siteId=
func (h *AdminHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	siteID := r.URL.Query().Get("siteId")
	status := r.URL.Query().Get("status")
	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 20)

	comments, total, err := store.ListAdminComments(siteID, status, page, limit)
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

type updateCommentRequest struct {
	Status  string `json:"status"`
	Content string `json:"content"`
}

// PATCH /api/admin/comments/:id
func (h *AdminHandler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	id := chi.URLParam(r, "id")

	var req updateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	comment, err := store.GetComment(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if comment == nil {
		writeError(w, http.StatusNotFound, "comment not found")
		return
	}

	if req.Status != "" {
		comment.Status = req.Status
	}
	if req.Content != "" {
		comment.Content = req.Content
	}

	if err := store.UpdateComment(comment); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update comment")
		return
	}

	writeJSON(w, http.StatusOK, comment)
}

type replyRequest struct {
	Content string `json:"content"`
}

// POST /api/admin/comments/:id/reply
func (h *AdminHandler) Reply(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	parentID := chi.URLParam(r, "id")
	claims := claimsFromContext(r)

	var req replyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	parent, err := store.GetComment(parentID)
	if err != nil || parent == nil {
		writeError(w, http.StatusNotFound, "parent comment not found")
		return
	}

	reply := &models.Comment{
		SiteID:    parent.SiteID,
		PageID:    parent.PageID,
		PageURL:   parent.PageURL,
		PageTitle: parent.PageTitle,
		ParentID:  parentID,
		UserID:    claims.Sub,
		Content:   req.Content,
		Status:    "approved",
	}

	if err := store.CreateComment(reply); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create reply")
		return
	}

	writeJSON(w, http.StatusCreated, reply)
}

// DELETE /api/admin/comments/:id
func (h *AdminHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	id := chi.URLParam(r, "id")

	if err := store.DeleteComment(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete comment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /api/admin/users?page=1
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	page := queryInt(r, "page", 1)
	limit := queryInt(r, "limit", 20)

	users, total, err := store.ListUsers(page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users": users,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

type updateUserRequest struct {
	Role   string `json:"role"`
	Banned *bool  `json:"banned"`
}

// PATCH /api/admin/users/:id
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	id := chi.URLParam(r, "id")

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := store.GetUser(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Banned != nil {
		user.Banned = *req.Banned
	}

	if err := store.UpdateUser(user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// GET /api/admin/sites
func (h *AdminHandler) ListSites(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	sites, err := store.ListSites()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sites")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sites": sites})
}

type updateSiteRequest struct {
	Theme string `json:"theme"`
}

// PATCH /api/admin/sites/:id
func (h *AdminHandler) UpdateSite(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	id := chi.URLParam(r, "id")

	var req updateSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	site, err := store.GetSite(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	if req.Theme != "" {
		site.Theme = req.Theme
	}

	if err := store.UpdateSite(site); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update site")
		return
	}

	writeJSON(w, http.StatusOK, site)
}

type createSiteRequest struct {
	Domain string `json:"domain"`
}

// POST /api/admin/sites
func (h *AdminHandler) CreateSite(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromContext(r)

	var req createSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	site := &models.Site{
		OwnerID: claims.Sub,
		Domain:  req.Domain,
	}

	if err := store.CreateSite(site); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create site")
		return
	}

	writeJSON(w, http.StatusCreated, site)
}
