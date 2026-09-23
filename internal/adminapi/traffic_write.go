package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/trafficguard"
	"net/http"
)

type trafficPolicyWrite struct {
	AccountID string              `json:"account_id"`
	Policy    trafficguard.Policy `json:"policy"`
}

func (s *Server) upsertTrafficPolicy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanWrite() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x trafficPolicyWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.AccountID == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	a := x.Policy.Action
	if a != trafficguard.ActionLog && a != trafficguard.ActionAlert && a != trafficguard.ActionDisable {
		writeJSON(w, 400, map[string]string{"error": "invalid_action"})
		return
	}
	_, err := s.DB.ExecContext(r.Context(), `INSERT INTO traffic_policies(id,account_id,min_samples,alpha,sigma_multiplier,min_bps,hard_bps,confirmations,action) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(account_id) DO UPDATE SET min_samples=EXCLUDED.min_samples,alpha=EXCLUDED.alpha,sigma_multiplier=EXCLUDED.sigma_multiplier,min_bps=EXCLUDED.min_bps,hard_bps=EXCLUDED.hard_bps,confirmations=EXCLUDED.confirmations,action=EXCLUDED.action,updated_at=now()`, x.AccountID, x.Policy.MinSamples, x.Policy.Alpha, x.Policy.SigmaMultiplier, x.Policy.MinBytesPerSec, x.Policy.HardBytesPerSec, x.Policy.Confirmations, a)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	w.WriteHeader(204)
}
