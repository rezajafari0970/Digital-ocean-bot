package cloudflare

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"github.com/lib/pq"
	"net"
	"strconv"
	"time"
)

var ErrDNSQuotaUnavailable = errors.New("cloudflare_dns_quota_exhausted")

type Endpoint struct {
	ID, Hostname, Address string
	ExpiresAt             time.Time
	Classes               []string
}

func (s Service) ProbeEndpoint(ctx context.Context, account, profile, panel string) (Endpoint, error) {
	var e Endpoint
	err := s.DB.QueryRowContext(ctx, `WITH candidate AS(SELECT r.id FROM cloudflare_records r JOIN cloudflare_zones z ON z.account_id=r.account_id AND z.id=r.zone_id JOIN cloudflare_accounts a ON a.id=r.account_id
 WHERE r.account_id=$1 AND r.profile_id=$2 AND r.panel_id=$3 AND r.state='DNS_READY' AND r.expires_at>now()+interval '5 minutes' AND r.observed_at>now()-interval '10 minutes' AND r.operator=COALESCE((SELECT spec->>'cloudflare_operator' FROM config_driver_profiles WHERE id=r.profile_id),'') AND z.ssl_strict AND z.checked_at>now()-interval '10 minutes' AND z.enabled AND z.present AND z.status='active' AND a.enabled
 ORDER BY r.probe_next_at,r.created_at LIMIT 1 FOR UPDATE OF r SKIP LOCKED)
 UPDATE cloudflare_records r SET probe_next_at=now()+interval '30 seconds' FROM candidate c WHERE r.id=c.id RETURNING r.id::text,r.hostname,r.expires_at,COALESCE(NULLIF(r.edge_address,''),r.hostname)`, account, profile, panel).Scan(&e.ID, &e.Hostname, &e.ExpiresAt, &e.Address)

	if errors.Is(err, sql.ErrNoRows) {
		var full bool
		queryErr := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloudflare_zones z WHERE z.account_id=$1 AND z.enabled AND z.present AND z.status='active')
 AND NOT EXISTS(SELECT 1 FROM cloudflare_zones z WHERE z.account_id=$1 AND z.enabled AND z.present AND z.status='active'
 AND (z.dns_record_quota IS NULL OR z.dns_quota_checked_at IS NULL OR z.dns_quota_checked_at<now()-interval '2 minutes'
 OR z.dns_record_quota>z.dns_record_usage+(SELECT count(*) FROM cloudflare_records r WHERE r.account_id=z.account_id AND r.zone_id=z.id AND (r.state IN('PREPARED','SENT') OR r.state='DNS_READY' AND r.activated_at>z.dns_quota_checked_at))))`, account).Scan(&full)
		if queryErr == nil && full {
			err = errors.Join(err, ErrDNSQuotaUnavailable)
		}
	}
	return e, err
}
func (s Service) Endpoints(ctx context.Context, account, profile, panel string, revision int64) ([]Endpoint, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT r.id::text,r.hostname,r.expires_at,r.verified_classes,COALESCE(NULLIF(r.edge_address,''),r.hostname) FROM cloudflare_records r JOIN cloudflare_zones z ON z.account_id=r.account_id AND z.id=r.zone_id JOIN cloudflare_accounts a ON a.id=r.account_id
 WHERE r.account_id=$1 AND r.profile_id=$2 AND r.panel_id=$3 AND r.state='DNS_READY' AND r.expires_at>now()+interval '5 minutes' AND r.verified_at>now()-interval '5 minutes' AND r.verified_revision=$4 AND r.observed_at>now()-interval '10 minutes' AND r.operator=COALESCE((SELECT spec->>'cloudflare_operator' FROM config_driver_profiles WHERE id=r.profile_id),'') AND z.ssl_strict AND z.checked_at>now()-interval '10 minutes' AND z.enabled AND z.present AND z.status='active' AND a.enabled ORDER BY r.created_at DESC`, account, profile, panel, revision)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Endpoint{}
	for rows.Next() {
		var x Endpoint
		if e = rows.Scan(&x.ID, &x.Hostname, &x.ExpiresAt, pq.Array(&x.Classes), &x.Address); e != nil {
			return nil, e
		}

		out = append(out, x)
	}
	return out, rows.Err()
}
func ConfirmEndpoint(ctx context.Context, tx *sql.Tx, id string, revision int64, classes []string) error {
	_, e := tx.ExecContext(ctx, "UPDATE cloudflare_records SET verified_at=now(),verified_revision=$2,verified_classes=$3 WHERE id=$1 AND state='DNS_READY' AND expires_at>now()+interval '5 minutes'", id, revision, pq.Array(classes))
	return e
}
func ProbeTLS(ctx context.Context, address, hostname string, port int) error {
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 4 * time.Second}, Config: &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}}
	c, e := d.DialContext(ctx, "tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if e != nil {
		return e
	}
	return c.Close()
}
