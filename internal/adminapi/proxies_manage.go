package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

func (s *Server) updateProxy(w http.ResponseWriter, r *http.Request) {
	principal, _ := principal(r.Context())
	if !principal.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	var old proxyWrite
	var secretRef, secretVersion string
	err := s.DB.QueryRowContext(r.Context(), `SELECT name,type,host,port,COALESCE(username,''),adapter,settings_revision,COALESCE(secret_ref,''),COALESCE((SELECT sec.xmin::text||':'||sec.updated_at::text FROM secrets sec WHERE sec.proxy_id=proxies.id AND sec.id=proxies.secret_ref),'') FROM proxies WHERE id=$1`, id).Scan(&old.Name, &old.Type, &old.Host, &old.Port, &old.Username, &old.Adapter, &old.Revision, &secretRef, &secretVersion)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, errorBody())
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	x := old
	x.Revision = 0 // Never substitute the server revision for an older client draft.
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&x) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if x.Revision < 1 {
		writeJSON(w, 409, map[string]string{"error": "proxy_settings_revision_required", "detail": "Reload the proxy settings and review your draft before saving."})
		return
	}
	check := x
	if x.Adapter == old.Adapter {
		check.Adapter = "generic"
	}
	if !validProxyWrite(check) {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	changed := x.Type != old.Type || x.Host != old.Host || x.Port != old.Port || x.Username != old.Username || x.Adapter != old.Adapter || x.Password != ""
	typ := network.ProxyType(x.Type)
	if changed {
		if !app.ProxyAdapterRegistered(x.Adapter) {
			writeJSON(w, 409, map[string]string{"error": "proxy_driver_unavailable"})
			return
		}
		probe := x
		if probe.Password == "" && secretRef != "" {
			secret, e := s.Container.Secrets.GetProxyAtVersion(r.Context(), id, secretRef, secretVersion)
			if e != nil {
				writeJSON(w, 409, map[string]string{"error": "proxy_credentials_changed"})
				return
			}
			probe.Password = string(secret)
			defer zeroBytes(secret)
		}
		typ, err = resolveProxyType(r, probe)
		if err != nil {
			writeJSON(w, 422, map[string]string{"error": "proxy_detection_failed"})
			return
		}
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
	rows, err := tx.QueryContext(r.Context(), `SELECT account_id::text FROM network_profiles WHERE proxy_id=$1 UNION SELECT account_id::text FROM account_proxy_pool WHERE proxy_id=$1 ORDER BY 1`, id)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	ids := []string{}
	for rows.Next() {
		var a string
		if err = rows.Scan(&a); err != nil {
			rows.Close()
			writeJSON(w, 500, errorBody())
			return
		}
		ids = append(ids, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	for _, a := range ids {
		for _, prefix := range []string{"account-proxy:", "account-route:"} {
			if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, prefix+a); err != nil {
				writeJSON(w, 409, map[string]string{"error": "proxy_route_busy"})
				return
			}
		}
	}
	var revision int64
	if err = tx.QueryRowContext(r.Context(), `SELECT settings_revision FROM proxies WHERE id=$1 FOR UPDATE`, id).Scan(&revision); err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	var currentSecretVersion string
	if secretRef != "" {
		if err = tx.QueryRowContext(r.Context(), `SELECT xmin::text||':'||updated_at::text FROM secrets WHERE proxy_id=$1 AND id=$2 FOR SHARE NOWAIT`, id, secretRef).Scan(&currentSecretVersion); err != nil {
			writeJSON(w, 409, map[string]string{"error": "proxy_credentials_changed"})
			return
		}
	}
	if currentSecretVersion != secretVersion {
		writeJSON(w, 409, map[string]string{"error": "proxy_credentials_changed"})
		return
	}
	if revision != x.Revision {
		writeJSON(w, 409, map[string]string{"error": "proxy_settings_conflict", "detail": "Proxy settings changed elsewhere. Reload and review before saving."})
		return
	}
	var assignmentsChanged bool
	if err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM network_profiles WHERE proxy_id=$1 AND NOT(account_id::text=ANY($2::text[])) UNION ALL SELECT 1 FROM account_proxy_pool WHERE proxy_id=$1 AND NOT(account_id::text=ANY($2::text[])))`, id, pq.Array(ids)).Scan(&assignmentsChanged); err != nil || assignmentsChanged {
		writeJSON(w, 409, map[string]string{"error": "proxy_assignments_changed"})
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE proxies SET name=$2,type=$3,host=$4,port=$5,username=NULLIF($6,''),adapter=$7,settings_revision=settings_revision+1,updated_at=now() WHERE id=$1`, id, x.Name, string(typ), x.Host, x.Port, x.Username, x.Adapter); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if x.Password != "" {
		if err = s.Container.Secrets.PutProxyTx(r.Context(), tx, id, "proxy-password", "proxy_password", []byte(x.Password)); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE proxies SET secret_ref='proxy-password' WHERE id=$1`, id); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if changed {
		if _, err = tx.ExecContext(r.Context(), `UPDATE proxies SET status='healthy',exit_ip=NULL,country=NULL,country_code=NULL,asn=NULL,latency_ms=NULL,health_error=NULL,failure_count=0,consecutive_successes=0,last_checked_at=now(),last_success_at=now() WHERE id=$1`, id); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		for _, a := range ids {
			if _, err = app.FenceCountryWriterTx(r.Context(), tx, a); err != nil {
				writeJSON(w, 500, errorBody())
				return
			}
			if _, err = tx.ExecContext(r.Context(), `UPDATE account_network_identities SET selected_target_country=NULL,sticky_session=NULL,exit_ip=NULL,subnet_key=NULL,asn=NULL,country=NULL,country_code=NULL,last_health_ok=false,last_health_at=NULL,rotation_started_at=NULL,fallback_active=false,updated_at=now() WHERE account_id=$1 AND EXISTS(SELECT 1 FROM network_profiles WHERE account_id=$1 AND proxy_id=$2)`, a, id); err != nil {
				writeJSON(w, 500, errorBody())
				return
			}
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE account_transport_state SET transport_epoch=transport_epoch+1,transition_reason='proxy-configuration-change',updated_at=now() WHERE account_id::text=ANY($1::text[])`, pq.Array(ids)); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 503, map[string]string{"error": "proxy_save_unconfirmed", "detail": "Read back the proxy revision before retrying."})
		return
	}
	writeJSON(w, 200, map[string]any{"type": string(typ), "settings_revision": revision + 1})
}
func (s *Server) deleteProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	if _, err := s.removeProxy(r.Context(), id, false); err != nil {
		writeJSON(w, 409, map[string]string{"error": "proxy_delete_pending", "detail": "Proxy assignments changed or a request is still in flight; refresh and retry."})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
