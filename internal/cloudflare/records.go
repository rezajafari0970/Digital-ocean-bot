package cloudflare

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflareschedule"
	"net/netip"
	"time"
)

type Policy struct {
	Account     string `json:"cloudflare_account,omitempty"`
	Operator    string `json:"cloudflare_operator,omitempty"`
	Records     int    `json:"cloudflare_records,omitempty"`
	Interval    int    `json:"cloudflare_interval_seconds,omitempty"`
	MinLifetime int    `json:"cloudflare_min_lifetime_seconds,omitempty"`
	MaxLifetime int    `json:"cloudflare_max_lifetime_seconds,omitempty"`
	MaxActive   int    `json:"cloudflare_max_active,omitempty"`
}

func (p Policy) Validate() error {
	if !ValidOperator(p.Operator) || !ValidID(p.Account) {
		return errors.New("profile_cloudflare_schedule_invalid")
	}
	return (cloudflareschedule.Schedule{Records: p.Records, Interval: p.Interval, MinLifetime: p.MinLifetime, MaxLifetime: p.MaxLifetime, MaxActive: p.MaxActive}).Validate()
}

// Durable scheduling uses wall time, never catch-up bursts after a restart.
func (s Service) plan(ctx context.Context, account string) error {
	var accessible bool
	if e := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM cloudflare_zones WHERE account_id=$1 AND enabled AND present AND status='active' AND dns_checked_at>now()-interval '10 minutes' AND dns_error_code='')", account).Scan(&accessible); e != nil {
		return e
	}
	if !accessible {
		return nil
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO cloudflare_schedules(profile_id,panel_id)
 SELECT cp.id,l.panel_id FROM config_driver_profiles cp JOIN config_driver_listeners l ON l.profile_id=cp.id
 WHERE cp.enabled AND cp.delete_requested_at IS NULL AND cp.spec->>'cloudflare_account'=$1::text ON CONFLICT DO NOTHING`, account)
	if e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT sc.profile_id::text,sc.panel_id::text,cp.spec,dep.host,dr.expires_at
 FROM cloudflare_schedules sc JOIN config_driver_profiles cp ON cp.id=sc.profile_id
 JOIN panel_instances pi ON pi.id=sc.panel_id JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id
 JOIN deployments dep ON dep.droplet_id=dr.id JOIN cloudflare_accounts ca ON ca.id=$1::uuid
 WHERE ca.enabled AND cp.enabled AND cp.delete_requested_at IS NULL AND cp.spec->>'cloudflare_account'=$1::text
 AND (cp.fleet OR pi.id=ANY(cp.panel_ids)) AND pi.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND dr.state IN('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 minutes')
 AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=pi.id AND ct.state<>'SUCCEEDED' AND cj.state NOT IN('SUCCEEDED','CANCELLED')) AND sc.next_batch_at<=now()
 ORDER BY (SELECT count(*) FROM cloudflare_records coverage WHERE coverage.profile_id=sc.profile_id AND coverage.panel_id=sc.panel_id AND coverage.state IN('PREPARED','SENT','DNS_READY') AND coverage.expires_at>now()+interval '5 minutes'),sc.next_batch_at,sc.profile_id,sc.panel_id
 LIMIT 8 FOR UPDATE OF sc SKIP LOCKED`, account)
	if e != nil {
		return e
	}
	type item struct {
		id, panel, host string
		raw             []byte
		until           sql.NullTime
	}
	items := []item{}
	for rows.Next() {
		var x item
		if e = rows.Scan(&x.id, &x.panel, &x.raw, &x.host, &x.until); e != nil {
			break
		}
		items = append(items, x)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	for _, x := range items {
		// Backpressure on one server must not monopolize the due schedule queue.
		if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_schedules SET next_batch_at=now()+interval '30 seconds' WHERE profile_id=$1 AND panel_id=$2", x.id, x.panel); e != nil {
			return e
		}
		var p Policy
		if json.Unmarshal(x.raw, &p) != nil || p.Validate() != nil {
			continue
		}
		ip, e := netip.ParseAddr(x.host)
		if e != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
			continue
		}
		var count int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM cloudflare_records WHERE profile_id=$1 AND panel_id=$2 AND state<>'DELETED'", x.id, x.panel).Scan(&count); e != nil {
			return e
		}
		n := min(p.Records, p.MaxActive-count)
		if n <= 0 {
			continue
		}
		for i := 0; i < n; i++ {
			var zone, name string
			e = tx.QueryRowContext(ctx, `SELECT z.id,z.name FROM cloudflare_zones z WHERE z.account_id=$1 AND z.enabled AND z.present AND z.status='active' AND z.dns_checked_at>now()-interval '10 minutes' AND z.dns_error_code=''
 AND (SELECT count(*) FROM cloudflare_records r WHERE r.account_id=z.account_id AND r.zone_id=z.id AND r.state<>'DELETED')<500
 AND (z.dns_record_quota IS NULL OR z.dns_quota_checked_at IS NULL OR z.dns_quota_checked_at<now()-interval '2 minutes' OR z.dns_record_quota>z.dns_record_usage+(SELECT count(*) FROM cloudflare_records qr WHERE qr.account_id=z.account_id AND qr.zone_id=z.id AND (qr.state IN('PREPARED','SENT') OR qr.state='DNS_READY' AND qr.activated_at>z.dns_quota_checked_at)))
 ORDER BY (SELECT count(*) FROM cloudflare_records r WHERE r.account_id=z.account_id AND r.zone_id=z.id AND r.state<>'DELETED'),z.id LIMIT 1`, account).Scan(&zone, &name)
			if errors.Is(e, sql.ErrNoRows) {
				// A partial batch still consumes its interval. Otherwise scarce
				// capacity could create extra records every 30 seconds.
				if i > 0 {
					if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_schedules SET next_batch_at=now()+$3*interval '1 second' WHERE profile_id=$1 AND panel_id=$2", x.id, x.panel, p.Interval); e != nil {
						return e
					}
				}
				return tx.Commit()
			}
			if e != nil {
				return e
			}
			hostname, e := Hostname(name)
			if e != nil {
				return e
			}
			id, e := NewID()
			if e != nil {
				return e
			}
			life, e := Lifetime(p.MinLifetime, p.MaxLifetime)
			if e != nil {
				return e
			}
			expiry := time.Now().Add(life)
			if x.until.Valid && x.until.Time.Before(expiry) {
				expiry = x.until.Time
			}
			edge, e := selectEdge(ctx, tx, account, zone, p.Operator, hostname)
			if e != nil {
				return e
			}
			_, e = tx.ExecContext(ctx, `INSERT INTO cloudflare_records(id,account_id,zone_id,profile_id,panel_id,hostname,origin_ip,expires_at,edge_address,operator,lifetime_seconds) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, account, zone, x.id, x.panel, hostname, x.host, expiry, edge, p.Operator, int(life.Seconds()))
			if e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_schedules SET next_batch_at=now()+$3*interval '1 second' WHERE profile_id=$1 AND panel_id=$2", x.id, x.panel, p.Interval); e != nil {
			return e
		}
	}
	return tx.Commit()
}

type recordJob struct {
	lifetime                                int
	planned                                 time.Time
	activated, serverExpiry                 sql.NullTime
	id, zone, hostname, ip, state, provider string
	expired                                 bool
}

func (s Service) recordOne(ctx context.Context, account string, d Driver) error {
	var r recordJob
	e := s.DB.QueryRowContext(ctx, `SELECT r.id::text,r.zone_id,r.hostname,host(r.origin_ip),r.state,r.provider_id,
 (pi.id IS NULL OR r.expires_at<=now() OR NOT ca.enabled OR NOT z.enabled OR NOT z.present OR z.status<>'active' OR r.operator<>COALESCE(cp.spec->>'cloudflare_operator','') OR NOT cp.enabled OR cp.delete_requested_at IS NOT NULL OR cp.spec->>'cloudflare_account' IS DISTINCT FROM r.account_id::text OR NOT(cp.fleet OR r.panel_id=ANY(cp.panel_ids)) OR NOT pi.enabled OR NOT a.enabled OR a.deletion_requested_at IS NOT NULL OR dr.state NOT IN('READY','EXPIRING') OR (dr.expires_at IS NOT NULL AND dr.expires_at<=now())) AS expired,r.lifetime_seconds,r.created_at,r.activated_at,dr.expires_at
 FROM cloudflare_records r JOIN cloudflare_accounts ca ON ca.id=r.account_id JOIN cloudflare_zones z ON z.account_id=r.account_id AND z.id=r.zone_id
 JOIN config_driver_profiles cp ON cp.id=r.profile_id LEFT JOIN panel_instances pi ON pi.id=r.panel_id LEFT JOIN droplets dr ON dr.id=pi.droplet_id LEFT JOIN accounts a ON a.id=pi.account_id
 WHERE r.account_id=$1 AND r.state NOT IN('DELETED','CONFLICT') AND r.next_attempt_at<=now()
 AND z.dns_error_code='' AND z.dns_checked_at>now()-interval '10 minutes'
 AND NOT(r.state='PREPARED' AND r.expires_at>now() AND z.dns_record_quota IS NOT NULL AND z.dns_quota_checked_at>now()-interval '2 minutes' AND z.dns_record_usage+(SELECT count(*) FROM cloudflare_records qr WHERE qr.account_id=z.account_id AND qr.zone_id=z.id AND qr.state='DNS_READY' AND qr.activated_at>z.dns_quota_checked_at)>=z.dns_record_quota)
 ORDER BY (r.expires_at<=now()) DESC,r.next_attempt_at LIMIT 1`, account).Scan(&r.id, &r.zone, &r.hostname, &r.ip, &r.state, &r.provider, &r.expired, &r.lifetime, &r.planned, &r.activated, &r.serverExpiry)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	// Read before every mutation, including interrupted or ambiguous operations.
	records, e := d.Records(ctx, r.zone, r.hostname)
	if e != nil {
		return s.recordError(ctx, r.id, e)
	}
	typ := "A"
	if ip, _ := netip.ParseAddr(r.ip); ip.Is6() {
		typ = "AAAA"
	}
	want := Record{Type: typ, Name: r.hostname, Content: r.ip, Comment: "dob-cloudflare:" + r.id, Proxied: true, TTL: 1}
	for _, got := range records {
		if len(records) != 1 || !Owns(want, got) {
			_, _ = s.DB.ExecContext(ctx, "UPDATE cloudflare_records SET state='CONFLICT',error_code='cloudflare_record_conflict',verified_at=NULL WHERE id=$1", r.id)
			return errors.New("cloudflare_record_conflict")
		}
	}
	if r.expired || r.state == "DELETING" {
		if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_records SET state='DELETING',verified_at=NULL WHERE id=$1", r.id); e != nil {
			return e
		}
		if len(records) == 0 {
			_, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_records SET state='DELETED',error_code='' WHERE id=$1", r.id)
			return e
		}
		if e = d.DeleteRecord(ctx, r.zone, records[0].ID); e != nil {
			return s.recordError(ctx, r.id, e)
		}
		// Deletion is confirmed by a later read; a lost DELETE response is harmless.
		_, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_records SET next_attempt_at=now()+interval '5 seconds' WHERE id=$1", r.id)
		return e
	}
	if len(records) == 1 {
		return s.activateRecord(ctx, r, records[0])
	}
	if r.state == "SENT" {
		return s.recordError(ctx, r.id, errors.New("cloudflare_write_unconfirmed"))
	}
	if r.state == "DNS_READY" { // Confirmed external removal: retire this name and create a new intent on the next scheduled batch.
		_, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_records SET state='DELETED',verified_at=NULL,error_code='cloudflare_record_removed' WHERE id=$1", r.id)
		return e
	}
	if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_records SET state='SENT',next_attempt_at=now()+interval '15 seconds' WHERE id=$1 AND state='PREPARED'", r.id); e != nil {
		return e
	}
	got, e := d.CreateRecord(ctx, r.zone, want)
	if e != nil {
		var api *APIError
		if errors.As(e, &api) && (api.Status == 400 || api.Status == 401 || api.Status == 403 || api.Status == 429) {
			_, _ = s.DB.ExecContext(ctx, "UPDATE cloudflare_records SET state='PREPARED' WHERE id=$1 AND state='SENT'", r.id)
		}
		return s.recordError(ctx, r.id, e)
	}
	if got.ID == "" || !Owns(want, got) {
		return s.recordError(ctx, r.id, errors.New("cloudflare_write_unconfirmed"))
	}
	return s.activateRecord(ctx, r, got)
}
func (s Service) recordError(ctx context.Context, id string, e error) error {
	if ctx.Err() == nil {
		var api *APIError
		if errors.As(e, &api) && (api.Status == 401 || api.Status == 403) {
			_, _ = s.DB.ExecContext(ctx, "UPDATE cloudflare_zones z SET dns_error_code=$2,dns_checked_at=now(),dns_next_attempt_at=now()+interval '5 minutes' FROM cloudflare_records r WHERE r.id=$1 AND z.account_id=r.account_id AND z.id=r.zone_id", id, ErrorCode(e))
		}
		_, _ = s.DB.ExecContext(ctx, "UPDATE cloudflare_records SET error_code=$2,failures=failures+1,next_attempt_at=now()+LEAST(300,15*power(2,LEAST(failures,4)))*interval '1 second' WHERE id=$1", id, ErrorCode(e))
	}
	return e
}

func (s Service) activateRecord(ctx context.Context, r recordJob, got Record) error {
	at := got.CreatedOn
	if r.activated.Valid {
		at = r.activated.Time
	} else if at.IsZero() || at.After(time.Now().Add(time.Minute)) || at.Before(r.planned.Add(-time.Minute)) {
		return s.recordError(ctx, r.id, errors.New("cloudflare_record_time_unconfirmed"))
	}
	expiry := at.Add(time.Duration(r.lifetime) * time.Second)
	if r.serverExpiry.Valid && r.serverExpiry.Time.Before(expiry) {
		expiry = r.serverExpiry.Time
	}
	_, e := s.DB.ExecContext(ctx, `UPDATE cloudflare_records SET provider_id=$2,state='DNS_READY',observed_at=now(),error_code='',failures=0,
 expires_at=CASE WHEN activated_at IS NULL THEN $4 ELSE expires_at END,activated_at=COALESCE(activated_at,$3),next_attempt_at=LEAST($4,now()+interval '5 minutes') WHERE id=$1`, r.id, got.ID, at, expiry)
	return e
}
