package adminapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
)

func (s *Server) saveIPPolicy(w http.ResponseWriter, r *http.Request) {
	account := r.PathValue("id")
	family, e := strconv.Atoi(r.PathValue("family"))
	if e != nil || (family != 4 && family != 6) || !accountOutputID.MatchString(account) {
		writeJSON(w, 400, map[string]string{"error": "invalid_ip_policy_scope"})
		return
	}
	var body struct {
		Revision int64       `json:"revision"`
		Policy   ipam.Policy `json:"policy"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&body); e != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_ip_policy"})
		return
	}
	var extra any
	if e = dec.Decode(&extra); e != io.EOF || body.Policy.Family != family {
		writeJSON(w, 400, map[string]string{"error": "invalid_ip_policy"})
		return
	}
	saved, e := (ipam.Store{DB: s.DB}).SavePolicy(r.Context(), account, body.Revision, body.Policy)
	switch {
	case errors.Is(e, ipam.ErrPolicy):
		writeJSON(w, 400, map[string]string{"error": "ip_policy_incomplete_or_invalid"})
	case errors.Is(e, ipam.ErrRevision):
		writeJSON(w, 409, map[string]string{"error": "ip_policy_revision_changed"})
	case errors.Is(e, ipam.ErrProtected):
		writeJSON(w, 409, map[string]string{"error": "account_disabled_or_deleting"})
	case e != nil:
		writeJSON(w, 503, map[string]string{"error": "ip_policy_not_saved"})
	default:
		writeJSON(w, 200, map[string]any{"saved": saved, "provider_execution_enabled": false, "message": "Settings saved; no allocation or network authority granted."})
	}
}

func (s *Server) saveIPFunding(w http.ResponseWriter, r *http.Request) {
	account := r.PathValue("id")
	if !accountOutputID.MatchString(account) {
		writeJSON(w, 400, map[string]string{"error": "invalid_account_id"})
		return
	}
	var body struct {
		Revision      int64     `json:"revision"`
		Currency      string    `json:"currency"`
		BudgetMicros  int64     `json:"budget_micros"`
		FundedUntil   time.Time `json:"funded_until"`
		MaxConcurrent int       `json:"max_concurrent"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&body); e != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_ip_funding"})
		return
	}
	var extra any
	if e := dec.Decode(&extra); e != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_ip_funding"})
		return
	}
	f, e := (ipam.Store{DB: s.DB}).SaveFunding(r.Context(), ipam.Funding{AccountID: account, Currency: body.Currency, BudgetMicros: body.BudgetMicros, FundedUntil: body.FundedUntil, MaxConcurrent: body.MaxConcurrent}, body.Revision)
	switch {
	case errors.Is(e, ipam.ErrPolicy):
		writeJSON(w, 400, map[string]string{"error": "ip_funding_incomplete_or_invalid"})
	case errors.Is(e, ipam.ErrRevision):
		writeJSON(w, 409, map[string]string{"error": "ip_funding_revision_changed"})
	case errors.Is(e, ipam.ErrProtected):
		writeJSON(w, 409, map[string]string{"error": "ip_funding_scope_blocked"})
	case e != nil:
		writeJSON(w, 503, map[string]string{"error": "ip_funding_not_saved"})
	default:
		writeJSON(w, 200, map[string]any{"saved": f, "provider_execution_enabled": false})
	}
}
