package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"time"
)

// Trojan uses the same fresh native snapshot, routing proof, share/account scope
// and country naming as VLESS. Failure here withholds Trojan only.
func (s *Server) collectTrojanOutput(ctx context.Context, panel, host string, raws []json.RawMessage) []outputRecord {
	var account, ref string
	var id int64
	var until time.Time
	var p trojantcp.Policy
	err := s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,t.credential_ref,t.remote_id,t.port,t.certificate_not_after,pol.direct_enabled,pol.residential_enabled
 FROM trojan_tcp_listeners t JOIN panel_instances pi ON pi.id=t.panel_id CROSS JOIN trojan_tcp_policy pol
 WHERE t.panel_id=$1 AND pol.enabled AND (pol.fleet OR t.panel_id=ANY(pol.panel_ids)) AND t.state='VERIFIED' AND t.verified_at>now()-interval '5 minutes' AND t.certificate_not_after>now()+interval '5 minutes'`, panel).Scan(&account, &ref, &id, &p.Port, &until, &p.Direct, &p.Residential)
	if err != nil {
		return nil
	}
	b, err := s.Container.Secrets.Get(ctx, account, ref)
	if err != nil {
		return nil
	}
	var credentials trojantcp.Credentials
	if json.Unmarshal(b, &credentials) != nil {
		return nil
	}
	clients := credentials.Clients(p)
	observed, err := trojantcp.Observe(raws, p, host, clients, sql.NullInt64{Int64: id, Valid: true})
	if err != nil || observed != id {
		return nil
	}
	result := []outputRecord{}
	until = until.Add(-5 * time.Minute)
	for _, c := range clients {
		var owned bool
		err = s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM trojan_tcp_clients WHERE panel_id=$1 AND client_id=$2 AND email=$3 AND route_class=$4)", panel, c.ID, c.Email, c.RouteClass).Scan(&owned)
		if err != nil || !owned {
			return nil
		}
		uri, err := export.TrojanTCPURI(export.TrojanTCP{Host: host, Port: p.Port, Password: c.Password, Remark: "Trojan"})
		if err != nil {
			return nil
		}
		result = append(result, outputRecord{ClientID: c.ID, URI: uri, VisibleUntil: &until})
	}
	return result
}
