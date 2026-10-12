package adminapi

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/advancedreality"
)

// Routes reuse existing authentication and admin authorization. Environment
// flag affects observation only; no environment variable enables fleet writes.
func (s *Server) advancedRealityAccess(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if !residentialAdmin(w, r) {
		return false
	}
	return true
}
func (s *Server) advancedRealityStatus(w http.ResponseWriter, r *http.Request) {
	if !s.advancedRealityAccess(w, r) {
		return
	}
	if os.Getenv("DOB_ADVANCED_REALITY_OBSERVE") != "1" {
		writeJSON(w, 200, map[string]any{"enabled": false, "mode": "DISABLED", "version": advancedreality.Version, "can_apply": false, "panels": []any{}, "capabilities": advancedreality.Capabilities(), "unmeasured": advancedreality.Unmeasured()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	b, e := advancedreality.Inventory(ctx, s.DB)
	if e != nil {
		writeJSON(w, 503, map[string]string{"error": "inventory_unavailable"})
		return
	}
	var status map[string]any
	if json.Unmarshal(b, &status) != nil {
		writeJSON(w, 503, errorBody())
		return
	}
	status["capabilities"] = advancedreality.Capabilities()
	status["unmeasured"] = advancedreality.Unmeasured()
	writeJSON(w, 200, status)
}
func (s *Server) advancedRealityValidate(w http.ResponseWriter, r *http.Request) {
	if !s.advancedRealityAccess(w, r) {
		return
	}
	if os.Getenv("DOB_ADVANCED_REALITY_OBSERVE") != "1" {
		writeJSON(w, 409, map[string]string{"error": "experiment_disabled"})
		return
	}
	// A full candidate may contain private keys. Trust actual TLS or loopback
	// transport, never a client-supplied X-Forwarded-Proto header.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if r.TLS == nil && (ip == nil || !ip.IsLoopback()) {
		writeJSON(w, 400, map[string]string{"error": "tls_or_loopback_required"})
		return
	}
	var q struct {
		Baseline  json.RawMessage `json:"baseline"`
		Candidate json.RawMessage `json:"candidate"`
		Core      string          `json:"core_version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2*advancedreality.MaxConfigBytes+4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&q) != nil || d.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	p, e := advancedreality.Inspect(q.Baseline, q.Candidate, q.Core)
	if e != nil {
		writeJSON(w, 422, map[string]string{"error": e.Error()})
		return
	}
	writeJSON(w, 200, p)
}
