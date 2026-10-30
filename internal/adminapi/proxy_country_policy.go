package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"net/http"
	"slices"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/geoctx"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
)

func (s *Server) proxyCountryPolicy(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	err := s.DB.QueryRowContext(r.Context(), `SELECT jsonb_build_object(
 'account_id',a.id,'countries',COALESCE(p.countries,'["AUTO"]'::jsonb),'revision',COALESCE(p.revision,0),
 'network_mode',COALESCE(n.mode,'direct'),'active_country',COALESCE(i.country_code,''),
 'active_proxy',COALESCE(px.name,''),'active_stage',COALESCE(s.active_stage,''),
 'applied_revision',COALESCE(s.applied_revision,0),'status',COALESCE(s.status,'legacy'),
 'detail',COALESCE(s.detail,''),'checked_at',s.checked_at,
 'identity_verified',COALESCE(i.last_health_ok,false) AND i.rotation_started_at IS NULL AND n.mode='proxy_required')
 FROM accounts a LEFT JOIN account_proxy_country_policies p ON p.account_id=a.id
 LEFT JOIN account_proxy_selection s ON s.account_id=a.id LEFT JOIN network_profiles n ON n.account_id=a.id
 LEFT JOIN account_network_identities i ON i.account_id=a.id LEFT JOIN proxies px ON px.id=n.proxy_id WHERE a.id=$1`, r.PathValue("id")).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "account_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"policy": json.RawMessage(raw), "country_options": geoctx.CountryOptions(), "strategy_version": 1})
}

func (s *Server) saveProxyCountryPolicy(w http.ResponseWriter, r *http.Request) {
	principal, _ := principal(r.Context())
	if !principal.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var request struct {
		Countries *[]string `json:"countries"`
		Revision  *int64    `json:"revision"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request.Revision == nil || request.Countries == nil || *request.Revision < 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_country_policy_request"})
		return
	}
	countries, err := proxyselection.Priorities("", *request.Countries)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_country_priorities", "detail": err.Error()})
		return
	}
	account, idErr := network.CanonicalAccountID(r.PathValue("id"))
	if idErr != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_account_id"})
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `SET LOCAL lock_timeout='5s'`); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	for _, prefix := range []string{"account-proxy:", "account-route:"} {
		if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, prefix+account); err != nil {
			writeJSON(w, 409, map[string]string{"error": "account_route_busy", "detail": "A request is using this route. Retry saving after it settles."})
			return
		}
	}
	var provider, mode, proxy string
	if err = tx.QueryRowContext(r.Context(), `SELECT a.provider,COALESCE(n.mode,'direct'),COALESCE(n.proxy_id::text,'') FROM accounts a LEFT JOIN network_profiles n ON n.account_id=a.id WHERE a.id=$1 FOR SHARE OF a NOWAIT`, account).Scan(&provider, &mode, &proxy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, 404, map[string]string{"error": "account_not_found"})
		} else {
			writeJSON(w, 409, map[string]string{"error": "account_busy"})
		}
		return
	}
	if !proxyselection.Legacy(countries) && (mode != "proxy_required" || proxy == "") {
		writeJSON(w, 409, map[string]string{"error": "proxy_country_requires_proxy", "detail": "Assign an account proxy before selecting a country."})
		return
	}
	revision := int64(0)
	var oldRaw []byte
	old := []string{proxyselection.Auto}
	err = tx.QueryRowContext(r.Context(), `SELECT countries,revision FROM account_proxy_country_policies WHERE account_id=$1 FOR UPDATE`, account).Scan(&oldRaw, &revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 500, errorBody())
		return
	}
	if len(oldRaw) > 0 && json.Unmarshal(oldRaw, &old) != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if revision != *request.Revision {
		writeJSON(w, 409, map[string]any{"error": "country_policy_conflict", "revision": revision, "countries": old, "detail": "Country priorities changed elsewhere. Reload and review before saving."})
		return
	}
	if !slices.Equal(old, countries) {
		raw, _ := json.Marshal(countries)
		if err = tx.QueryRowContext(r.Context(), `INSERT INTO account_proxy_country_policies(account_id,countries) VALUES($1,$2)
   ON CONFLICT(account_id) DO UPDATE SET countries=EXCLUDED.countries,revision=account_proxy_country_policies.revision+1,updated_at=now() RETURNING revision`, account, raw).Scan(&revision); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if _, err = app.FenceCountryWriterTx(r.Context(), tx, account); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE account_proxy_selection SET work_revision=$2,applied_revision=0,scope_hash='',checkpoint='{}',candidate=NULL,active_stage='',status='pending',detail='Saved; waiting for a verified eligible route',updated_at=now() WHERE account_id=$1`, account, revision); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		// Retain committed identity evidence. A policy edit is not a successful probe.
		if mode == "proxy_required" {
			if _, err = tx.ExecContext(r.Context(), `UPDATE account_network_identities SET last_health_ok=false WHERE account_id=$1`, account); err != nil {
				writeJSON(w, 500, errorBody())
				return
			}
			if proxyselection.Legacy(countries) {
				if _, err = tx.ExecContext(r.Context(), `UPDATE account_network_identities SET selected_target_country=NULL,sticky_session=NULL,exit_ip=NULL,subnet_key=NULL,last_health_at=NULL,rotation_started_at=NULL,fallback_active=false WHERE account_id=$1 AND selected_target_country IS NOT NULL`, account); err != nil {
					writeJSON(w, 500, errorBody())
					return
				}
			}
			if _, err = proxycontrol.BumpTransportEpochTx(r.Context(), tx, account, provider, &proxy, "country-policy-change"); err != nil {
				writeJSON(w, 500, errorBody())
				return
			}
		}
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 503, map[string]string{"error": "country_policy_commit_unconfirmed", "detail": "Read back the policy revision before retrying."})
		return
	}
	writeJSON(w, 200, map[string]any{"saved": true, "countries": countries, "revision": revision})
}

func (s *Server) proxyDrivers(w http.ResponseWriter, r *http.Request) {
	drivers := make([]map[string]any, 0)
	for _, name := range app.ProxyAdapterNames() {
		cap := app.ProxyAdapterCapabilities(name)
		drivers = append(drivers, map[string]any{"name": name, "country_targeting": cap.CountryTargeting, "sticky_session": cap.StickySession, "fixed_egress": cap.FixedEgress, "dedicated_port": cap.DedicatedPort})
	}
	writeJSON(w, 200, map[string]any{"drivers": drivers, "country_options": geoctx.CountryOptions()})
}
