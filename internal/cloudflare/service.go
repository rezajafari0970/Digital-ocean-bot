package cloudflare

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"time"
)

type TokenStore interface {
	GetCloudflare(context.Context, string) ([]byte, error)
}
type Service struct {
	DB               *sql.DB
	Secrets          TokenStore
	Factory          Factory
	CertificateRoots []byte
}

func (s Service) Driver(ctx context.Context, id string) (Driver, error) {
	if s.Factory != nil {
		return s.Factory(ctx, id)
	}
	if s.Secrets == nil {
		return nil, errors.New("cloudflare_credentials_unavailable")
	}
	b, e := s.Secrets.GetCloudflare(ctx, id)
	if e != nil {
		return nil, errors.New("cloudflare_credentials_unavailable")
	}
	defer clear(b)
	var a Auth
	if json.Unmarshal(b, &a) != nil {
		return nil, errors.New("cloudflare_credentials_invalid")
	}
	c := NewClient(a)
	c.Before = s.Budget
	return c, nil
}

// One aggregate budget covers all tokens, API handlers and worker processes.
func (s Service) Budget(ctx context.Context) error {
	var at time.Time
	e := s.DB.QueryRowContext(ctx, `UPDATE cloudflare_api_budget SET next_at=GREATEST(next_at,now())+interval '350 milliseconds' WHERE singleton RETURNING next_at-interval '350 milliseconds'`).Scan(&at)
	if e != nil {
		return e
	}
	timer := time.NewTimer(max(time.Until(at), 0))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (s Service) Locked(ctx context.Context, key string, fn func() error) error {
	c, e := s.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	var held bool
	if e = c.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", "cloudflare:"+key).Scan(&held); e != nil {
		return e
	}
	if !held {
		return errors.New("cloudflare_busy")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, e := c.ExecContext(cleanup, "SELECT pg_advisory_unlock_all()"); e != nil {
			_ = c.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	return fn()
}
func (s Service) Refresh(ctx context.Context, id string) error {
	d, e := s.Driver(ctx, id)
	if e != nil {
		return e
	}
	zones, e := d.Zones(ctx)
	if e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_zones SET present=false WHERE account_id=$1", id); e != nil {
		return e
	}
	for _, z := range zones {
		if !ValidDomain(z.Name) || z.Type != "full" {
			continue
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO cloudflare_zones(account_id,id,name,status) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,id) DO UPDATE SET name=excluded.name,status=excluded.status,present=true`, id, z.ID, z.Name, z.Status)
		if e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, "UPDATE cloudflare_accounts SET refreshed_at=now(),error_code='',next_attempt_at=now() WHERE id=$1", id)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s Service) RunOne(ctx context.Context) error {
	var id string
	e := s.DB.QueryRowContext(ctx, `WITH candidate AS(SELECT id FROM cloudflare_accounts WHERE next_attempt_at<=now() ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE cloudflare_accounts a SET next_attempt_at=now()+interval '500 milliseconds' FROM candidate c WHERE a.id=c.id RETURNING a.id::text`).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	_ = s.RefreshRangesIfDue(ctx) // Range failure falls back to ordinary owned DNS.
	e = s.Locked(ctx, "account:"+id, func() error {
		d, e := s.Driver(ctx, id)
		if e != nil {
			return e
		}
		zoneError := s.checkZone(ctx, id, d)
		planError := s.plan(ctx, id)
		if e = s.recordOne(ctx, id, d); e != nil {
			return e
		}
		if zoneError != nil {
			return zoneError
		}
		return planError
	})
	if e != nil && ctx.Err() == nil && e.Error() != "cloudflare_busy" {
		delay := 500 * time.Millisecond
		var api *APIError
		if errors.As(e, &api) && (api.Status == 401 || api.Status == 429) {
			delay = api.RetryAfter
		}
		code := ErrorCode(e)
		_, _ = s.DB.ExecContext(ctx, "UPDATE cloudflare_accounts SET error_code=$2,next_attempt_at=now()+$3*interval '1 second' WHERE id=$1", id, code, delay.Seconds())
	}
	if e == nil {
		_, _ = s.DB.ExecContext(ctx, "UPDATE cloudflare_accounts SET error_code='' WHERE id=$1", id)
	}
	return e
}
func ErrorCode(e error) string {
	if e == nil {
		return ""
	}
	switch e.Error() {
	case "cloudflare_permission_denied", "cloudflare_rate_limited", "cloudflare_busy", "cloudflare_record_conflict", "cloudflare_write_unconfirmed", "cloudflare_no_active_zones", "cloudflare_ssl_strict_required", "cloudflare_grpc_required", "cloudflare_credentials_unavailable", "cloudflare_certificate_issue_unconfirmed", "cloudflare_certificate_invalid", "cloudflare_certificate_node_retrying", "cloudflare_certificate_install_unconfirmed", "cloudflare_csr_invalid", "cloudflare_account_disabled":
		return e.Error()
	}
	return "cloudflare_operation_retrying"
}
func NewID() (string, error) { return sanaei.UUIDv4() }

// Zone settings are independent evidence. A failed settings API cannot publish
// indefinitely using an old strict-TLS observation.
func (s Service) checkZone(ctx context.Context, account string, d Driver) error {
	var zone string
	var manage bool
	e := s.DB.QueryRowContext(ctx, `SELECT z.id,a.manage_ssl FROM cloudflare_zones z JOIN cloudflare_accounts a ON a.id=z.account_id WHERE z.account_id=$1 AND z.enabled AND z.present AND a.enabled AND z.next_attempt_at<=now() AND (z.checked_at IS NULL OR z.checked_at<now()-interval '5 minutes') ORDER BY z.checked_at NULLS FIRST LIMIT 1`, account).Scan(&zone, &manage)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_zones SET next_attempt_at=now()+interval '5 minutes' WHERE account_id=$1 AND id=$2", account, zone); e != nil {
		return e
	}
	mode, e := d.Setting(ctx, zone, "ssl")
	if e != nil {
		return e
	}
	if mode != "strict" && manage {
		if e = d.SetSetting(ctx, zone, "ssl", "strict"); e != nil {
			return e
		}
		mode, e = d.Setting(ctx, zone, "ssl")
		if e != nil {
			return e
		}
	}
	strict := mode == "strict"
	if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_zones SET ssl_strict=$3,checked_at=now() WHERE account_id=$1 AND id=$2", account, zone, strict); e != nil {
		return e
	}
	if !strict {
		return errors.New("cloudflare_ssl_strict_required")
	}
	return nil
}
