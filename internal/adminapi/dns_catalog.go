package adminapi

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/resolverpolicy"
	"net/http"
)

func (s *Server) getDNSCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(resolverpolicy.Catalog)
}
