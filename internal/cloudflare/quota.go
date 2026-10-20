package cloudflare

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
)

type DNSQuota struct {
	Limit *int
	Usage int
}
type DNSQuotaDriver interface {
	DNSQuota(context.Context, string) (DNSQuota, error)
}

func (c *Client) DNSQuota(ctx context.Context, zone string) (DNSQuota, error) {
	var wire struct {
		Limit *int `json:"record_quota"`
		Usage *int `json:"record_usage"`
	}
	_, e := c.request(ctx, "GET", "/zones/"+url.PathEscape(zone)+"/dns_records/usage", nil, &wire)
	if e != nil {
		return DNSQuota{}, e
	}
	if wire.Usage == nil || *wire.Usage < 0 || wire.Limit != nil && *wire.Limit < 0 {
		return DNSQuota{}, errors.New("cloudflare_dns_quota_unavailable")
	}
	return DNSQuota{Limit: wire.Limit, Usage: *wire.Usage}, nil
}
func (s Service) checkDNSQuota(ctx context.Context, account string, d Driver) error {
	driver, ok := d.(DNSQuotaDriver)
	if !ok {
		return nil
	}
	var zone string
	e := s.DB.QueryRowContext(ctx, `SELECT z.id FROM cloudflare_zones z JOIN cloudflare_accounts a ON a.id=z.account_id WHERE z.account_id=$1 AND z.enabled AND z.present AND z.status='active' AND a.enabled AND z.dns_error_code='' AND z.dns_quota_next_attempt_at<=now() ORDER BY z.dns_quota_checked_at NULLS FIRST LIMIT 1`, account).Scan(&zone)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_zones SET dns_quota_next_attempt_at=now()+interval '1 minute' WHERE account_id=$1 AND id=$2", account, zone); e != nil {
		return e
	}
	quota, e := driver.DNSQuota(ctx, zone)
	if e != nil {
		// An unavailable quota endpoint must not prevent ownership checks or cleanup.
		_, dbErr := s.DB.ExecContext(ctx, "UPDATE cloudflare_zones SET dns_quota_error_code='cloudflare_dns_quota_unavailable' WHERE account_id=$1 AND id=$2", account, zone)
		return dbErr
	}
	_, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_zones SET dns_record_quota=$3,dns_record_usage=$4,dns_quota_checked_at=now(),dns_quota_error_code='' WHERE account_id=$1 AND id=$2", account, zone, quota.Limit, quota.Usage)
	return e
}
