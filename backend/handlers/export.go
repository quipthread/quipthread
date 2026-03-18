package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/quipthread/quipthread/config"
	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/exporter"
	"github.com/quipthread/quipthread/middleware"
)

type ExportHandler struct {
	store db.Store
	cfg   *config.Config
}

func NewExportHandler(store db.Store, cfg *config.Config) *ExportHandler {
	return &ExportHandler{store: store, cfg: cfg}
}

func (h *ExportHandler) db(r *http.Request) db.Store {
	if s, ok := db.StoreFromContext(r.Context()); ok {
		return s
	}
	return h.store
}

func (h *ExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)
	q := r.URL.Query()

	siteID := q.Get("siteId")
	if siteID == "" {
		writeError(w, http.StatusBadRequest, "siteId is required")
		return
	}

	format := q.Get("format")
	if format == "" {
		format = "native"
	}
	if format != "native" && format != "csv" {
		writeError(w, http.StatusBadRequest, "format must be native or csv")
		return
	}

	status := q.Get("status")
	if status == "" {
		status = "approved"
	}

	filter := db.ExportFilter{Status: status, PageID: q.Get("pageId")}

	if raw := q.Get("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339")
			return
		}
		filter.From = &t
	}
	if raw := q.Get("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339")
			return
		}
		filter.To = &t
	}

	// CSV is Starter+ in cloud mode.
	if format == "csv" && h.cfg.CloudMode {
		sub, err := middleware.GetCachedSubscription(middleware.AccountIDFromRequest(r), store)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load subscription")
			return
		}
		if middleware.PlanRank[sub.Plan] < middleware.PlanRank["starter"] {
			writeError(w, http.StatusPaymentRequired, "plan_upgrade_required")
			return
		}
	}

	site, err := store.GetSite(siteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	comments, err := store.ExportComments(siteID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export failed")
		return
	}
	if comments == nil {
		comments = nil // keep nil; exporters handle empty slices fine
	}

	date := time.Now().UTC().Format("2006-01-02")

	switch format {
	case "csv":
		filename := fmt.Sprintf("quipthread-export-%s-%s.csv", siteID, date)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		if err := exporter.WriteCSV(w, comments); err != nil {
			// Headers already sent; nothing useful we can do.
			return
		}
	default:
		filename := fmt.Sprintf("quipthread-export-%s-%s.json", siteID, date)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		if err := exporter.WriteNative(w, comments, site); err != nil {
			return
		}
	}
}
