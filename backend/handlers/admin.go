package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	cloudpkg "github.com/quipthread/quipthread/cloud"
	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/models"
	"github.com/quipthread/quipthread/sanitize"
	"github.com/quipthread/quipthread/session"
)

type AdminHandler struct {
	store  db.Store
	config *config.Config
	cloud  cloudpkg.Store // cloud control-plane store; nil in self-hosted mode
}

func NewAdminHandler(store db.Store, cfg *config.Config) *AdminHandler {
	return NewAdminHandlerWithCloud(store, cfg, nil)
}

// NewAdminHandlerWithCloud constructs an AdminHandler with an optional cloud
// control-plane store used for dark-deployment site registry dual writes.
func NewAdminHandlerWithCloud(store db.Store, cfg *config.Config, cs cloudpkg.Store) *AdminHandler {
	return &AdminHandler{store: store, config: cfg, cloud: cs}
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
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	comment, err := store.GetComment(id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	if comment == nil {
		writeError(w, r, http.StatusNotFound, "comment not found")
		return
	}

	if req.Status != "" {
		comment.Status = req.Status
	}
	if req.Content != "" {
		comment.Content = sanitize.CommentHTML(req.Content)
	}

	if err := store.UpdateComment(comment); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to update comment")
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
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, r, http.StatusBadRequest, "content is required")
		return
	}
	req.Content = sanitize.CommentHTML(req.Content)

	parent, err := store.GetComment(parentID)
	if err != nil || parent == nil {
		writeError(w, r, http.StatusNotFound, "parent comment not found")
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
		if writeQuotaError(w, r, err) {
			return
		}
		writeError(w, r, http.StatusInternalServerError, "failed to create reply")
		return
	}

	writeJSON(w, http.StatusCreated, reply)
}

// DELETE /api/admin/comments/:id
func (h *AdminHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	id := chi.URLParam(r, "id")

	if err := store.DeleteComment(id); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to delete comment")
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
		writeError(w, r, http.StatusInternalServerError, "failed to list users")
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
	Role         string `json:"role"`
	Banned       *bool  `json:"banned"`
	ShadowBanned *bool  `json:"shadow_banned"`
}

// PATCH /api/admin/users/:id
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	id := chi.URLParam(r, "id")

	// Prevent admins from demoting or banning themselves.
	if claims, ok := r.Context().Value(session.UserKey).(*session.Claims); ok && claims != nil {
		if claims.Sub == id {
			writeError(w, r, http.StatusForbidden, "cannot modify your own account")
			return
		}
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := store.GetUser(id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, r, http.StatusNotFound, "user not found")
		return
	}

	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Banned != nil {
		user.Banned = *req.Banned
	}
	if req.ShadowBanned != nil {
		user.ShadowBanned = *req.ShadowBanned
	}

	if err := store.UpdateUser(user); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to update user")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// GET /api/admin/sites
func (h *AdminHandler) ListSites(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	sites, err := store.ListSites()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to list sites")
		return
	}
	views := make([]siteView, 0, len(sites))
	for _, s := range sites {
		views = append(views, newSiteView(s))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sites": views})
}

// siteView is the API representation of a Site. The raw SSO secret is excluded
// at the model level (json:"-"); sso_enabled is the only SSO state exposed.
type siteView struct {
	*models.Site
	SSOEnabled bool `json:"sso_enabled"`
}

func newSiteView(site *models.Site) siteView {
	return siteView{Site: site, SSOEnabled: site.SSOSecret != nil}
}

func writeSiteJSON(w http.ResponseWriter, status int, site *models.Site) {
	writeJSON(w, status, newSiteView(site))
}

type updateSiteRequest struct {
	Theme          string `json:"theme"`
	NotifyInterval *int   `json:"notify_interval"`
}

// PATCH /api/admin/sites/:id
func (h *AdminHandler) UpdateSite(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	id := chi.URLParam(r, "id")

	var req updateSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	site, err := store.GetSite(id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	if site == nil {
		writeError(w, r, http.StatusNotFound, "site not found")
		return
	}

	if req.Theme != "" {
		site.Theme = req.Theme
	}

	if req.NotifyInterval != nil {
		if *req.NotifyInterval < db.MinSiteDigestIntervalSeconds || *req.NotifyInterval > db.MaxSiteDigestIntervalSeconds {
			writeError(w, r, http.StatusBadRequest, "notify_interval_out_of_range")
			return
		}
		// notify_interval is Pro+ in cloud mode; ignored in self-hosted/dev builds.
		if h.config.CloudMode {
			sub, err := middleware.GetCachedSubscription(middleware.AccountIDFromRequest(r), store)
			if err != nil || middleware.PlanRank[sub.Plan] < middleware.PlanRank["pro"] {
				writeError(w, r, http.StatusPaymentRequired, "plan_upgrade_required")
				return
			}
		}
		site.NotifyInterval = req.NotifyInterval
	}

	if err := store.UpdateSite(site); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to update site")
		return
	}

	writeSiteJSON(w, http.StatusOK, site)
}

