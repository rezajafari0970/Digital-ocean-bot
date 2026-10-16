package residentialsync

import (
	"context"
	"time"
)

type ProofCoverage struct {
	ObservedAt  time.Time `json:"observed_at"`
	Eligible    int       `json:"eligible_serving"`
	Applied     int       `json:"current_applied"`
	Fresh       int       `json:"verified_under_60s"`
	Never       int       `json:"never_verified"`
	Due         int       `json:"due"`
	RawFresh    int       `json:"raw_under_15s"`
	RawWithheld int       `json:"raw_fresh_proof_stale"`
	AgeMedian   float64   `json:"proof_age_median_seconds"`
	AgeP95      float64   `json:"proof_age_p95_seconds"`
	AgeMax      float64   `json:"proof_age_max_seconds"`
	OldestDue   float64   `json:"oldest_due_seconds"`
}

// Coverage reports observed proof freshness separately from APPLIED intent.
// Missing/stale proof is never made current by this diagnostic query.
func (s Service) Coverage(ctx context.Context) (ProofCoverage, error) {
	var v ProofCoverage
	err := s.DB.QueryRowContext(ctx, `WITH eligible AS (
 SELECT DISTINCT p.id,c.revision FROM panel_instances p
 JOIN droplets dr ON dr.id=p.droplet_id CROSS JOIN residential_routing_control c
 WHERE c.singleton AND c.enabled AND (c.fleet OR p.id=ANY(c.panel_ids))
 AND p.enabled AND dr.state IN('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now())
 AND EXISTS(SELECT 1 FROM deployments d WHERE d.droplet_id=p.droplet_id AND d.state='PANEL_COMPLETE')
 ), observations AS (
 SELECT e.id,r.verified_at,r.next_check_at,
 COALESCE(r.state='APPLIED' AND r.revision=e.revision,false) AS applied,
 COALESCE(r.state='APPLIED' AND r.revision=e.revision AND r.verified_at>now()-interval '60 seconds',false) AS fresh,
 (r.panel_id IS NULL OR r.revision<>e.revision OR r.next_check_at<=now()) AS due,
 EXISTS(SELECT 1 FROM output_config_snapshots o WHERE o.panel_id=e.id AND o.last_seen_at>now()-interval '15 seconds') AS raw_fresh
 FROM eligible e LEFT JOIN panel_routing_state r ON r.panel_id=e.id
 )
 SELECT clock_timestamp(),count(*),count(*) FILTER(WHERE applied),count(*) FILTER(WHERE fresh),
 count(*) FILTER(WHERE verified_at IS NULL),count(*) FILTER(WHERE due),
 count(*) FILTER(WHERE raw_fresh),count(*) FILTER(WHERE raw_fresh AND NOT fresh),
 COALESCE(percentile_cont(0.5) WITHIN GROUP(ORDER BY EXTRACT(epoch FROM now()-verified_at)),0),
 COALESCE(percentile_cont(0.95) WITHIN GROUP(ORDER BY EXTRACT(epoch FROM now()-verified_at)),0),
 COALESCE(max(EXTRACT(epoch FROM now()-verified_at)),0),
 COALESCE(max(GREATEST(0,EXTRACT(epoch FROM now()-next_check_at))) FILTER(WHERE due),0)
 FROM observations`).Scan(&v.ObservedAt, &v.Eligible, &v.Applied, &v.Fresh, &v.Never, &v.Due, &v.RawFresh, &v.RawWithheld, &v.AgeMedian, &v.AgeP95, &v.AgeMax, &v.OldestDue)
	return v, err
}
