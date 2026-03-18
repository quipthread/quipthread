package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/quipthread/quipthread/db"
	"github.com/quipthread/quipthread/importer"
	"github.com/quipthread/quipthread/models"
)

// importFromReader is a shared helper for text-based importers (XML/JSON).
// It reads a multipart form with "siteId" and "file" fields, parses the file
// using the provided function, upserts synthetic users, and bulk-imports comments.
func (h *AdminHandler) importFromReader(
	w http.ResponseWriter,
	r *http.Request,
	parse func(io.Reader) (*importer.Result, error),
) {
	store := h.db(r)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	siteID := r.FormValue("siteId")
	if siteID == "" {
		writeError(w, http.StatusBadRequest, "siteId is required")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	result, err := parse(file)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "parse error: "+err.Error())
		return
	}

	usersInserted := h.upsertImportedUsers(store, result.Users)

	commentsInserted, err := store.ImportComments(siteID, result.Comments)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "import failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users_inserted":    usersInserted,
		"comments_inserted": commentsInserted,
	})
}

// saveTempFile copies the "file" multipart field to a temporary file on disk.
// The caller is responsible for removing the file with os.Remove.
func saveTempFile(r *http.Request) (string, error) {
	file, _, err := r.FormFile("file")
	if err != nil {
		return "", err
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "qt-import-*.db")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// upsertImportedUsers upserts each synthetic user from an import result and
// returns the number of successful upserts.
func (h *AdminHandler) upsertImportedUsers(store db.Store, users []*models.User) int {
	n := 0
	for _, u := range users {
		if err := store.UpsertUser(u); err == nil {
			n++
		}
	}
	return n
}

// POST /api/admin/import/disqus
func (h *AdminHandler) ImportDisqus(w http.ResponseWriter, r *http.Request) {
	h.importFromReader(w, r, importer.ParseDisqus)
}

// POST /api/admin/import/wordpress
func (h *AdminHandler) ImportWordPress(w http.ResponseWriter, r *http.Request) {
	h.importFromReader(w, r, importer.ParseWordPress)
}

// POST /api/admin/import/remark42
func (h *AdminHandler) ImportRemark42(w http.ResponseWriter, r *http.Request) {
	h.importFromReader(w, r, importer.ParseRemark42)
}

// POST /api/admin/import/native
func (h *AdminHandler) ImportNative(w http.ResponseWriter, r *http.Request) {
	h.importFromReader(w, r, importer.ParseNative)
}

// POST /api/admin/import/quipthread
// Accepts a Quipthread SQLite database file and imports all users + comments.
func (h *AdminHandler) ImportQuipthreadDB(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)

	if err := r.ParseMultipartForm(128 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	siteID := r.FormValue("siteId")
	if siteID == "" {
		writeError(w, http.StatusBadRequest, "siteId is required")
		return
	}

	path, err := saveTempFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer os.Remove(path)

	result, err := importer.ImportQuipthreadDB(path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "import error: "+err.Error())
		return
	}

	usersInserted := h.upsertImportedUsers(store, result.Users)

	commentsInserted, err := store.ImportComments(siteID, result.Comments)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "import failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users_inserted":    usersInserted,
		"comments_inserted": commentsInserted,
	})
}

// POST /api/admin/import/sqlite/inspect
// Accepts a SQLite file and returns the schema of every table with sample values.
func (h *AdminHandler) ImportSQLiteInspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	path, err := saveTempFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer os.Remove(path)

	tables, err := importer.InspectSQLite(path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "inspect error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"tables": tables})
}

// POST /api/admin/import/sqlite/run
// Accepts a SQLite file + mapping JSON (form field "mapping") and imports
// comments using the column mapping the user configured in the UI.
func (h *AdminHandler) ImportSQLiteRun(w http.ResponseWriter, r *http.Request) {
	store := h.db(r)

	if err := r.ParseMultipartForm(128 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	siteID := r.FormValue("siteId")
	if siteID == "" {
		writeError(w, http.StatusBadRequest, "siteId is required")
		return
	}

	mappingJSON := r.FormValue("mapping")
	if mappingJSON == "" {
		writeError(w, http.StatusBadRequest, "mapping is required")
		return
	}

	var mapping importer.ColumnMapping
	if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
		writeError(w, http.StatusBadRequest, "invalid mapping JSON: "+err.Error())
		return
	}

	if mapping.Table == "" {
		writeError(w, http.StatusBadRequest, "mapping.table is required")
		return
	}

	path, err := saveTempFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer os.Remove(path)

	// Build the allowedCols whitelist from the actual schema before running.
	tables, err := importer.InspectSQLite(path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "inspect error: "+err.Error())
		return
	}
	allowedCols := make(map[string]bool)
	for _, t := range tables {
		if t.Name == mapping.Table {
			for _, col := range t.Columns {
				allowedCols[col.Name] = true
			}
			break
		}
	}

	result, err := importer.ImportMappedSQLite(path, mapping, allowedCols)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "import error: "+err.Error())
		return
	}

	usersInserted := h.upsertImportedUsers(store, result.Users)

	commentsInserted, err := store.ImportComments(siteID, result.Comments)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "import failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users_inserted":    usersInserted,
		"comments_inserted": commentsInserted,
	})
}
