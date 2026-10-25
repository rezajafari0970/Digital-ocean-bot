package cloudflare

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflareschedule"
)

// FleetCapacity is advisory. It neither admits DNS writes nor changes profiles,
// selected domains, TLS, expiry, account eligibility or publication proofs.
type FleetCapacity struct {
	Profiles               int    `json:"profiles"`
	CurrentServers         int64  `json:"current_servers"`
	PlannedServers         int64  `json:"planned_servers"`
	CurrentRequiredRecords int64  `json:"current_required_records"`
	RequiredRecords        int64  `json:"required_records"`
	KnownRecordCapacity    int64  `json:"known_record_capacity"`
	AvailableNow           int64  `json:"available_now"`
	EligibleZones          int    `json:"eligible_zones"`
	UnknownZones           int    `json:"unknown_zones"`
	BlockedZones           int    `json:"blocked_zones"`
	InvalidProfiles        int    `json:"invalid_profiles"`
	Complete               bool   `json:"complete"`
	Shortfall              *int64 `json:"shortfall,omitempty"`
}

// Capacity uses one read-only snapshot and a constant number of queries. It does
// not call a provider. Targets include paused-by-provider accounts so restoring
// their authentication cannot silently overbook a supposedly sufficient fleet.
func (s Service) Capacity(ctx context.Context) (map[string]*FleetCapacity, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := map[string]*FleetCapacity{}
	get := func(id string) *FleetCapacity {
		if out[id] == nil {
			out[id] = &FleetCapacity{}
		}
		return out[id]
	}
	rows, err := tx.QueryContext(ctx, `
WITH enabled_accounts AS (
 SELECT a.id,a.desired_server_count FROM accounts a
 WHERE a.enabled AND a.deleted_at IS NULL AND a.deletion_requested_at IS NULL
), fleet AS (
 SELECT COALESCE(sum(GREATEST(a.desired_server_count,
  (SELECT count(*) FROM droplets dr WHERE dr.account_id=a.id
   AND dr.state IN('PROVISIONING','READY','EXPIRING')
   AND (dr.expires_at IS NULL OR dr.expires_at>now())))),0)::bigint AS target
 FROM enabled_accounts a
), scoped AS (
 SELECT pi.id,a.provider_state,dr.state,dr.expires_at,
  EXISTS(SELECT 1 FROM deployments dep WHERE dep.droplet_id=dr.id AND dep.state='PANEL_COMPLETE') AS complete,
  EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id
   WHERE ct.panel_id=pi.id AND ct.state<>'SUCCEEDED' AND cj.state NOT IN('SUCCEEDED','CANCELLED')) AS cleaning
 FROM panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id
 JOIN accounts a ON a.id=pi.account_id JOIN enabled_accounts ea ON ea.id=a.id
 WHERE pi.enabled AND dr.state IN('PROVISIONING','READY','EXPIRING')
 AND (dr.expires_at IS NULL OR dr.expires_at>now())
)
SELECT ca.id::text,cp.spec,
 (SELECT count(*) FROM scoped x WHERE (cp.fleet OR x.id=ANY(cp.panel_ids))
  AND x.provider_state='ACTIVE' AND x.state IN('READY','EXPIRING') AND x.complete AND NOT x.cleaning
  AND (x.expires_at IS NULL OR x.expires_at>now()+interval '10 minutes')),
 CASE WHEN cp.fleet THEN fleet.target ELSE
  (SELECT count(*) FROM scoped x WHERE x.id=ANY(cp.panel_ids) AND NOT x.cleaning) END
FROM config_driver_profiles cp JOIN cloudflare_accounts ca ON ca.id::text=cp.spec->>'cloudflare_account'
CROSS JOIN fleet WHERE ca.enabled AND cp.enabled AND cp.delete_requested_at IS NULL`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var raw []byte
		var current, planned int64
		if err = rows.Scan(&id, &raw, &current, &planned); err != nil {
			rows.Close()
			return nil, err
		}
		c := get(id)
		c.Profiles++
		var p Policy
		if json.Unmarshal(raw, &p) != nil || p.Validate() != nil {
			c.InvalidProfiles++
			continue
		}
		schedule := cloudflareschedule.Schedule{Records: p.Records, Interval: p.Interval, MinLifetime: p.MinLifetime, MaxLifetime: p.MaxLifetime, MaxActive: p.MaxActive}
		perServer := int64(schedule.RequiredActive())
		c.CurrentServers += current
		c.PlannedServers += max(current, planned)
		c.CurrentRequiredRecords += current * perServer
		c.RequiredRecords += max(current, planned) * perServer
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `
SELECT z.account_id::text,
 COALESCE(ca.enabled AND z.present AND z.status='active' AND z.ssl_strict
  AND z.checked_at>now()-interval '10 minutes'
  AND z.dns_checked_at>now()-interval '10 minutes' AND z.dns_error_code='',false),
 COALESCE(z.dns_quota_checked_at>now()-interval '2 minutes' AND z.dns_quota_error_code='',false),
 z.dns_record_quota,z.dns_record_usage,
 (SELECT count(*) FROM cloudflare_records r WHERE r.account_id=z.account_id AND r.zone_id=z.id AND r.state<>'DELETED'),
 (SELECT count(*) FROM cloudflare_records r WHERE r.account_id=z.account_id AND r.zone_id=z.id
  AND r.state<>'DELETED' AND r.provider_id<>'' AND r.activated_at<=z.dns_quota_checked_at),
 (SELECT count(*) FROM cloudflare_records r WHERE r.account_id=z.account_id AND r.zone_id=z.id
  AND (r.state IN('PREPARED','SENT') OR r.state='DNS_READY' AND r.activated_at>z.dns_quota_checked_at))
FROM cloudflare_zones z JOIN cloudflare_accounts ca ON ca.id=z.account_id WHERE z.enabled`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var eligible, fresh bool
		var quota sql.NullInt64
		var usage, tracked, confirmed, pending int64
		if err = rows.Scan(&id, &eligible, &fresh, &quota, &usage, &tracked, &confirmed, &pending); err != nil {
			rows.Close()
			return nil, err
		}
		c := get(id)
		if !eligible {
			c.BlockedZones++
			continue
		}
		c.EligibleZones++
		if !fresh || !quota.Valid {
			c.UnknownZones++
			continue
		}
		// Provider usage includes unrelated records: only confirmed managed
		// records already present at the observation may be reclaimed here.
		// The worker also imposes a per-zone limit of 500 tracked records.
		external := max(int64(0), usage-confirmed)
		c.KnownRecordCapacity += min(int64(500), max(int64(0), quota.Int64-external))
		c.AvailableNow += max(int64(0), min(500-tracked, quota.Int64-usage-pending))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, c := range out {
		c.Complete = c.UnknownZones == 0 && c.InvalidProfiles == 0
		if c.Complete {
			shortfall := max(int64(0), c.RequiredRecords-c.KnownRecordCapacity)
			c.Shortfall = &shortfall
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
