package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/middleware"
	"github.com/quipthread/quipthread/models"
)

const maxNormalJSONBodyBytes int64 = middleware.MaxNormalBodyBytes

var errModRulesBodyTooLarge = errors.New("modrules request body too large")

func decodeModRulesJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxNormalJSONBodyBytes) //nolint:gosec // G120: admin JSON request limit
	if r.ContentLength > maxNormalJSONBodyBytes {
		return errModRulesBodyTooLarge
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	// Consume the bounded body so a valid JSON value followed by an oversized
	// suffix cannot bypass the request limit. Only whitespace is accepted after
	// the first value.
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return err
	}
	return nil
}

func modRulesBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.Is(err, errModRulesBodyTooLarge) || errors.As(err, &maxErr)
}

type ModRulesHandler struct {
	store db.Store
	cfg   *config.Config
}

func NewModRulesHandler(store db.Store, cfg *config.Config) *ModRulesHandler {
	return &ModRulesHandler{store: store, cfg: cfg}
}

func (h *ModRulesHandler) db(r *http.Request) db.Store {
	if s, ok := db.StoreFromContext(r.Context()); ok {
		return s
	}
	return h.store
}

// GET /api/admin/modrules/blocklist
func (h *ModRulesHandler) List(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	if !h.planCheck(w, r, store) {
		return
	}
	terms, err := store.ListBlockedTerms()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to list blocked terms")
		return
	}
	if terms == nil {
		terms = []*models.BlockedTerm{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"terms": terms})
}

// POST /api/admin/modrules/blocklist  body: {"term": "...", "is_regex": false}
func (h *ModRulesHandler) Add(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	if !h.planCheck(w, r, store) {
		return
	}
	var body struct {
		Term    string `json:"term"`
		IsRegex bool   `json:"is_regex"`
	}
	if err := decodeModRulesJSON(w, r, &body); err != nil {
		if modRulesBodyTooLarge(err) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, r, http.StatusBadRequest, "term is required")
		return
	}
	if strings.TrimSpace(body.Term) == "" {
		writeError(w, r, http.StatusBadRequest, "term is required")
		return
	}
	if body.IsRegex && !h.cfg.CloudMode {
		writeError(w, r, http.StatusForbidden, "regex_blocklist_cloud_only")
		return
	}
	term := strings.TrimSpace(body.Term)
	if !body.IsRegex {
		term = strings.ToLower(term)
	}
	if body.IsRegex {
		if _, err := regexp.Compile(term); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_regex: "+err.Error())
			return
		}
	}
	t, err := store.AddBlockedTerm(term, body.IsRegex)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to add term")
		return
	}
	middleware.InvalidateBlockedTermsCache()
	writeJSON(w, http.StatusCreated, t)
}

// DELETE /api/admin/modrules/blocklist/{id}
func (h *ModRulesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	if !h.planCheck(w, r, store) {
		return
	}
	id := chi.URLParam(r, "id")
	if err := store.DeleteBlockedTerm(id); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to delete term")
		return
	}
	middleware.InvalidateBlockedTermsCache()
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/admin/modrules/blocklist/import  body: {"url": "..."}
func (h *ModRulesHandler) Import(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	if !h.planCheck(w, r, store) {
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeModRulesJSON(w, r, &body); err != nil {
		if modRulesBodyTooLarge(err) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, r, http.StatusBadRequest, "url is required")
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		writeError(w, r, http.StatusBadRequest, "url is required")
		return
	}

	raw, err := fetchUntrustedBlocklistURL(r.Context(), strings.TrimSpace(body.URL))
	if err != nil {
		if errors.Is(err, errInvalidRemoteURL) {
			writeError(w, r, http.StatusBadRequest, "invalid URL")
			return
		}
		writeError(w, r, http.StatusBadGateway, "failed to fetch URL")
		return
	}

	var terms []string
	seen := make(map[string]struct{})
	for _, lineB := range bytes.Split(raw, []byte("\n")) {
		t := strings.ToLower(strings.TrimSpace(string(lineB)))
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		terms = append(terms, t)
	}

	if len(terms) == 0 {
		writeJSON(w, http.StatusOK, map[string]int{"added": 0, "skipped": 0})
		return
	}

	added, err := store.BulkAddBlockedTerms(terms)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to import terms")
		return
	}
	middleware.InvalidateBlockedTermsCache()
	writeJSON(w, http.StatusOK, map[string]int{"added": added, "skipped": len(terms) - added})
}

// planCheck enforces Pro+ in cloud mode; returns true if the request may proceed.
func (h *ModRulesHandler) planCheck(w http.ResponseWriter, r *http.Request, store db.Store) bool {
	if !h.cfg.CloudMode {
		return true
	}
	sub, err := middleware.GetCachedSubscription(middleware.AccountIDFromRequest(r), store)
	if err != nil || middleware.PlanRank[sub.Plan] < middleware.PlanRank["pro"] {
		writeError(w, r, http.StatusPaymentRequired, "plan_upgrade_required")
		return false
	}
	return true
}
