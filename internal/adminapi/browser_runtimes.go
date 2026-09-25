package adminapi

import (
	"encoding/json"
	"net/http"
	"os"
)

func (s *Server) browserRuntimes(
	w http.ResponseWriter,
	r *http.Request,
) {
	const file = "/var/lib/digital-ocean-bot/browser-runtimes.json"

	data, err := os.ReadFile(file)
	if err != nil {
		writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{
				"error": "browser_runtime_registry_unavailable",
			},
		)
		return
	}

	var runtimes []map[string]any

	if err := json.Unmarshal(data, &runtimes); err != nil {
		writeJSON(
			w,
			http.StatusInternalServerError,
			map[string]string{
				"error": "invalid_browser_runtime_registry",
			},
		)
		return
	}

	available := 0

	for _, x := range runtimes {
		if ok, _ := x["available"].(bool); ok {
			available++
		}
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"total":     len(runtimes),
			"available": available,
			"runtimes":  runtimes,
		},
	)
}
