package handlers

import (
	"net/http"

	"github.com/quipthread/quipthread/db"
)

func writeQuotaError(w http.ResponseWriter, r *http.Request, err error) bool {
	code := db.QuotaErrorCode(err)
	if code == "" {
		return false
	}
	writeError(w, r, http.StatusPaymentRequired, code)
	return true
}
