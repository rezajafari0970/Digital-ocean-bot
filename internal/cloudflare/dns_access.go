package cloudflare

import (
	"context"
	"database/sql"
	"errors"
)

// Each zone has its own read-only access check and retry window. A denied zone
// cannot create intents or consume the record queue of a healthy sibling.
func (s Service) checkDNSAccess(ctx context.Context, account string, d Driver) error {
	var zone, name string
	e := s.DB.QueryRowContext(ctx, `SELECT z.id,z.name FROM cloudflare_zones z JOIN cloudflare_accounts a ON a.id=z.account_id
 WHERE z.account_id=$1 AND z.dns_next_attempt_at<=now()
 AND ((a.enabled AND z.enabled AND z.present AND z.status='active') OR EXISTS(SELECT 1 FROM cloudflare_records r WHERE r.account_id=z.account_id AND r.zone_id=z.id AND r.state NOT IN('DELETED','CONFLICT')))
 ORDER BY z.dns_next_attempt_at,z.id LIMIT 1`, account).Scan(&zone, &name)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_zones SET dns_next_attempt_at=now()+interval '5 minutes' WHERE account_id=$1 AND id=$2", account, zone); e != nil {
		return e
	}
	_, accessErr := d.Records(ctx, zone, name)
	code := ErrorCode(accessErr)
	if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_zones SET dns_checked_at=now(),dns_error_code=$3 WHERE account_id=$1 AND id=$2", account, zone, code); e != nil {
		return e
	}
	return accessErr
}

func (s Service) dnsAccessFailure(ctx context.Context, account string) error {
	var code string
	e := s.DB.QueryRowContext(ctx, `SELECT dns_error_code FROM cloudflare_zones WHERE account_id=$1 AND enabled AND present AND status='active' AND dns_error_code<>'' ORDER BY id LIMIT 1`, account).Scan(&code)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	return errors.New(code)
}

// PREPARED plus no provider/activation receipt means no uncertain creation is
// outstanding. Retire only expired local intents; SENT always requires a read.
func (s Service) retireUnsentExpired(ctx context.Context, account string) error {
	_, e := s.DB.ExecContext(ctx, `UPDATE cloudflare_records SET state='DELETED',error_code='',verified_at=NULL
 WHERE account_id=$1 AND state='PREPARED' AND provider_id='' AND activated_at IS NULL
 AND (expires_at<=now() OR NOT EXISTS(SELECT 1 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id WHERE p.id=cloudflare_records.panel_id AND p.enabled AND d.state IN('READY','EXPIRING') AND (d.expires_at IS NULL OR d.expires_at>now())))`, account)
	return e
}
