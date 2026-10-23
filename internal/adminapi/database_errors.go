package adminapi

import (
	"errors"
	"github.com/lib/pq"
	"net/http"
)

// Database corruption is a storage incident, never a provider credential error.
func writeDatabaseReadError(w http.ResponseWriter, err error, fallback string) {
	var pg *pq.Error
	if errors.As(err, &pg) && (pg.Code == "XX001" || pg.Code == "XX002") {
		w.Header().Set("Retry-After", "30")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database_recovery_required"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fallback})
}