// DELETE /api/admin/sites/:id
func (h *AdminHandler) DeleteSite(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	id := chi.URLParam(r, "id")

	site, err := store.GetSite(id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	if site == nil {
		writeError(w, r, http.StatusNotFound, "site not found")
		return
	}

	claims := claimsFromContext(r)
	if claims == nil || (claims.Role != "admin" && claims.Sub != site.OwnerID) {
		writeError(w, r, http.StatusForbidden, "only owners and admins may delete a site")
		return
	}

	if h.config.CloudMode && h.cloud != nil {
		h.deleteSiteCloud(w, r, store, id)
		return
	}

	if err := store.DeleteSite(id); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to delete site")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// deleteSiteCloud performs the cloud-mode site deletion with registry dual
// writes. Ordering is deliberate:
//
//  1. deactivate the registry entry first, so control-plane consumers stop
//     seeing the site before its tenant data disappears;
//  2. delete the tenant data;
//  3. remove the registry row only after tenant deletion succeeded.
//
// Failure semantics:
//   - deactivation failure (other than a missing legacy entry): tenant data is
//     left untouched and the request fails deterministically;
//   - missing registry entry (sites created before dual writes existed):
//     deletion proceeds; there is nothing to deactivate or remove;
//   - tenant deletion failure: the registry entry stays inactive — ownership
//     is retained and the deletion can be retried;
//   - registry row removal failure after successful tenant deletion: logged,
//     never silent; the row remains behind in the inactive state, which is safe.
func (h *AdminHandler) deleteSiteCloud(w http.ResponseWriter, r *http.Request, store db.Store, siteID string) {
	accountID := middleware.AccountIDFromRequest(r)
	if accountID == "" {
		writeError(w, r, http.StatusInternalServerError, "account resolution failed")
		return
	}

	if err := h.cloud.DeactivateSiteRegistration(siteID, accountID, time.Now().UTC()); err != nil {
		if !errors.Is(err, cloudpkg.ErrSiteRegistryNotFound) {
			writeError(w, r, http.StatusInternalServerError, "failed to deactivate site registration")
			return
		}
		log.Printf("site delete: no registry entry for site %s (legacy site); skipping registry writes", strconv.Quote(siteID))
	}

	if err := store.DeleteSite(siteID); err != nil {
		// Registry entry intentionally left inactive so ownership is retained.
		log.Printf("site delete: tenant deletion failed for site %s (registry left inactive): %s", strconv.Quote(siteID), strconv.Quote(err.Error()))
		writeError(w, r, http.StatusInternalServerError, "failed to delete site")
		return
	}

	if err := h.cloud.DeleteSiteRegistration(siteID, accountID); err != nil {
		// Tenant data is already gone; leaving the inactive row behind is safe
		// but must not be silently ignored.
		log.Printf("site delete: registry cleanup failed for site %s (row left inactive): %s", strconv.Quote(siteID), strconv.Quote(err.Error()))
	}

	w.WriteHeader(http.StatusNoContent)
}

type createSiteRequest struct {
	Domain string `json:"domain"`
}

// POST /api/admin/sites
//
// In cloud mode, site creation dual-writes to the dark-deployment site
// registry with deliberate ordering:
//
//  1. reserve a registry row for a pre-generated site ID,
//  2. create the tenant site using that ID,
//  3. activate the registry row before responding success.
//
// Failure semantics:
//   - reservation conflict or failure: no tenant site is created;
//   - tenant create failure: the registry row is intentionally left in the
//     non-active "reserved" state so the account's claim survives and the
//     creation can be retried or reconciled, instead of silently dropping it;
//   - activation failure: the request fails deterministically even though the
//     tenant site exists; the row stays "reserved" and is recoverable via
//     retry/reconciliation. Registry failures are never silently ignored.
//
// Self-hosted mode is unchanged: no registry writes occur.
func (h *AdminHandler) CreateSite(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	claims := claimsFromContext(r)

	var req createSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Domain == "" {
		writeError(w, r, http.StatusBadRequest, "domain is required")
		return
	}

	if h.config.CloudMode && h.cloud != nil {
		h.createSiteCloud(w, r, store, claims, req.Domain)
		return
	}

	site := &models.Site{
		OwnerID: claims.Sub,
		Domain:  req.Domain,
	}

	if err := store.CreateSite(site); err != nil {
		if writeQuotaError(w, r, err) {
			return
		}
		writeError(w, r, http.StatusInternalServerError, "failed to create site")
		return
	}

	writeSiteJSON(w, http.StatusCreated, site)
}

func (h *AdminHandler) createSiteCloud(w http.ResponseWriter, r *http.Request, store db.Store, claims *session.Claims, domain string) {
	accountID := middleware.AccountIDFromRequest(r)
	if accountID == "" {
		writeError(w, r, http.StatusInternalServerError, "account resolution failed")
		return
	}

	site := &models.Site{
		ID:      uuid.NewString(),
		OwnerID: claims.Sub,
		Domain:  domain,
	}

	if err := h.cloud.ReserveSiteRegistration(site.ID, accountID, time.Now().UTC()); err != nil {
		if errors.Is(err, cloudpkg.ErrSiteRegistryConflict) {
			writeError(w, r, http.StatusConflict, "site already registered")
			return
		}
		log.Printf("site create: reservation failed for site %s (no tenant site created): %v", site.ID, err)
		writeError(w, r, http.StatusInternalServerError, "failed to reserve site registration")
		return
	}

	if err := store.CreateSite(site); err != nil {
		if writeQuotaError(w, r, err) {
			return
		}
		// Registry row deliberately left reserved so the claim survives for
		// retry/reconciliation rather than being silently dropped.
		log.Printf("site create: tenant insert failed for site %s (reservation kept): %v", site.ID, err)
		writeError(w, r, http.StatusInternalServerError, "failed to create site")
		return
	}

	if err := h.cloud.ActivateSiteRegistration(site.ID, accountID, time.Now().UTC()); err != nil {
		// Tenant site exists but registration is incomplete: fail loudly rather
		// than silently ignoring the registry write. The row remains reserved.
		log.Printf("site create: activation failed for site %s (row left reserved): %v", site.ID, err)
		writeError(w, r, http.StatusInternalServerError, "site created but registration incomplete")
		return
	}

	writeSiteJSON(w, http.StatusCreated, site)
}
