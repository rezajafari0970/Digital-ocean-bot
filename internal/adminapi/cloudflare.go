package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) cloudflareService() cloudflare.Service {
	return cloudflare.Service{DB: s.DB, Secrets: s.Container.Secrets}
}
func (s *Server) cloudflareStatus(w http.ResponseWriter, r *http.Request) {
	where, _ := outputPublicationFilter("ALL")
	available := `SELECT DISTINCT o.cloudflare_record_id AS id FROM output_config_snapshots o JOIN panel_instances p ON p.id=o.panel_id JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=dr.account_id JOIN deployments d ON d.droplet_id=dr.id CROSS JOIN residential_routing_control rc LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id LEFT JOIN panel_client_routes cr ON cr.panel_id=p.id AND cr.client_id=o.client_id ` + where + ` AND o.cloudflare_record_id IS NOT NULL`
	rows, e := s.DB.QueryContext(r.Context(), "WITH probe_ready AS MATERIALIZED ("+available+") "+`SELECT jsonb_build_object('id',a.id,'name',a.name,'enabled',a.enabled,'revision',a.revision,'manage_ssl',a.manage_ssl,'error_code',a.error_code,'refreshed_at',a.refreshed_at,
 'zones',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',z.id,'name',z.name,'status',z.status,'enabled',z.enabled,'present',z.present,'ssl_strict',z.ssl_strict,'dns_checked_at',z.dns_checked_at,'dns_error_code',z.dns_error_code) ORDER BY z.name) FROM cloudflare_zones z WHERE z.account_id=a.id),'[]'::jsonb),
 'records',COALESCE((SELECT jsonb_agg(t ORDER BY t.expires_at) FROM (SELECT r.id,r.hostname,r.profile_id,r.panel_id,r.state,r.expires_at,r.verified_at,r.error_code,EXISTS(SELECT 1 FROM probe_ready pr WHERE pr.id=r.id) AS probe_ready FROM cloudflare_records r WHERE r.account_id=a.id AND r.state<>'DELETED' ORDER BY r.expires_at LIMIT 500)t),'[]'::jsonb),
 'record_count',(SELECT count(*) FROM cloudflare_records r WHERE r.account_id=a.id AND r.state<>'DELETED'),
 'dns_record_count',(SELECT count(*) FROM cloudflare_records r WHERE r.account_id=a.id AND r.state<>'DELETED' AND r.provider_id<>''),
 'pending_record_count',(SELECT count(*) FROM cloudflare_records r WHERE r.account_id=a.id AND r.state IN('PREPARED','SENT')),
 'probe_ready_count',(SELECT count(*) FROM cloudflare_records r JOIN probe_ready pr ON pr.id=r.id WHERE r.account_id=a.id))
 FROM cloudflare_accounts a ORDER BY a.created_at`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	items := []json.RawMessage{}
	for rows.Next() {
		var b json.RawMessage
		if rows.Scan(&b) != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		items = append(items, b)
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"accounts": items, "transport": "ws/tls", "ip_source": "https://www.cloudflare.com/ips/", "operator_measurements_required": true})
}
func (s *Server) saveCloudflareAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string           `json:"id"`
		Name      string           `json:"name"`
		Revision  int64            `json:"revision"`
		Enabled   bool             `json:"enabled"`
		ManageSSL bool             `json:"manage_ssl"`
		Auth      *cloudflare.Auth `json:"auth,omitempty"`
		Zones     []string         `json:"zone_ids"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF || !cloudflare.ValidID(body.ID) || strings.TrimSpace(body.Name) == "" || len(body.Name) > 100 || len(body.Zones) > 100 || body.Revision < 0 {
		writeJSON(w, 400, map[string]string{"error": "cloudflare_request_invalid"})
		return
	}
	create := r.Method == "POST"
	if create && (body.Revision != 0 || body.Auth == nil) || !create && (body.ID != r.PathValue("id") || body.Revision < 1) {
		writeJSON(w, 400, errorBody())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	var zones []cloudflare.Zone
	if body.Auth != nil {
		if body.Auth.Validate() != nil {
			writeJSON(w, 400, map[string]string{"error": "cloudflare_credentials_invalid"})
			return
		}
		client := cloudflare.NewClient(*body.Auth)
		client.Before = s.cloudflareService().Budget
		var e error
		zones, e = client.Zones(ctx)
		if e != nil {
			writeJSON(w, 422, map[string]string{"error": cloudflare.ErrorCode(e)})
			return
		}
	}
	e := s.cloudflareService().Locked(ctx, "account:"+body.ID, func() error {
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if create {
			_, e = tx.ExecContext(ctx, "INSERT INTO cloudflare_accounts(id,name,enabled,manage_ssl) VALUES($1,$2,$3,$4)", body.ID, strings.TrimSpace(body.Name), body.Enabled, body.ManageSSL)
		} else {
			var revision int64
			e = tx.QueryRowContext(ctx, "SELECT revision FROM cloudflare_accounts WHERE id=$1 FOR UPDATE", body.ID).Scan(&revision)
			if e != nil {
				return e
			}
			if revision != body.Revision {
				return errors.New("cloudflare_revision_changed")
			}
			_, e = tx.ExecContext(ctx, "UPDATE cloudflare_accounts SET name=$2,enabled=$3,manage_ssl=$4,revision=revision+1,next_attempt_at=now(),error_code='' WHERE id=$1", body.ID, strings.TrimSpace(body.Name), body.Enabled, body.ManageSSL)
		}
		if e != nil {
			return e
		}
		if body.Auth != nil {
			b, _ := json.Marshal(body.Auth)
			defer clear(b)
			if e = s.Container.Secrets.PutCloudflareTx(ctx, tx, body.ID, b); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_zones SET present=false WHERE account_id=$1", body.ID); e != nil {
				return e
			}
			for _, z := range zones {
				if z.Type != "full" || !cloudflare.ValidDomain(z.Name) {
					continue
				}
				_, e = tx.ExecContext(ctx, `INSERT INTO cloudflare_zones(account_id,id,name,status) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,id) DO UPDATE SET name=excluded.name,status=excluded.status,present=true`, body.ID, z.ID, z.Name, z.Status)
				if e != nil {
					return e
				}
			}
			if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_accounts SET refreshed_at=now() WHERE id=$1", body.ID); e != nil {
				return e
			}
		}
		if body.Zones == nil {
			body.Zones = []string{}
		}
		var found int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM cloudflare_zones WHERE account_id=$1 AND id=ANY($2::text[]) AND present AND status='active'", body.ID, pq.Array(body.Zones)).Scan(&found); e != nil {
			return e
		}
		if found != len(body.Zones) {
			return errors.New("cloudflare_zone_selection_invalid")
		}
		if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_zones SET enabled=id=ANY($2::text[]) WHERE account_id=$1", body.ID, pq.Array(body.Zones)); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_zones SET dns_checked_at=NULL,dns_error_code='',dns_next_attempt_at=now() WHERE account_id=$1", body.ID); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_records SET next_attempt_at=now() WHERE account_id=$1 AND state<>'DELETED'", body.ID); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE config_driver_listeners l SET needs_reconcile=true,next_attempt_at=now() FROM config_driver_profiles p WHERE l.profile_id=p.id AND p.spec->>'cloudflare_account'=$1", body.ID); e != nil {
			return e
		}
		return tx.Commit()
	})
	if e != nil {
		code := "cloudflare_save_unconfirmed"
		switch e.Error() {
		case "cloudflare_revision_changed", "cloudflare_zone_selection_invalid", "cloudflare_busy":
			code = e.Error()
		}
		writeJSON(w, 409, map[string]string{"error": code})
		return
	}
	writeJSON(w, 200, map[string]any{"id": body.ID, "state": "saved", "revision": body.Revision + 1})
}
func (s *Server) refreshCloudflareAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !cloudflare.ValidID(id) {
		writeJSON(w, 400, errorBody())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	e := s.cloudflareService().Locked(ctx, "account:"+id, func() error { return s.cloudflareService().Refresh(ctx, id) })
	if errors.Is(e, sql.ErrNoRows) {
		writeJSON(w, 404, errorBody())
		return
	}
	if e != nil {
		writeJSON(w, 422, map[string]string{"error": cloudflare.ErrorCode(e)})
		return
	}
	writeJSON(w, 200, map[string]string{"state": "refreshed"})
}

func (s *Server) cloudflareIPPool(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), "SELECT network::text,fetched_at FROM cloudflare_ip_ranges ORDER BY network")
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	ranges := []map[string]any{}
	for rows.Next() {
		var network string
		var at time.Time
		if rows.Scan(&network, &at) != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		ranges = append(ranges, map[string]any{"network": network, "fetched_at": at})
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"source": cloudflare.OfficialIPSource, "ranges": ranges, "measurement_source": "operator-reported authenticated protocol probes", "measurement_max_age_seconds": 3600})
}
func (s *Server) cloudflareIPRefresh(w http.ResponseWriter, r *http.Request) {
	if e := s.cloudflareService().RefreshRanges(r.Context(), cloudflare.OfficialRanges{}); e != nil {
		writeJSON(w, 502, map[string]string{"error": "cloudflare_ip_source_unavailable"})
		return
	}
	writeJSON(w, 200, map[string]string{"state": "refreshed"})
}
func (s *Server) cloudflareMeasurement(w http.ResponseWriter, r *http.Request) {
	var m cloudflare.Measurement
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&m) != nil || dec.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, errorBody())
		return
	}
	if e := s.cloudflareService().SaveMeasurement(r.Context(), m); e != nil {
		writeJSON(w, 400, map[string]string{"error": "cloudflare_measurement_invalid_or_unowned"})
		return
	}
	writeJSON(w, 200, map[string]string{"state": "recorded"})
}

func (s *Server) cloudflareProbeTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !cloudflare.ValidID(id) {
		writeJSON(w, 400, errorBody())
		return
	}
	where, _ := outputPublicationFilter("ALL")
	var uri string
	e := s.DB.QueryRowContext(r.Context(), `SELECT o.uri FROM output_config_snapshots o JOIN panel_instances p ON p.id=o.panel_id JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=dr.account_id JOIN deployments d ON d.droplet_id=dr.id CROSS JOIN residential_routing_control rc LEFT JOIN panel_routing_state rs ON rs.panel_id=p.id LEFT JOIN panel_client_routes cr ON cr.panel_id=p.id AND cr.client_id=o.client_id `+where+` AND o.cloudflare_record_id=$1 ORDER BY (cr.route_class='DIRECT') DESC LIMIT 1`, id).Scan(&uri)
	if errors.Is(e, sql.ErrNoRows) {
		writeJSON(w, 409, map[string]string{"error": "cloudflare_probe_target_not_ready"})
		return
	}
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), "SELECT network::text FROM cloudflare_ip_ranges WHERE fetched_at>now()-interval '7 days' ORDER BY network")
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	ranges := []string{}
	for rows.Next() {
		var n string
		if rows.Scan(&n) != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		ranges = append(ranges, n)
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"record_id": id, "uri": uri, "ranges": ranges, "source": cloudflare.OfficialIPSource})
}
