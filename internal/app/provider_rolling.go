package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"log"
	"strings"
	"sync"
	"time"
)

type providerRefreshOptions struct{ Deadline, Lead, Poll time.Duration }

var providerRefreshDefaults = providerRefreshOptions{45 * time.Second, 30 * time.Second, time.Second}

// RunProviderSnapshots gives each account independent bounded progress. Admission
// encloses all work and finalization; discovery never holds SQL while waiting.
func (c Container) RunProviderSnapshots(ctx context.Context, admit func(context.Context, func(context.Context) error) error) {
	c.runProviderSnapshots(ctx, admit, providerRefreshDefaults)
}
func (c Container) runProviderSnapshots(ctx context.Context, admit func(context.Context, func(context.Context) error) error, o providerRefreshOptions) {
	if admit == nil {
		admit = func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
	}
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	done := make(chan string, 2)
	active := map[string]bool{}
	retry := map[string]time.Time{}
	var discoveryRetry time.Time
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		supervision.Pulse(ctx)
		supervision.Idle(ctx)
		select {
		case <-ctx.Done():
			return
		case id := <-done:
			delete(active, id)
			retry[id] = time.Now().Add(time.Second)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		for id, at := range retry {
			if !now.Before(at) {
				delete(retry, id)
			}
		}
		if len(active) < 2 && !now.Before(discoveryRetry) {
			excluded := make([]string, 0, len(active)+len(retry))
			for id := range active {
				excluded = append(excluded, id)
			}
			for id := range retry {
				excluded = append(excluded, id)
			}
			var ids []string
			err := admit(ctx, func(ctx context.Context) error {
				scan, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				var e error
				ids, e = c.discoverProviderRefreshes(scan, excluded, 2-len(active), o.Lead)
				return e
			})
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				discoveryRetry = time.Now().Add(5 * time.Second)
				log.Printf("provider refresh discovery: %v", err)
			} else {
				for _, id := range ids {
					active[id] = true
					wg.Add(1)
					go func(id string) {
						defer wg.Done()
						_ = admit(ctx, func(ctx context.Context) error {
							c.refreshProviderAccount(ctx, id, time.Minute, o.Lead, o.Deadline)
							return ctx.Err()
						})
						// Capacity two equals the maximum number of assigned workers, so
						// cancellation can join without a consumer draining completions.
						done <- id
					}(id)
				}
			}
		}
		timer.Reset(o.Poll)
	}
}

func (c Container) discoverProviderRefreshes(ctx context.Context, excluded []string, limit int, lead time.Duration) ([]string, error) {
	// Cadence calculation shares the locked recheck's exact idle predicate.
	// Backoff always wins, including independent hard restrictions.
	idle := strings.ReplaceAll(providerIdleSQL, "$1", "target.id")
	query := `WITH eligible AS (
 SELECT target.*,CASE WHEN $4 AND EXISTS(SELECT 1 FROM proxy_economy_policy p WHERE p.singleton AND p.enabled AND (p.canary_account_id IS NULL OR p.canary_account_id=target.id))
 AND (` + idle + `) THEN 300 ELSE 60 END AS desired_interval,
 (SELECT max(created_at) FROM provider_snapshots ps WHERE ps.account_id=target.id AND ps.canonical IS NOT NULL) AS snapshot_at
 FROM accounts target WHERE target.enabled AND target.deleted_at IS NULL AND target.deletion_requested_at IS NULL
 AND NOT(target.id=ANY($1::uuid[]))
 ), due AS (
 SELECT id,provider_refresh_attempt_at,
 GREATEST(
 GREATEST(COALESCE(provider_refresh_attempt_at,provider_checked_at,'epoch'::timestamptz),COALESCE(provider_error_at,'epoch'::timestamptz),COALESCE(provider_state_at,'epoch'::timestamptz))+
 GREATEST(CASE provider_state WHEN 'LOCKED' THEN 45 WHEN 'TOKEN_INVALID' THEN 300 WHEN 'PERMISSION_DENIED' THEN 300 WHEN 'BILLING_BLOCKED' THEN 300 WHEN 'RATE_LIMITED' THEN 120 WHEN 'TRANSPORT_ERROR' THEN 15 WHEN 'PROXY_ERROR' THEN 15 ELSE 0 END,
 CASE provider_error_state WHEN 'LOCKED' THEN 45 WHEN 'TOKEN_INVALID' THEN 300 WHEN 'PERMISSION_DENIED' THEN 300 WHEN 'BILLING_BLOCKED' THEN 300 WHEN 'RATE_LIMITED' THEN 120 WHEN 'TRANSPORT_ERROR' THEN 15 WHEN 'PROXY_ERROR' THEN 15 ELSE 0 END)*interval '1 second',
 CASE WHEN provider_inventory_interval_seconds=desired_interval AND provider_state='ACTIVE' AND COALESCE(provider_error_state,'')='' AND provider_checked_at IS NOT NULL AND provider_checked_at<=now() AND snapshot_at IS NOT NULL
 THEN LEAST(snapshot_at,provider_checked_at)+GREATEST(1,desired_interval-$3)*interval '1 second'
 ELSE 'epoch'::timestamptz END
 ) AS eligible_at FROM eligible
 )
 SELECT id::text FROM due WHERE eligible_at<=now()
 ORDER BY eligible_at,provider_refresh_attempt_at NULLS FIRST,id LIMIT $2`
	// IDs originate in PostgreSQL, not user input. Text encoding avoids driver-
	// specific array helpers while parameters remain bound.
	array := "{" + strings.Join(excluded, ",") + "}"
	rows, err := c.DB.QueryContext(ctx, query, array, limit, lead.Seconds(), c.Economy != nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
